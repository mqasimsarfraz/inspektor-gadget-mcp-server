package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func newWaitTool() server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Wait for a given amount of time"),
		mcp.WithNumber("waitTime",
			mcp.Description("Number of seconds to wait"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
	}
	tool := mcp.NewTool(
		"wait",
		opts...,
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: waitHandler(),
	}
}

func waitHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		waitTime := request.GetInt("waitTime", 1)
		time.Sleep(time.Duration(waitTime) * time.Second)
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				mcp.TextContent{
					Type: "text",
					Text: fmt.Sprintf("%d seconds have passed", waitTime),
				},
			},
		}, nil
	}
}
