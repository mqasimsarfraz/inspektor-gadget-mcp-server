// Copyright 2025 The Inspektor Gadget authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gadgetmanager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/inspektor-gadget/inspektor-gadget/cmd/kubectl-gadget/utils"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/datasource"
	igjson "github.com/inspektor-gadget/inspektor-gadget/pkg/datasource/formatters/json"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/environment"
	gadgetcontext "github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-context"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-service/api"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/operators/simple"
	igruntime "github.com/inspektor-gadget/inspektor-gadget/pkg/runtime"
	grpcruntime "github.com/inspektor-gadget/inspektor-gadget/pkg/runtime/grpc"
)

// GadgetManager is an interface for managing gadgets.
type GadgetManager interface {
	// Run starts a gadget with the given image and parameters, returning the output as a string.
	Run(image string, params map[string]string, timeout time.Duration) (string, error)
	// RunDetached starts a gadget with the given image and parameters in the background, returning its ID.
	RunDetached(image string, params map[string]string) (string, error)
	// Results returns the stored result buffer from a gadget
	Results(id string) (string, error)
	// Stop stops a gadget
	Stop(id string) error
	// GetInfo retrieves information about a gadget image via runtime.
	GetInfo(ctx context.Context, image string) (*api.GadgetInfo, error)
	// Close closes the gadget manager and releases any resources.
	Close() error
}

type gadgetManager struct {
	runtime igruntime.Runtime
}

// NewGadgetManager creates a new GadgetManager instance.
func NewGadgetManager(runtime string) (GadgetManager, error) {
	var rt igruntime.Runtime
	var err error
	switch runtime {
	case "grpc-k8s":
		rt, err = newGrpcK8sRuntime()
	default:
		return nil, fmt.Errorf("unsupported gadget manager runtime: %s", runtime)
	}
	if err != nil {
		return nil, fmt.Errorf("creating gadget manager runtime: %w", err)
	}
	if err := rt.Init(nil); err != nil {
		return nil, fmt.Errorf("initializing gadget manager runtime: %w", err)
	}
	return &gadgetManager{
		runtime: rt,
	}, nil
}

func newGrpcK8sRuntime() (igruntime.Runtime, error) {
	environment.Environment = environment.Kubernetes
	rt := grpcruntime.New(grpcruntime.WithConnectUsingK8SProxy)
	if err := rt.Init(nil); err != nil {
		return nil, fmt.Errorf("initializing grpc gadget manager: %w", err)
	}
	config, err := utils.KubernetesConfigFlags.ToRESTConfig()
	if err != nil {
		return nil, fmt.Errorf("creating RESTConfig: %w", err)
	}
	rt.SetRestConfig(config)
	return rt, nil
}

func (g *gadgetManager) Run(image string, params map[string]string, timeout time.Duration) (string, error) {
	const opPriority = 50000
	var jsonBuffer []byte
	myOperator := simple.New("myOperator",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, d := range gadgetCtx.GetDataSources() {
				jsonFormatter, _ := igjson.New(d,
					igjson.WithShowAll(true),
				)

				// skip data sources that have the annotation "cli.default-output-mode"
				// set to "none"Add commentMore actions
				if m, ok := d.Annotations()["cli.default-output-mode"]; ok && m == "none" {
					continue
				}

				d.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					jsonData := jsonFormatter.Marshal(data)
					jsonBuffer = append(jsonBuffer, jsonData...)
					jsonBuffer = append(jsonBuffer, '\n')
					return nil
				}, opPriority)
			}
			return nil
		}),
	)

	gadgetCtx := gadgetcontext.New(
		context.Background(),
		image,
		gadgetcontext.WithDataOperators(
			myOperator,
		),
		gadgetcontext.WithTimeout(timeout),
	)

	if err := g.runtime.RunGadget(gadgetCtx, nil, params); err != nil {
		return "", fmt.Errorf("running gadget: %w", err)
	}
	return string(jsonBuffer), nil
}

func (g *gadgetManager) RunDetached(image string, params map[string]string) (string, error) {
	gadgetCtx := gadgetcontext.New(
		context.Background(),
		image,
	)

	p := g.runtime.ParamDescs().ToParams()

	newID := make([]byte, 16)
	rand.Read(newID)
	idString := hex.EncodeToString(newID)

	p.Set(grpcruntime.ParamID, idString)
	p.Set(grpcruntime.ParamDetach, "true")
	if err := g.runtime.RunGadget(gadgetCtx, p, params); err != nil {
		return "", fmt.Errorf("running gadget: %w", err)
	}
	return idString, nil
}

func (g *gadgetManager) Stop(id string) error {
	if err := g.runtime.(*grpcruntime.Runtime).RemoveGadgetInstance(context.Background(), g.runtime.ParamDescs().ToParams(), id); err != nil {
		return fmt.Errorf("stopping to gadget: %w", err)
	}
	return nil
}

func (g *gadgetManager) Results(id string) (string, error) {
	const opPriority = 50000
	var jsonBuffer []byte
	myOperator := simple.New("myOperator",
		simple.OnInit(func(gadgetCtx operators.GadgetContext) error {
			for _, d := range gadgetCtx.GetDataSources() {
				jsonFormatter, _ := igjson.New(d,
					igjson.WithShowAll(true),
				)

				d.Subscribe(func(source datasource.DataSource, data datasource.Data) error {
					jsonData := jsonFormatter.Marshal(data)
					jsonBuffer = append(jsonBuffer, jsonData...)
					jsonBuffer = append(jsonBuffer, '\n')
					return nil
				}, opPriority)
			}
			return nil
		}),
	)

	to, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()

	gadgetCtx := gadgetcontext.New(
		to,
		id,
		gadgetcontext.WithDataOperators(
			myOperator,
		),
		gadgetcontext.WithID(id),
		gadgetcontext.WithUseInstance(true),
		gadgetcontext.WithTimeout(time.Second),
	)

	if err := g.runtime.RunGadget(gadgetCtx, g.runtime.ParamDescs().ToParams(), map[string]string{}); err != nil {
		return "", fmt.Errorf("attaching to gadget: %w", err)
	}
	return string(jsonBuffer), nil
}

func (g *gadgetManager) GetInfo(ctx context.Context, image string) (*api.GadgetInfo, error) {
	gadgetCtx := gadgetcontext.New(
		ctx,
		image,
	)

	info, err := g.runtime.GetGadgetInfo(gadgetCtx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("get gadget info: %w", err)
	}
	return info, nil
}

func (g *gadgetManager) Close() error {
	if g.runtime != nil {
		return g.runtime.Close()
	}
	return nil
}
