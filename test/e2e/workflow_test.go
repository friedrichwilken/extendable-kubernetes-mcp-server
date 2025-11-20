// Package e2e contains end-to-end tests that validate complete MCP client-server workflows.
// These tests simulate real-world usage scenarios and validate the entire system end-to-end.
package e2e

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

// TestCompleteWorkflowStdio tests a complete MCP workflow over stdio transport
func TestCompleteWorkflowStdio(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary
	serverPath := buildServerBinary(t)

	// Start server in stdio mode
	cmd := exec.Command(serverPath, "--log-level", "0")

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err, "Failed to create stdin pipe")

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "Failed to create stdout pipe")

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err, "Failed to create stderr pipe")

	// Start the server
	err = cmd.Start()
	require.NoError(t, err, "Failed to start server")

	// Cleanup
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	})

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Test complete workflow: Initialize -> List Tools -> Call Tool -> Cleanup
	t.Run("complete_mcp_workflow", func(t *testing.T) {
		testCompleteWorkflow(t, stdin, stdout)
	})
}

func testCompleteWorkflow(t *testing.T, stdin io.Writer, stdout io.Reader) {
	// Step 1: Initialize MCP connection
	t.Log("Step 1: Initializing MCP connection...")
	initRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "e2e-test-client",
				"version": "1.0.0",
			},
		},
	}

	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err, "Failed to send init request")

	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	require.NotEmpty(t, initResponse, "Should receive init response")

	var parsedInit map[string]any
	err = json.Unmarshal([]byte(initResponse), &parsedInit)
	require.NoError(t, err, "Should parse init response")

	// Verify successful initialization
	if result, ok := parsedInit["result"]; ok {
		resultMap := result.(map[string]any)
		assert.Contains(t, resultMap, "protocolVersion", "Init should contain protocol version")
		assert.Contains(t, resultMap, "capabilities", "Init should contain capabilities")
		t.Log("✅ MCP initialization successful")
	} else {
		t.Skipf("MCP initialization failed (may be expected without k8s): %v", parsedInit["error"])
		return
	}

	// Step 2: Discover available tools
	t.Log("Step 2: Discovering available tools...")
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	err = sendJSONRPCRequest(t, stdin, listToolsRequest)
	require.NoError(t, err, "Failed to send tools/list request")

	toolsResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	require.NotEmpty(t, toolsResponse, "Should receive tools response")

	var parsedTools map[string]any
	err = json.Unmarshal([]byte(toolsResponse), &parsedTools)
	require.NoError(t, err, "Should parse tools response")

	var tools []any
	if result, ok := parsedTools["result"]; ok {
		resultMap := result.(map[string]any)
		if toolsList, ok := resultMap["tools"].([]any); ok {
			tools = toolsList
			t.Logf("✅ Found %d tools", len(tools))
		}
	}

	require.NotEmpty(t, tools, "Should have at least one tool available")

	// Step 3: Call a safe read-only tool (namespaces_list)
	t.Log("Step 3: Calling namespaces_list tool...")
	toolCallRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "namespaces_list",
			"arguments": map[string]any{},
		},
	}

	err = sendJSONRPCRequest(t, stdin, toolCallRequest)
	require.NoError(t, err, "Failed to send tool call request")

	toolResponse := readJSONRPCResponse(t, stdout, 15*time.Second)
	require.NotEmpty(t, toolResponse, "Should receive tool response")

	var parsedToolCall map[string]any
	err = json.Unmarshal([]byte(toolResponse), &parsedToolCall)
	require.NoError(t, err, "Should parse tool call response")

	// Verify tool call result (may succeed or fail depending on k8s connectivity)
	if result, ok := parsedToolCall["result"]; ok {
		t.Log("✅ Tool call successful")
		assert.NotNil(t, result, "Tool call should return result")
	} else if errorObj, ok := parsedToolCall["error"]; ok {
		errorMap := errorObj.(map[string]any)
		t.Logf("Tool call failed (expected without k8s): %v", errorMap["message"])
		// This is acceptable - we're testing the protocol, not k8s connectivity
	}

	// Step 4: Test notification (if supported)
	t.Log("Step 4: Testing notifications...")
	notificationRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}

	err = sendJSONRPCRequest(t, stdin, notificationRequest)
	// Notifications don't expect responses, so we just verify no errors
	assert.NoError(t, err, "Should send notification without error")

	t.Log("✅ Complete workflow test successful!")
}

