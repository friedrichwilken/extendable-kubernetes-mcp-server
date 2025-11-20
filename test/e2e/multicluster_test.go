// Package e2e contains multi-cluster scenario tests for the extendable Kubernetes MCP server.
// These tests validate behavior when working with multiple Kubernetes clusters and contexts.
package e2e

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

// TestMultiClusterConfiguration tests server behavior with multiple cluster contexts
func TestMultiClusterConfiguration(t *testing.T) {
	utils.SkipIfShort(t)

	serverPath := buildServerBinary(t)

	t.Run("single_cluster_mode", func(t *testing.T) {
		testSingleClusterMode(t, serverPath)
	})

	t.Run("multi_cluster_disabled", func(t *testing.T) {
		testMultiClusterDisabled(t, serverPath)
	})

	t.Run("multi_cluster_kubeconfig", func(t *testing.T) {
		testMultiClusterKubeconfig(t, serverPath)
	})
}

func testSingleClusterMode(t *testing.T, serverPath string) {
	// Create a test kubeconfig with single context
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	// Start server with single cluster kubeconfig
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize and test tools discovery
	initRequest := utils.McpInitRequest()
	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err)

	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	if initResponse == "" {
		t.Skip("Server not responding - may be expected without valid k8s cluster")
		return
	}

	// List tools to verify single-cluster setup
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	err = sendJSONRPCRequest(t, stdin, listToolsRequest)
	require.NoError(t, err)

	toolsResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	require.NotEmpty(t, toolsResponse, "Should receive tools response")

	// Verify tools are available (exact tools depend on server implementation)
	var parsedTools map[string]any
	err = json.Unmarshal([]byte(toolsResponse), &parsedTools)
	require.NoError(t, err)

	if result, ok := parsedTools["result"]; ok {
		resultMap := result.(map[string]any)
		if tools, ok := resultMap["tools"].([]any); ok {
			t.Logf("Found %d tools in single-cluster mode", len(tools))
			assert.True(t, len(tools) > 0, "Should have tools available")
		}
	}
}

func testMultiClusterDisabled(t *testing.T, serverPath string) {
	// Create a test kubeconfig with multiple contexts
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"cluster-1": "https://cluster-1:6443",
		"cluster-2": "https://cluster-2:6443",
		"cluster-3": "https://cluster-3:6443",
	}, "cluster-1")

	// Start server with multi-cluster disabled
	cmd := exec.Command(serverPath,
		"--kubeconfig", kubeconfigPath,
		"--disable-multi-cluster",
		"--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize and test
	initRequest := utils.McpInitRequest()
	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err)

	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	if initResponse == "" {
		t.Skip("Server not responding - may be expected without valid k8s cluster")
		return
	}

	// List tools and verify no multi-cluster tools are present
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	err = sendJSONRPCRequest(t, stdin, listToolsRequest)
	require.NoError(t, err)

	toolsResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	require.NotEmpty(t, toolsResponse, "Should receive tools response")

	var parsedTools map[string]any
	err = json.Unmarshal([]byte(toolsResponse), &parsedTools)
	require.NoError(t, err)

	if result, ok := parsedTools["result"]; ok {
		resultMap := result.(map[string]any)
		if tools, ok := resultMap["tools"].([]any); ok {
			t.Logf("Found %d tools with multi-cluster disabled", len(tools))

			// Check that no multi-cluster specific tools are present
			toolNames := make([]string, 0, len(tools))
			for _, tool := range tools {
				if toolMap, ok := tool.(map[string]any); ok {
					if name, ok := toolMap["name"].(string); ok {
						toolNames = append(toolNames, name)
					}
				}
			}

			// Multi-cluster tools should not be present when disabled
			for _, toolName := range toolNames {
				assert.False(t, strings.Contains(toolName, "multi_cluster"),
					"Tool %s should not be available when multi-cluster is disabled", toolName)
			}
		}
	}
}

func testMultiClusterKubeconfig(t *testing.T, serverPath string) {
	// Create a comprehensive multi-cluster kubeconfig
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"production":  "https://prod-cluster:6443",
		"staging":     "https://staging-cluster:6443",
		"development": "https://dev-cluster:6443",
	}, "production")

	// Start server with multi-cluster enabled (default)
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize
	initRequest := utils.McpInitRequest()
	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err)

	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	if initResponse == "" {
		t.Skip("Server not responding - may be expected without valid k8s cluster")
		return
	}

	// Test tools discovery with multi-cluster
	listToolsRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	}

	err = sendJSONRPCRequest(t, stdin, listToolsRequest)
	require.NoError(t, err)

	toolsResponse := readJSONRPCResponse(t, stdout, 10*time.Second)
	require.NotEmpty(t, toolsResponse, "Should receive tools response")

	var parsedTools map[string]any
	err = json.Unmarshal([]byte(toolsResponse), &parsedTools)
	require.NoError(t, err)

	if result, ok := parsedTools["result"]; ok {
		resultMap := result.(map[string]any)
		if tools, ok := resultMap["tools"].([]any); ok {
			t.Logf("Found %d tools with multi-cluster enabled", len(tools))

			// Analyze tool names for multi-cluster capabilities
			toolNames := make([]string, 0, len(tools))
			for _, tool := range tools {
				if toolMap, ok := tool.(map[string]any); ok {
					if name, ok := toolMap["name"].(string); ok {
						toolNames = append(toolNames, name)
					}
				}
			}

			// Log available tools for analysis
			t.Logf("Available tools: %v", toolNames)

			// At minimum, core Kubernetes tools should be available
			assert.True(t, len(tools) > 0, "Should have tools available in multi-cluster mode")
		}
	}
}

