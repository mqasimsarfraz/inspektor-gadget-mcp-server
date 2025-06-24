package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func (r *GadgetToolRegistry) newStopTool() server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Stops a gadget with an ID"),
		mcp.WithString("id",
			mcp.Description("ID of the running gadget"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
	}
	tool := mcp.NewTool(
		"stop-gadget",
		opts...,
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: r.stopHandler(),
	}
}

func (r *GadgetToolRegistry) stopHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetString("id", "")
		if id == "" {
			return nil, fmt.Errorf("an id is required")
		}

		err := r.gadgetMgr.Stop(id)
		if err != nil {
			return nil, fmt.Errorf("failed to stop gadget with id %q: %w", id, err)
		}
		return mcp.NewToolResultText(fmt.Sprintf("Gadget with ID %q has been stopped", id)), nil
	}
}

func (r *GadgetToolRegistry) newGetResultsTool() server.ServerTool {
	opts := []mcp.ToolOption{
		mcp.WithDescription("Returns the collected events from a gadget instance with a specific ID. Please review the data and provide a concise summary to the user."),
		mcp.WithString("id",
			mcp.Description("ID of the running gadget instance"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
	}
	tool := mcp.NewTool(
		"get-results",
		opts...,
	)
	return server.ServerTool{
		Tool:    tool,
		Handler: r.getResultsHandler(),
	}
}

func (r *GadgetToolRegistry) getResultsHandler() server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		id := request.GetString("id", "")
		if id == "" {
			return nil, fmt.Errorf("an id is required")
		}

		resp, err := r.gadgetMgr.Results(id)
		if err != nil {
			return nil, fmt.Errorf("attaching to gadget %s: %w", id, err)
		}
		return mcp.NewToolResultText(truncateResults(resp)), nil
	}
}
