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
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/inspektor-gadget/inspektor-gadget/pkg/gadget-service/api"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"gopkg.in/yaml.v3"

	metadatav1 "github.com/inspektor-gadget/inspektor-gadget/pkg/metadata/v1"
	"github.com/mqasimsarfraz/inspektor-gadget-mcp-server/pkg/gadgetmanager"
)

const descriptionTemplate = `The {{ .Name }} tool is designed to {{ .Description }} in {{ .Environment }} environments. It uses a map of key-value pairs called params to configure its behavior but does not require any specific parameters to function.`

var log = slog.Default().With("component", "tools")

// GadgetToolRegistry is a simple registry for server tools based on gadgets.
type GadgetToolRegistry struct {
	tools     []server.ServerTool
	gadgetMgr gadgetmanager.GadgetManager
	logger    *slog.Logger
}

type ToolData struct {
	Name        string
	Description string
	Environment string
}

// NewToolRegistry creates a new GadgetToolRegistry instance.
func NewToolRegistry(manager gadgetmanager.GadgetManager) *GadgetToolRegistry {
	return &GadgetToolRegistry{
		tools:     make([]server.ServerTool, 0),
		gadgetMgr: manager,
		logger:    slog.Default(),
	}
}

func (r *GadgetToolRegistry) All() []server.ServerTool {
	return r.tools
}

func (r *GadgetToolRegistry) Prepare(ctx context.Context, images []string) error {
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
			log.Warn("Skipping gadget image due to error", "image", result.info.ImageName, "error", result.err)
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
		r.tools = append(r.tools, st)
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
	tmpl, err := template.New("description").Parse(descriptionTemplate)
	if err != nil {
		return tool, fmt.Errorf("parsing template: %w", err)
	}
	var out bytes.Buffer
	td := ToolData{
		Name:        normalizeToolName(metadata.Name),
		Description: metadata.Description,
		Environment: "Kubernetes",
	}
	if err = tmpl.ExecuteTemplate(&out, "description", td); err != nil {
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
			mcp.DefaultNumber(10),
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
