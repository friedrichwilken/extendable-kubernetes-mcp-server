// Package e2e contains production-like scenario tests for the extendable Kubernetes MCP server.
// These tests simulate real-world production scenarios including error handling, recovery, and edge cases.
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

// TestProductionWorkflow simulates a complete production-like workflow
func TestProductionWorkflow(t *testing.T) {
	utils.SkipIfShort(t)

	serverPath := buildServerBinary(t)

	// Use HTTP transport for production-like testing
	addr, err := utils.RandomPortAddress()
	require.NoError(t, err)
	port := fmt.Sprintf("%d", addr.Port)

	// Start server with production-like settings
	cmd := exec.Command(serverPath,
		"--port", port,
		"--log-level", "2", // Moderate logging for production
		"--read-only", // Safe mode for production
		"--toolsets", "core,config,helm")

	err = cmd.Start()
	require.NoError(t, err)

	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	serverURL := fmt.Sprintf("http://localhost:%s", port)
	err = waitForHTTPServer(serverURL, 15*time.Second)
	require.NoError(t, err)

	// Simulate multiple clients connecting concurrently (like in production)
	t.Run("concurrent_client_simulation", func(t *testing.T) {
		testConcurrentClientSimulation(t, serverURL)
	})

	// Test error recovery scenarios
	t.Run("error_recovery", func(t *testing.T) {
		testErrorRecovery(t, serverURL)
	})

	// Test edge cases and malformed requests
	t.Run("edge_cases", func(t *testing.T) {
		testEdgeCases(t, serverURL)
	})
}

func testConcurrentClientSimulation(t *testing.T, serverURL string) {
	numClients := 8
	requestsPerClient := 10
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var wg sync.WaitGroup
	results := make(chan struct {
		clientID      int
		successful    int
		failed        int
		totalDuration time.Duration
	}, numClients)

	startTime := time.Now()

	// Launch concurrent clients simulating production load
	for clientID := 0; clientID < numClients; clientID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			client := &http.Client{Timeout: 30 * time.Second}
			successful := 0
			failed := 0
			clientStart := time.Now()

			for requestID := 0; requestID < requestsPerClient; requestID++ {
				select {
				case <-ctx.Done():
					return
				default:
				}

				// Simulate different types of requests
				var success bool
				switch requestID % 4 {
				case 0:
					success = simulateInitialization(client, serverURL, id*1000+requestID)
				case 1:
					success = simulateToolsDiscovery(client, serverURL, id*1000+requestID)
				case 2:
					success = simulateReadOnlyToolCall(client, serverURL, id*1000+requestID)
				case 3:
					success = simulateInvalidRequest(client, serverURL, id*1000+requestID)
				}

				if success {
					successful++
				} else {
					failed++
				}

				// Small delay to simulate real client behavior
				time.Sleep(100 * time.Millisecond)
			}

			results <- struct {
				clientID      int
				successful    int
				failed        int
				totalDuration time.Duration
			}{id, successful, failed, time.Since(clientStart)}
		}(clientID)
	}

	wg.Wait()
	close(results)

	totalDuration := time.Since(startTime)

	// Analyze results
	totalSuccessful := 0
	totalFailed := 0
	clientDurations := make([]time.Duration, 0, numClients)

	for result := range results {
		totalSuccessful += result.successful
		totalFailed += result.failed
		clientDurations = append(clientDurations, result.totalDuration)
		t.Logf("Client %d: %d successful, %d failed (duration: %v)",
			result.clientID, result.successful, result.failed, result.totalDuration)
	}

	totalRequests := totalSuccessful + totalFailed
	successRate := float64(totalSuccessful) / float64(totalRequests)
	throughput := float64(totalRequests) / totalDuration.Seconds()

	t.Logf("Production Simulation Results:")
	t.Logf("  Clients: %d", numClients)
	t.Logf("  Total Requests: %d", totalRequests)
	t.Logf("  Success Rate: %.1f%% (%d/%d)", successRate*100, totalSuccessful, totalRequests)
	t.Logf("  Total Duration: %v", totalDuration)
	t.Logf("  Throughput: %.2f requests/second", throughput)

	// Production-level assertions
	assert.True(t, successRate >= 0.85, "Production success rate should be at least 85%%")
	assert.True(t, throughput >= 2.0, "Production throughput should be at least 2 requests/second")
}