// TestWorkflowPerformance tests performance under various loads
func TestWorkflowPerformance(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary
	serverPath := buildServerBinary(t)

	// Test scenarios with different loads
	scenarios := []struct {
		name              string
		concurrentClients int
		requestsPerClient int
		timeout           time.Duration
	}{
		{"single_client", 1, 10, 30 * time.Second},
		{"light_load", 3, 5, 45 * time.Second},
		{"moderate_load", 5, 3, 60 * time.Second},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			testWorkflowPerformance(t, serverPath, scenario.concurrentClients, scenario.requestsPerClient, scenario.timeout)
		})
	}
}

func testWorkflowPerformance(t *testing.T, serverPath string, concurrentClients, requestsPerClient int, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Start server
	cmd := exec.Command(serverPath, "--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	// Performance metrics
	startTime := time.Now()
	successCount := 0
	errorCount := 0

	results := make(chan struct {
		success  bool
		duration time.Duration
	}, concurrentClients*requestsPerClient)

	// Launch concurrent clients
	for i := 0; i < concurrentClients; i++ {
		go func(clientID int) {
			clientStartTime := time.Now()

			// Each client performs initialization + multiple tool calls
			for j := 0; j < requestsPerClient; j++ {
				requestStartTime := time.Now()

				// Send a simple list tools request
				listToolsRequest := map[string]any{
					"jsonrpc": "2.0",
					"id":      clientID*1000 + j,
					"method":  "tools/list",
					"params":  map[string]any{},
				}

				err := sendJSONRPCRequest(t, stdin, listToolsRequest)
				if err != nil {
					results <- struct {
						success  bool
						duration time.Duration
					}{false, time.Since(requestStartTime)}
					continue
				}

				// Try to read response (with timeout)
				response := readJSONRPCResponse(t, stdout, 5*time.Second)
				success := response != ""

				results <- struct {
					success  bool
					duration time.Duration
				}{success, time.Since(requestStartTime)}
			}

			t.Logf("Client %d completed in %v", clientID, time.Since(clientStartTime))
		}(i)
	}

	// Collect results
	totalRequests := concurrentClients * requestsPerClient
	responseTimes := make([]time.Duration, 0, totalRequests)

	for i := 0; i < totalRequests; i++ {
		select {
		case result := <-results:
			responseTimes = append(responseTimes, result.duration)
			if result.success {
				successCount++
			} else {
				errorCount++
			}
		case <-ctx.Done():
			t.Logf("Test timed out after %v", timeout)
			break
		}
	}

	totalDuration := time.Since(startTime)

	// Performance analysis
	t.Logf("Performance Results for %d concurrent clients, %d requests each:", concurrentClients, requestsPerClient)
	t.Logf("  Total Duration: %v", totalDuration)
	t.Logf("  Success Rate: %d/%d (%.1f%%)", successCount, totalRequests, float64(successCount)/float64(totalRequests)*100)
	t.Logf("  Error Rate: %d/%d (%.1f%%)", errorCount, totalRequests, float64(errorCount)/float64(totalRequests)*100)

	if len(responseTimes) > 0 {
		// Calculate average response time
		totalResponseTime := time.Duration(0)
		for _, rt := range responseTimes {
			totalResponseTime += rt
		}
		avgResponseTime := totalResponseTime / time.Duration(len(responseTimes))
		t.Logf("  Average Response Time: %v", avgResponseTime)

		// Performance assertions
		assert.True(t, float64(successCount)/float64(totalRequests) >= 0.8, "Success rate should be at least 80%")
		assert.True(t, avgResponseTime < 5*time.Second, "Average response time should be under 5 seconds")
	}

	// Throughput calculation
	if totalDuration > 0 {
		throughput := float64(successCount) / totalDuration.Seconds()
		t.Logf("  Throughput: %.2f successful requests/second", throughput)

		// Basic throughput assertion (adjust based on requirements)
		assert.True(t, throughput >= 0.5, "Throughput should be at least 0.5 requests/second")
	}
}

// Helper functions are now in helpers.go
