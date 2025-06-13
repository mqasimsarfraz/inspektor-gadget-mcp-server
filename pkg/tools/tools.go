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

package tools

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/inspektor-gadget/inspektor-gadget/cmd/kubectl-gadget/utils"
	"github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-service/api"
	metadatav1 "github.com/inspektor-gadget/inspektor-gadget/pkg/metadata/v1"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/gadgetmanager"
)

//go:embed templates
var templates embed.FS

var log = slog.Default().With("component", "tools")

type ToolRegistryCallback func(tool ...server.ServerTool)

// GadgetToolRegistry is a simple registry for server tools based on gadgets.
type GadgetToolRegistry struct {
	tools     map[string]server.ServerTool
	mu        sync.Mutex
	callbacks []ToolRegistryCallback
	gadgetMgr gadgetmanager.GadgetManager
}

type ToolData struct {
	Name        string
	Description string
	Environment string
}

// NewToolRegistry creates a new GadgetToolRegistry instance.
func NewToolRegistry(manager gadgetmanager.GadgetManager) *GadgetToolRegistry {
	return &GadgetToolRegistry{
		tools:     make(map[string]server.ServerTool),
		gadgetMgr: manager,
	}
}

func (r *GadgetToolRegistry) all() []server.ServerTool {
	tools := make([]server.ServerTool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}
	return tools
}

func (r *GadgetToolRegistry) RegisterCallback(callback ToolRegistryCallback) {
	r.callbacks = append(r.callbacks, callback)
}

func (r *GadgetToolRegistry) Prepare(ctx context.Context, images []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	deployTool := newDeployTool(r, images)
	undeployTool := newUndeployTool()
	r.tools[deployTool.Tool.Name] = deployTool
	r.tools[undeployTool.Tool.Name] = undeployTool

	// Skip registering gadgets if Inspektor Gadget is not deployed
	deployed, _, err := isInspektorGadgetDeployed(ctx)
	if err != nil {
		return fmt.Errorf("checking if Inspektor Gadget is deployed: %w", err)
	}
	if deployed {
		err = r.registerGadgets(ctx, images)
		if err != nil {
			return fmt.Errorf("registering gadgets: %w", err)
		}
	} else {
		log.Info("Inspektor Gadget is not deployed, skipping gadget registration")
	}

	for _, callback := range r.callbacks {
		log.Debug("Invoking tool registry callback", "tools_count", len(r.tools))
		callback(r.all()...)
	}

	return nil
}

func (r *GadgetToolRegistry) registerGadgets(ctx context.Context, images []string) error {
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		img  string
		info *api.GadgetInfo
		err  error
	}, len(images))

	for _, img := range images {
		wg.Add(1)
		go func(image string) {
			defer wg.Done()
			info, err := r.gadgetMgr.GetInfo(ctx, image)
			resultsChan <- struct {
				img  string
				info *api.GadgetInfo
				err  error
			}{img: img, info: info, err: err}
		}(img)
	}

	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	for result := range resultsChan {
		if result.err != nil {
			log.Warn("Skipping gadget image due to error", "image", result.img, "error", result.err)
			continue
		}
		info := result.info
		t, err := r.toolFromGadgetInfo(info)
		if err != nil {
			return fmt.Errorf("creating tool from gadget info for %s: %w", info.ImageName, err)
		}
		h := r.handlerFromGadgetInfo(info)
		st := server.ServerTool{
			Tool:    t,
			Handler: h,
		}
		log.Debug("Adding tool", "image", info.ImageName, "name", t.Name)
		r.tools[normalizeToolName(info.ImageName)] = st
	}

	return nil
}

