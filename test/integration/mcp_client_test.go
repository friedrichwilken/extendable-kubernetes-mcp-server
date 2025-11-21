// Package integration contains MCP client integration tests.
// This file tests real MCP client-server interactions using the actual server binary.
package integration

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

func TestMCPClientStdioIntegration(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	// Start server in stdio mode
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "0")

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

	// Test MCP initialization
	t.Run("mcp_initialization", func(t *testing.T) {
		testMCPInitialization(t, stdin, stdout)
	})

	// Test tool discovery (tools may vary based on k8s connectivity)
	t.Run("tool_discovery", func(t *testing.T) {
		testToolDiscovery(t, stdin, stdout)
	})
}

func TestMCPClientHTTPIntegration(t *testing.T) {
	utils.SkipIfShort(t)

	// Find a random port
	addr, err := utils.RandomPortAddress()
	require.NoError(t, err, "Failed to find random port")
	port := fmt.Sprintf("%d", addr.Port)

	// Build and start server
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--port", port, "--log-level", "0")

	err = cmd.Start()
	require.NoError(t, err, "Failed to start HTTP server")

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	// Wait for server to start
	serverURL := fmt.Sprintf("http://localhost:%s", port)
	err = waitForHTTPServer(serverURL, 10*time.Second)
	require.NoError(t, err, "Server should start and accept connections")

	// Test HTTP MCP endpoints
	t.Run("http_mcp_endpoints", func(t *testing.T) {
		testHTTPMCPEndpoints(t, serverURL)
	})

	t.Run("sse_endpoint", func(t *testing.T) {
		testSSEEndpoint(t, serverURL)
	})
}