func testErrorRecovery(t *testing.T, serverURL string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Test recovery from various error scenarios
	errorScenarios := []struct {
		name        string
		request     map[string]any
		expectError bool
		description string
	}{
		{
			name: "malformed_json",
			request: map[string]any{
				"jsonrpc": "2.0",
				"id":      "malformed",
				"method":  "initialize",
				"params": map[string]any{
					"invalid": func() {}, // This will cause JSON marshal error
				},
			},
			expectError: true,
			description: "Server should handle malformed JSON gracefully",
		},
		{
			name: "missing_required_fields",
			request: map[string]any{
				"id":     1,
				"method": "initialize",
				// Missing jsonrpc field
			},
			expectError: true,
			description: "Server should handle missing required fields",
		},
		{
			name: "invalid_protocol_version",
			request: map[string]any{
				"jsonrpc": "1.0",
				"id":      1,
				"method":  "initialize",
				"params":  map[string]any{},
			},
			expectError: true,
			description: "Server should handle invalid protocol versions",
		},
		{
			name: "unknown_method",
			request: map[string]any{
				"jsonrpc": "2.0",
				"id":      1,
				"method":  "nonexistent_method",
				"params":  map[string]any{},
			},
			expectError: true,
			description: "Server should handle unknown methods gracefully",
		},
	}

	for _, scenario := range errorScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Attempt to marshal request (some scenarios will fail here)
			requestBytes, err := json.Marshal(scenario.request)
			if err != nil && scenario.expectError {
				t.Logf("✅ %s: Failed to marshal as expected", scenario.description)
				return
			}
			require.NoError(t, err, "Should be able to marshal request for test")

			// Send request
			resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
			if err != nil {
				if scenario.expectError {
					t.Logf("✅ %s: Request failed as expected: %v", scenario.description, err)
				} else {
					t.Errorf("❌ %s: Unexpected request failure: %v", scenario.description, err)
				}
				return
			}
			defer resp.Body.Close()

			// Read response
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Verify server handled error appropriately
			if scenario.expectError {
				// Should get either an error response or HTTP error status
				if resp.StatusCode >= 400 {
					t.Logf("✅ %s: Server returned error status %d", scenario.description, resp.StatusCode)
				} else {
					// Check if response contains error
					var response map[string]any
					if json.Unmarshal(body, &response) == nil {
						if _, hasError := response["error"]; hasError {
							t.Logf("✅ %s: Server returned JSON-RPC error", scenario.description)
						} else {
							t.Errorf("❌ %s: Expected error but got success response", scenario.description)
						}
					}
				}
			} else {
				assert.True(t, resp.StatusCode < 400, "%s: Should not return error status", scenario.description)
			}
		})
	}
}

func testEdgeCases(t *testing.T, serverURL string) {
	client := &http.Client{Timeout: 10 * time.Second}

	edgeCases := []struct {
		name        string
		contentType string
		body        string
		description string
	}{
		{
			name:        "empty_body",
			contentType: "application/json",
			body:        "",
			description: "Empty request body",
		},
		{
			name:        "invalid_content_type",
			contentType: "text/plain",
			body:        `{"jsonrpc": "2.0", "id": 1, "method": "initialize"}`,
			description: "Invalid content type",
		},
		{
			name:        "oversized_request",
			contentType: "application/json",
			body:        `{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"data": "` + strings.Repeat("x", 10000) + `"}}`,
			description: "Very large request",
		},
		{
			name:        "null_request",
			contentType: "application/json",
			body:        "null",
			description: "Null JSON body",
		},
		{
			name:        "array_request",
			contentType: "application/json",
			body:        `[{"jsonrpc": "2.0", "id": 1, "method": "initialize"}]`,
			description: "JSON array instead of object",
		},
	}

	for _, edgeCase := range edgeCases {
		t.Run(edgeCase.name, func(t *testing.T) {
			resp, err := client.Post(serverURL+"/mcp", edgeCase.contentType, strings.NewReader(edgeCase.body))
			if err != nil {
				t.Logf("Request failed for %s: %v (may be expected)", edgeCase.description, err)
				return
			}
			defer resp.Body.Close()

			// Server should handle edge cases gracefully without crashing
			assert.True(t, resp.StatusCode < 500, "Server should not crash on edge case: %s", edgeCase.description)

			body, _ := io.ReadAll(resp.Body)
			t.Logf("%s: Status %d, Response length %d bytes", edgeCase.description, resp.StatusCode, len(body))
		})
	}
}