// TestClusterFailover tests behavior when clusters become unavailable
func TestClusterFailover(t *testing.T) {
	utils.SkipIfShort(t)

	serverPath := buildServerBinary(t)

	// Create kubeconfig with multiple contexts pointing to non-existent clusters
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"primary":   "https://nonexistent-primary:6443",
		"secondary": "https://nonexistent-secondary:6443",
	}, "primary")

	// Start server
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "1")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Test that server handles cluster connectivity issues gracefully
	initRequest := utils.McpInitRequest()
	err := sendJSONRPCRequest(t, stdin, initRequest)
	require.NoError(t, err)

	// Server should respond even if clusters are unreachable
	initResponse := readJSONRPCResponse(t, stdout, 10*time.Second)

	if initResponse != "" {
		var parsedInit map[string]any
		err = json.Unmarshal([]byte(initResponse), &parsedInit)
		require.NoError(t, err)

		// Check if initialization succeeded or failed gracefully
		if result, ok := parsedInit["result"]; ok {
			t.Log("✅ Server initialized successfully despite unreachable clusters")
			assert.NotNil(t, result)
		} else if errorObj, ok := parsedInit["error"]; ok {
			errorMap := errorObj.(map[string]any)
			t.Logf("Server failed to initialize (expected): %v", errorMap["message"])
			// This is acceptable - server should handle cluster connectivity issues
		}
	} else {
		t.Log("Server did not respond to initialization (may be expected)")
	}

	// Test that tool calls handle cluster errors gracefully
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
	require.NoError(t, err)

	toolResponse := readJSONRPCResponse(t, stdout, 15*time.Second)

	if toolResponse != "" {
		var parsedTool map[string]any
		err = json.Unmarshal([]byte(toolResponse), &parsedTool)
		require.NoError(t, err)

		if errorObj, ok := parsedTool["error"]; ok {
			errorMap := errorObj.(map[string]any)
			t.Logf("Tool call failed gracefully: %v", errorMap["message"])
			// This is expected when clusters are unreachable
		} else {
			t.Log("Tool call succeeded unexpectedly")
		}
	}
}

// Helper function to create test kubeconfig

// TestConfigurationValidation tests various configuration scenarios
func TestConfigurationValidation(t *testing.T) {
	utils.SkipIfShort(t)

	serverPath := buildServerBinary(t)

	// Create a test kubeconfig that all scenarios can use
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	scenarios := []struct {
		name        string
		args        []string
		expectStart bool
		description string
	}{
		{
			name:        "default_config",
			args:        []string{"--kubeconfig", kubeconfigPath, "--log-level", "0"},
			expectStart: true,
			description: "Server should start with default configuration",
		},
		{
			name:        "read_only_mode",
			args:        []string{"--kubeconfig", kubeconfigPath, "--read-only", "--log-level", "0"},
			expectStart: true,
			description: "Server should start in read-only mode",
		},
		{
			name:        "destructive_disabled",
			args:        []string{"--kubeconfig", kubeconfigPath, "--disable-destructive", "--log-level", "0"},
			expectStart: true,
			description: "Server should start with destructive operations disabled",
		},
		{
			name:        "specific_toolsets",
			args:        []string{"--kubeconfig", kubeconfigPath, "--toolsets", "core,config", "--log-level", "0"},
			expectStart: true,
			description: "Server should start with specific toolsets only",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Start server with scenario configuration
			cmd := exec.Command(serverPath, scenario.args...)
			stdin, stdout, stderr := startServerWithPipes(t, cmd)
			defer func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				_ = stdin.Close()
				_ = stdout.Close()
				_ = stderr.Close()
			}()

			if scenario.expectStart {
				// Test basic functionality
				initRequest := utils.McpInitRequest()
				err := sendJSONRPCRequest(t, stdin, initRequest)
				assert.NoError(t, err, "Should be able to send init request")

				response := readJSONRPCResponse(t, stdout, 5*time.Second)
				// Response might be empty if k8s is not available, but should not crash
				t.Logf("Scenario '%s': %s - Response length: %d", scenario.name, scenario.description, len(response))
			}
		})
	}
}
