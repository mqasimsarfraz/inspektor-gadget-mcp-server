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
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/deployer"
)

const (
	defaultChartUrl    = "oci://ghcr.io/inspektor-gadget/inspektor-gadget/charts/gadget"
	defaultReleaseName = "gadget"
	defaultNamespace   = "gadget"
)

func newDeployTool(registry *GadgetToolRegistry, images []string) server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Deploy Inspektor Gadget on the target system"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace to deploy Inspektor Gadget into, only set if user explicitly specifies a namespace"),
			mcp.DefaultString(defaultNamespace),
		),
		mcp.WithString("release",
			mcp.Description("Name of Helm release to create for Inspektor Gadget, only set if user explicitly specifies a release name"),
			mcp.DefaultString(defaultReleaseName),
		),
		mcp.WithString("chart_version",
			mcp.Description("Version of the Inspektor Gadget Helm chart to deploy, only set if user explicitly specifies a version"),
		),
	}
	tool := mcp.NewTool(
		"deploy_inspektor_gadget",
		opts...,
	)

	return server.ServerTool{
		Tool:    tool,
		Handler: deployHandler(registry, images),
	}
}

func deployHandler(registry *GadgetToolRegistry, images []string) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var err error
		version := request.GetString("chart_version", "")
		if version == "" {
			version, err = getLatestChartVersion()
			if err != nil {
				return nil, fmt.Errorf("get latest chart version: %w", err)
			}
		}
		chartUrl := fmt.Sprintf("%s:%s", defaultChartUrl, version)
		releaseName := request.GetString("release", defaultReleaseName)
		namespace := request.GetString("namespace", defaultNamespace)

		ist, err := deployer.NewDeployer(deployer.KubernetesEnv)
		if err != nil {
			return nil, fmt.Errorf("create deployer: %w", err)
		}

		opts := []deployer.RunOption{
			deployer.WithChartURL(chartUrl),
			deployer.WithReleaseName(releaseName),
			deployer.WithNamespace(namespace),
		}
		err = ist.Deploy(ctx, opts...)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Register the tool with the registry
		go func() {
			// We need to wait to ensure Inspektor Gadget is fully deployed before registering the tools
			// TODO: Can we do this more elegantly?
			log.Debug("Waiting for Inspektor Gadget to be fully deployed before registering tools")
			time.Sleep(10 * time.Second)

			registry.mu.Lock()
			defer registry.mu.Unlock()
			err = registry.registerGadgets(ctx, images)
			if err != nil {
				log.Warn("failed to register tool", "error", err)
				return
			}
			for _, callback := range registry.callbacks {
				log.Debug("Invoking tool registry callback", "tools_count", len(registry.tools))
				callback(registry.all()...)
			}
		}()

		return mcp.NewToolResultText("Inspektor Gadget deploy completed successfully"), nil
	}
}

// getLatestChartVersion is a placeholder function that simulates fetching the latest chart version.
// TODO: Get this from registry or github releases.
func getLatestChartVersion() (string, error) {
	return "1.0.0-dev", nil
}