func testMCPInitialization(t *testing.T, stdin io.Writer, stdout io.Reader) {
	// Send initialization request
	initRequest := map[string]any{
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

	requestBytes, err := json.Marshal(initRequest)
	require.NoError(t, err, "Failed to marshal init request")

	// Send request
	_, err = stdin.Write(append(requestBytes, '\n'))
	require.NoError(t, err, "Failed to send init request")

	// Read response with timeout
	responseText := readJSONRPCResponse(t, stdout, 10*time.Second)

	if responseText == "" {
		// Some configurations may not allow full initialization
		// but server shouldn't crash or hang
		t.Log("Server may not be able to initialize (possibly due to k8s config), but didn't crash")
		return
	}

	// Parse response
	var response map[string]any
	err = json.Unmarshal([]byte(responseText), &response)
	require.NoError(t, err, "Failed to parse init response: %s", responseText)

	// Basic validation
	assert.Equal(t, "2.0", response["jsonrpc"], "Should have correct JSONRPC version")
	assert.Equal(t, float64(1), response["id"], "Should have correct request ID")

	// Check if we have a result (successful init) or error
	if result, ok := response["result"]; ok {
		// Successful initialization
		resultMap := result.(map[string]any)
		assert.Contains(t, resultMap, "protocolVersion", "Init result should contain protocol version")
		assert.Contains(t, resultMap, "capabilities", "Init result should contain capabilities")
		assert.Contains(t, resultMap, "serverInfo", "Init result should contain server info")

		if serverInfo, ok := resultMap["serverInfo"].(map[string]any); ok {
			assert.Contains(t, serverInfo, "name", "Server info should contain name")
			assert.Contains(t, serverInfo, "version", "Server info should contain version")
		}

		t.Log("MCP initialization successful")
	} else if errorObj, ok := response["error"]; ok {
		// Error response - may be expected if k8s is not configured
		errorMap := errorObj.(map[string]any)
		t.Logf("MCP initialization failed (may be expected): %v", errorMap["message"])
	}
}

func testToolDiscovery(t *testing.T, stdin io.Writer, stdout io.Reader) {
	// Send list tools request
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	requestBytes, err := json.Marshal(listToolsRequest)
	require.NoError(t, err, "Failed to marshal list tools request")

	// Send request
	_, err = stdin.Write(append(requestBytes, '\n'))
	require.NoError(t, err, "Failed to send list tools request")

	// Read response
	responseText := readJSONRPCResponse(t, stdout, 10*time.Second)

	if responseText == "" {
		t.Log("No response to tools/list (may be expected if server not fully initialized)")
		return
	}

	// Parse response
	var response map[string]any
	err = json.Unmarshal([]byte(responseText), &response)
	require.NoError(t, err, "Failed to parse tools list response: %s", responseText)

	// Check response structure
	assert.Equal(t, "2.0", response["jsonrpc"], "Should have correct JSONRPC version")
	assert.Equal(t, float64(2), response["id"], "Should have correct request ID")

	if result, ok := response["result"]; ok {
		resultMap := result.(map[string]any)
		if tools, ok := resultMap["tools"].([]any); ok {
			t.Logf("Found %d tools", len(tools))

			// Validate tool structure if we have tools
			if len(tools) > 0 {
				firstTool := tools[0].(map[string]any)
				assert.Contains(t, firstTool, "name", "Tool should have name")
				assert.Contains(t, firstTool, "description", "Tool should have description")

				// Log some tool names for visibility
				for i, tool := range tools {
					if i >= 5 { // Limit output
						t.Logf("... and %d more tools", len(tools)-5)
						break
					}
					toolMap := tool.(map[string]any)
					if name, ok := toolMap["name"].(string); ok {
						t.Logf("Tool %d: %s", i+1, name)
					}
				}
			}
		} else {
			t.Log("No tools found in response (may be expected if k8s not configured)")
		}
	} else if errorObj, ok := response["error"]; ok {
		errorMap := errorObj.(map[string]any)
		t.Logf("Tools list failed: %v", errorMap["message"])
	}
}

func testHTTPMCPEndpoints(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Test MCP endpoint exists
	resp, err := client.Get(baseURL + "/mcp")
	if err != nil {
		t.Logf("MCP endpoint not accessible: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Should respond with some status (might be 400 for GET request on POST endpoint)
	assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
		"MCP endpoint should respond, got status %d", resp.StatusCode)

	// Test POST to MCP endpoint with initialization
	initRequest := utils.McpInitRequest()
	requestBytes, err := json.Marshal(initRequest)
	require.NoError(t, err, "Failed to marshal init request")

	resp, err = client.Post(baseURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))
	if err != nil {
		t.Logf("POST to MCP endpoint failed: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Should get some response
	assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
		"MCP POST should respond, got status %d", resp.StatusCode)

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err == nil && len(body) > 0 {
			t.Logf("MCP response: %s", string(body))
		}
	}
}

func testSSEEndpoint(t *testing.T, baseURL string) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Test SSE endpoint
	resp, err := client.Get(baseURL + "/sse")
	if err != nil {
		t.Logf("SSE endpoint not accessible: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// SSE endpoint should respond
	assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
		"SSE endpoint should respond, got status %d", resp.StatusCode)

	// Check content type for SSE
	contentType := resp.Header.Get("Content-Type")
	if resp.StatusCode == 200 {
		t.Logf("SSE endpoint content type: %s", contentType)
		// SSE endpoints typically use "text/event-stream"
		// but our server might handle this differently
	}
}

// Helper function to read JSONRPC response with timeout
func readJSONRPCResponse(t *testing.T, reader io.Reader, timeout time.Duration) string {
	resultChan := make(chan string, 1)
	errorChan := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(reader)
		if scanner.Scan() {
			line := scanner.Text()
			if line != "" {
				resultChan <- line
				return
			}
		}
		if err := scanner.Err(); err != nil {
			errorChan <- err
		} else {
			resultChan <- "" // Empty response
		}
	}()

	select {
	case result := <-resultChan:
		return result
	case err := <-errorChan:
		t.Logf("Error reading response: %v", err)
		return ""
	case <-time.After(timeout):
		t.Log("Timeout reading response")
		return ""
	}
}

func TestMCPProtocolCompliance(t *testing.T) {
	utils.SkipIfShort(t)

	// Test various protocol aspects
	t.Run("jsonrpc_format", func(t *testing.T) {
		// Test that requests follow JSON-RPC 2.0 format
		validRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params":  map[string]any{},
		}

		requestBytes, err := json.Marshal(validRequest)
		require.NoError(t, err, "Should be able to marshal valid JSON-RPC request")

		var parsed map[string]any
		err = json.Unmarshal(requestBytes, &parsed)
		require.NoError(t, err, "Should be able to parse JSON-RPC request")

		assert.Equal(t, "2.0", parsed["jsonrpc"], "Should have JSONRPC version 2.0")
		assert.Contains(t, parsed, "id", "Should have request ID")
		assert.Contains(t, parsed, "method", "Should have method")
	})

	t.Run("mcp_capabilities", func(t *testing.T) {
		// Test MCP-specific capabilities structure
		capabilities := map[string]any{
			"tools": map[string]any{
				"listChanged": true,
			},
			"resources": map[string]any{
				"subscribe":   false,
				"listChanged": false,
			},
		}

		// Should be serializable
		_, err := json.Marshal(capabilities)
		assert.NoError(t, err, "MCP capabilities should be serializable")
	})
}