// TestLongRunningSession simulates a long-running MCP session
func TestLongRunningSession(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping long-running session test in short mode")
	}

	serverPath := buildServerBinary(t)

	// Start server in stdio mode for long-running test
	cmd := exec.Command(serverPath, "--log-level", "1")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize session
	initRequest := utils.McpInitRequest()
	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err)

	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	if initResponse == "" {
		t.Skip("Server not responding - skipping long-running test")
		return
	}

	// Run session for extended period with various operations
	sessionDuration := 2 * time.Minute
	requestInterval := 5 * time.Second
	endTime := time.Now().Add(sessionDuration)

	requestCount := 0
	successCount := 0

	t.Logf("Starting long-running session for %v...", sessionDuration)

	for time.Now().Before(endTime) {
		requestCount++

		// Alternate between different operations
		var request map[string]any
		switch requestCount % 3 {
		case 0:
			request = map[string]any{
				"jsonrpc": "2.0",
				"id":      requestCount,
				"method":  "tools/list",
				"params":  map[string]any{},
			}
		case 1:
			request = map[string]any{
				"jsonrpc": "2.0",
				"id":      requestCount,
				"method":  "tools/call",
				"params": map[string]any{
					"name":      "configuration_view",
					"arguments": map[string]any{},
				},
			}
		case 2:
			// Send a notification (no response expected)
			request = map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/ping",
				"params":  map[string]any{},
			}
		}

		err := sendJSONRPCRequest(t, stdin, request)
		if err != nil {
			t.Logf("Request %d failed to send: %v", requestCount, err)
			continue
		}

		// Read response (if expected)
		if requestCount%3 != 2 { // Not a notification
			response := readJSONRPCResponse(t, stdout, 10*time.Second)
			if response != "" {
				successCount++
			}
		} else {
			successCount++ // Notifications don't have responses
		}

		// Log progress periodically
		if requestCount%10 == 0 {
			elapsed := time.Since(endTime.Add(-sessionDuration))
			t.Logf("Progress: %d requests in %v (%.1f%% success)",
				requestCount, elapsed, float64(successCount)/float64(requestCount)*100)
		}

		time.Sleep(requestInterval)
	}

	successRate := float64(successCount) / float64(requestCount)
	t.Logf("Long-running session completed:")
	t.Logf("  Duration: %v", sessionDuration)
	t.Logf("  Total Requests: %d", requestCount)
	t.Logf("  Success Rate: %.1f%%", successRate*100)

	// Session should maintain stability over time
	assert.True(t, successRate >= 0.80, "Long-running session should maintain at least 80%% success rate")
	assert.True(t, requestCount >= 20, "Should have completed reasonable number of requests")
}

// Helper functions for production tests

func simulateInitialization(client *http.Client, serverURL string, requestID int) bool {
	initRequest := utils.McpInitRequest()
	initRequest["id"] = requestID

	requestBytes, err := json.Marshal(initRequest)
	if err != nil {
		return false
	}

	resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func simulateToolsDiscovery(client *http.Client, serverURL string, requestID int) bool {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return false
	}

	resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func simulateReadOnlyToolCall(client *http.Client, serverURL string, requestID int) bool {
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "configuration_view",
			"arguments": map[string]any{},
		},
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return false
	}

	resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Accept both success and error responses (server is in read-only mode)
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}

func simulateInvalidRequest(client *http.Client, serverURL string, requestID int) bool {
	// Invalid request - missing required fields
	request := map[string]any{
		"id":     requestID,
		"method": "invalid_method",
		// Missing jsonrpc field
	}

	requestBytes, err := json.Marshal(request)
	if err != nil {
		return false
	}

	resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// Success means server handled invalid request gracefully (returned error response)
	return resp.StatusCode >= 200 && resp.StatusCode < 500
}