func (r *GadgetToolRegistry) toolFromGadgetInfo(info *api.GadgetInfo) (mcp.Tool, error) {
	var tool mcp.Tool
	var metadata *metadatav1.GadgetMetadata
	err := yaml.Unmarshal(info.Metadata, &metadata)
	if err != nil {
		return tool, fmt.Errorf("unmarshalling gadget metadata: %w", err)
	}
	tmpl, err := template.ParseFS(templates, "templates/toolDescription.tmpl")
	if err != nil {
		return tool, fmt.Errorf("parsing template: %w", err)
	}
	var out bytes.Buffer
	td := ToolData{
		Name:        normalizeToolName(metadata.Name),
		Description: metadata.Description,
		Environment: "Kubernetes",
	}
	if err = tmpl.Execute(&out, td); err != nil {
		return tool, fmt.Errorf("executing template for gadget %s: %w", info.ImageName, err)
	}
	var params = make(map[string]interface{})
	for _, p := range info.Params {
		params[p.Prefix+p.Key] = map[string]interface{}{
			"type":        "string",
			"description": p.Description,
		}
	}

	opts := []mcp.ToolOption{
		mcp.WithDescription(out.String()),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithObject("params",
			mcp.Required(),
			mcp.Description("key-value pairs of parameters to pass to the gadget"),
			mcp.Properties(params),
		),
		mcp.WithNumber("timeout",
			mcp.Description("Timeout in seconds for the gadget to run"),
		),
	}
	tool = mcp.NewTool(
		normalizeToolName(metadata.Name),
		opts...,
	)
	return tool, nil
}

func (r *GadgetToolRegistry) handlerFromGadgetInfo(info *api.GadgetInfo) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		timeout := 10 * time.Second
		params := defaultParamsFromGadgetInfo(info)
		args := request.GetArguments()
		if args != nil {
			if t, ok := args["timeout"].(float64); ok {
				timeout = time.Duration(t) * time.Second
			}
			if p, ok := args["params"].(map[string]interface{}); ok {
				for k, v := range p {
					if strVal, ok := v.(string); ok {
						params[k] = strVal
					} else {
						return nil, fmt.Errorf("invalid type for parameter %s: expected string, got %T", k, v)
					}
				}
			}
		}

		log.Debug("Running gadget", "image", info.ImageName, "params", params, "timeout", timeout)
		resp, err := r.gadgetMgr.Run(info.ImageName, params, timeout)
		if err != nil {
			return nil, fmt.Errorf("starting gadget %s: %w", info.ImageName, err)
		}
		return mcp.NewToolResultText(resp), nil
	}
}

func defaultParamsFromGadgetInfo(info *api.GadgetInfo) map[string]string {
	params := make(map[string]string)
	for _, p := range info.Params {
		if p.DefaultValue != "" {
			params[p.Prefix+p.Key] = p.DefaultValue
		}
	}
	return params
}

func normalizeToolName(name string) string {
	// Normalize tool name to lowercase and replace spaces with dashes
	return strings.Replace(name, " ", "_", -1)
}

// A generic function to check if Inspektor Gadget is deployed in the cluster e.g using kubectl-gadget, helm, or other means.
// It returns a boolean indicating if it is deployed, the namespace it is deployed in, and any error encountered
func isInspektorGadgetDeployed(ctx context.Context) (bool, string, error) {
	restConfig, err := utils.KubernetesConfigFlags.ToRESTConfig()
	if err != nil {
		return false, "", fmt.Errorf("creating RESTConfig: %w", err)
	}

	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return false, "", fmt.Errorf("setting up trace client: %w", err)
	}

	opts := metav1.ListOptions{LabelSelector: "k8s-app=gadget"}
	pods, err := client.CoreV1().Pods("").List(ctx, opts)
	if err != nil {
		return false, "", fmt.Errorf("getting pods: %w", err)
	}
	if len(pods.Items) == 0 {
		log.Debug("No Inspektor Gadget pods found")
		return false, "", nil
	}

	var namespaces []string
	for _, pod := range pods.Items {
		if !slices.Contains(namespaces, pod.Namespace) {
			namespaces = append(namespaces, pod.Namespace)
		}
	}
	if len(namespaces) > 1 {
		log.Debug("Multiple namespaces found for Inspektor Gadget pods", "namespaces", namespaces)
		return false, "", fmt.Errorf("multiple namespaces found for Inspektor Gadget pods: %v", namespaces)
	}
	return true, namespaces[0], nil
}
