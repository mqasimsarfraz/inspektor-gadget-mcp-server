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

const maxResultLen = 64 * 1024 // 64kb

//go:embed templates
var templates embed.FS

var log = slog.Default().With("component", "tools")

type ToolRegistryCallback func(tool ...server.ServerTool)

// GadgetToolRegistry is a simple registry for server tools based on gadgets.
type GadgetToolRegistry struct {
	gadgetMgr gadgetmanager.GadgetManager
	env       string

	tools     map[string]server.ServerTool
	mu        sync.Mutex
	callbacks []ToolRegistryCallback
}

type ToolData struct {
	Name        string
	Description string
	Environment string
	Fields      []FieldData
}

type FieldData struct {
	Name           string
	Description    string
	PossibleValues string
}

// NewToolRegistry creates a new GadgetToolRegistry instance.
func NewToolRegistry(manager gadgetmanager.GadgetManager, env string) *GadgetToolRegistry {
	return &GadgetToolRegistry{
		tools:     make(map[string]server.ServerTool),
		gadgetMgr: manager,
		env:       env,
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

	r.registerDefaultTools(ctx, images)

	return nil
}

func (r *GadgetToolRegistry) registerDefaultTools(ctx context.Context, images []string) {
	for _, tool := range r.getDefaultTools(ctx, images) {
		log.Debug("Adding default tool", "name", tool.Tool.Name)
		r.tools[tool.Tool.Name] = tool
	}

	// invoke callbacks
	for _, callback := range r.callbacks {
		log.Debug("Invoking tool registry callback", "tools_count", len(r.tools))
		callback(r.all()...)
	}
}

func (r *GadgetToolRegistry) getDefaultTools(ctx context.Context, images []string) []server.ServerTool {
	tools := []server.ServerTool{
		r.newStopTool(),
		r.newGetResultsTool(),
		newWaitTool(),
	}

	// Add environment-specific tools
	tools = append(tools, r.getToolsForEnvironment(ctx, images)...)

	return tools
}

func (r *GadgetToolRegistry) getToolsForEnvironment(ctx context.Context, images []string) []server.ServerTool {
	var tools []server.ServerTool
	if r.env == "kubernetes" {
		tools = []server.ServerTool{
			newDeployTool(r, images),
			newUndeployTool(),
			newIsDeployedTool(),
		}
		deployed, _, err := isInspektorGadgetDeployed(ctx)
		if err != nil || !deployed {
			log.Warn("Inspektor Gadget is not deployed, skipping fetching gadget tools", "error", err)
			return tools
		}
	}

	gadgetTools, err := r.getGadgetTools(ctx, images)
	if err != nil {
		log.Warn("Failed to get gadget tools", "error", err)
		return tools
	}

	return append(tools, gadgetTools...)
}

func (r *GadgetToolRegistry) getGadgetTools(ctx context.Context, images []string) ([]server.ServerTool, error) {
	sem := make(chan struct{}, 8) // Limit concurrency to 8
	var wg sync.WaitGroup
	resultsChan := make(chan struct {
		img  string
		info *api.GadgetInfo
		err  error
	}, len(images))

	for _, img := range images {
		wg.Add(1)
		sem <- struct{}{}
		go func(image string) {
			defer func() {
				wg.Done()
				<-sem
			}()
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

	var tools []server.ServerTool
	for result := range resultsChan {
		if result.err != nil {
			log.Warn("Skipping gadget image due to error", "image", result.img, "error", result.err)
			continue
		}
		info := result.info
		t, err := r.toolFromGadgetInfo(info)
		if err != nil {
			return nil, fmt.Errorf("creating tool from gadget info for %s: %w", info.ImageName, err)
		}
		h := r.handlerFromGadgetInfo(info)
		st := server.ServerTool{
			Tool:    t,
			Handler: h,
		}
		log.Debug("Adding tool", "name", t.Name, "image", info.ImageName)
		tools = append(tools, st)
	}

	return tools, nil
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
	var fields []FieldData
	if len(info.DataSources) > 0 {
		for _, field := range info.DataSources[0].Fields {
			fields = append(fields, FieldData{
				Name:           field.FullName,
				Description:    field.Annotations[metadatav1.DescriptionAnnotation],
				PossibleValues: field.Annotations[metadatav1.ValueOneOfAnnotation],
			})
		}
	}
	var out bytes.Buffer
	td := ToolData{
		Name:        normalizeToolName(metadata.Name),
		Description: metadata.Description,
		Environment: r.env,
		Fields:      fields,
	}
	if err = tmpl.Execute(&out, td); err != nil {
		return tool, fmt.Errorf("executing template for gadget %s: %w", info.ImageName, err)
	}
	params := make(map[string]interface{})
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
		mcp.WithBoolean("background",
			mcp.Description("Run in background, allowing the gadget run continuously until stopped, allowing real-time data or "+
				"interaction with other tools. Unless specified, the gadget should run in the foreground and return results after completion."+
				"But if gadget needs to run for longer periods or collect some real-time data after performing an action set this to true.",
			),
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
		background := false
		if args != nil {
			if t, ok := args["background"]; ok {
				background = t.(bool)
			}
			if t, ok := args["timeout"].(float64); ok {
				timeout = time.Duration(t) * time.Second
			}
			// set map-fetch-interval to half of the timeout to limit the volume of data fetched
			if _, ok := params["operator.oci.ebpf.map-fetch-interval"]; ok && !background {
				params["operator.oci.ebpf.map-fetch-interval"] = (timeout / 2).String()
			}
			// If params is provided, merge it with the default parameters
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

		if background {
			id, err := r.gadgetMgr.RunDetached(info.ImageName, params)
			if err != nil {
				return nil, fmt.Errorf("running gadget: %w", err)
			}
			return mcp.NewToolResultText(fmt.Sprintf("The gadget has been started with ID %s.", id)), nil
		}

		log.Debug("Running gadget", "image", info.ImageName, "params", params, "timeout", timeout)
		resp, err := r.gadgetMgr.Run(info.ImageName, params, timeout)
		if err != nil {
			return nil, fmt.Errorf("starting gadget %s: %w", info.ImageName, err)
		}
		return mcp.NewToolResultText(truncateResults(resp)), nil
	}
}

func defaultParamsFromGadgetInfo(info *api.GadgetInfo) map[string]string {
	params := make(map[string]string)
	for _, p := range info.Params {
		params[p.Prefix+p.Key] = p.DefaultValue
	}
	return params
}

func normalizeToolName(name string) string {
	// Normalize tool name to lowercase and replace spaces with dashes
	return strings.ReplaceAll(name, " ", "_")
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

func truncateResults(results string) string {
	if len(results) > maxResultLen {
		return fmt.Sprintf("\n<results>%s</results>\n<isTruncated>true</isTruncated>\n", results[:maxResultLen]+"…")
	}
	return fmt.Sprintf("\n<results>%s</results>\n", results)
}
