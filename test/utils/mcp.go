// Package utils provides MCP testing utilities adapted for the newer MCP SDK.
// These utilities help test MCP protocol compliance and server behavior.
package utils

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// McpInitRequest creates a standard MCP initialization request for testing.
// This is a simplified version for now - will be enhanced as we build MCP tests.
func McpInitRequest() map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
}

// CreateTestTools creates a set of test tools for validation.
// This is useful for testing tool discovery and execution.
func CreateTestTools() []mcp.Tool {
	return []mcp.Tool{
		{
			Name:        "test_tool_get",
			Description: "A test tool for getting resources",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Name of the resource",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "test_tool_list",
			Description: "A test tool for listing resources",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
	}
}

// ValidateInitializeResponse validates that an MCP initialize response is well-formed.
// Simplified version using generic map for now.
func ValidateInitializeResponse(t *testing.T, response map[string]any) {
	require.Contains(t, response, "protocolVersion", "Response should contain protocol version")
	require.Contains(t, response, "capabilities", "Response should contain capabilities")
	require.Contains(t, response, "serverInfo", "Response should contain server info")

	serverInfo, ok := response["serverInfo"].(map[string]any)
	require.True(t, ok, "serverInfo should be a map")
	require.Contains(t, serverInfo, "name", "Server info should contain name")
	require.Contains(t, serverInfo, "version", "Server info should contain version")
}

// ValidateListToolsResponse validates that a list tools response contains expected tools.
// Simplified version using generic types for now.
func ValidateListToolsResponse(t *testing.T, response map[string]any, expectedToolNames []string) {
	require.Contains(t, response, "tools", "Response should contain tools")

	tools, ok := response["tools"].([]any)
	require.True(t, ok, "Tools should be an array")

	toolNames := make([]string, len(tools))
	for i, toolInterface := range tools {
		tool, ok := toolInterface.(map[string]any)
		require.True(t, ok, "Each tool should be a map")

		name, ok := tool["name"].(string)
		require.True(t, ok, "Tool should have a name string")
		toolNames[i] = name

		require.NotEmpty(t, name, "Tool name should not be empty")
		require.Contains(t, tool, "description", "Tool should have description")
		require.Contains(t, tool, "inputSchema", "Tool should have input schema")
	}

	// Verify expected tools are present
	for _, expectedName := range expectedToolNames {
		require.Contains(t, toolNames, expectedName, "Expected tool %s should be present", expectedName)
	}
}

// ValidateCallToolResponse validates that a call tool response is well-formed.
// Simplified version using generic types for now.
func ValidateCallToolResponse(t *testing.T, response map[string]any) {
	require.NotNil(t, response, "Tool call result should not be nil")
	require.Contains(t, response, "content", "Tool call result should contain content")
}

// AssertToolExists verifies that a tool with the given name exists in the tools list.
// Simplified version for basic tool validation.
func AssertToolExists(t *testing.T, toolNames []string, toolName string) {
	require.Contains(t, toolNames, toolName, "Expected to find tool named %s", toolName)
}

// AssertToolCount verifies the expected number of tools.
func AssertToolCount(t *testing.T, tools []any, expectedCount int) {
	require.Len(t, tools, expectedCount, "Expected %d tools, got %d", expectedCount, len(tools))
}

// CreateTestContext creates a test context with timeout.
func CreateTestContext(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30) // 30 second timeout
	t.Cleanup(cancel)
	return ctx
}
