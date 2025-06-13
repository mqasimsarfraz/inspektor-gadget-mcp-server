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

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/inspektor-gadget/ig-mcp-server/pkg/deployer"
)

func newUndeployTool() server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Undeploy Inspektor Gadget from the target system"),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithString("release",
			mcp.Description("Name of Helm release to remove, only set if user explicitly specifies a release name"),
			mcp.DefaultString(defaultReleaseName),
		),
		mcp.WithString("namespace",
			mcp.Description("Kubernetes namespace to undeploy Inspektor Gadget from, only set if user explicitly specifies a namespace"),
			mcp.DefaultString(defaultNamespace),
		),
	}
	tool := mcp.NewTool(
		"undeploy_inspektor_gadget",
		opts...,
	)

	return server.ServerTool{
		Tool:    tool,
		Handler: undeployHandler,
	}
}

func undeployHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	releaseName := request.GetString("release", defaultReleaseName)
	namespace := request.GetString("namespace", defaultNamespace)

	ist, err := deployer.NewDeployer(deployer.KubernetesEnv)
	if err != nil {
		return nil, fmt.Errorf("create deployer: %w", err)
	}

	opts := []deployer.RunOption{
		deployer.WithReleaseName(releaseName),
		deployer.WithNamespace(namespace),
	}
	err = ist.Undeploy(ctx, opts...)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText("Inspektor Gadget undeploy completed successfully"), nil
}
