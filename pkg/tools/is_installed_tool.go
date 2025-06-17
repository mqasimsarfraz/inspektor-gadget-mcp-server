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
)

func newIsDeployedTool() server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Check if Inspektor Gadget is deployed on the target system. Doesn't rely on if mcp server deployed it or not but checks if the Inspektor Gadget resources are present in the cluster."),
		mcp.WithReadOnlyHintAnnotation(true),
	}
	tool := mcp.NewTool(
		"is_inspektor_gadget_deployed",
		opts...,
	)

	return server.ServerTool{
		Tool:    tool,
		Handler: isDeployedHandler,
	}
}

func isDeployedHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	isDeployed, ns, err := isInspektorGadgetDeployed(ctx)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if !isDeployed {
		return mcp.NewToolResultError("Inspektor Gadget is not deployed"), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Inspektor Gadget is deployed in namespace %s", ns)), nil
}
