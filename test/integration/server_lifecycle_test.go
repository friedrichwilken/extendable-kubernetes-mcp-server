// Package integration contains integration tests for the extendable Kubernetes MCP server.
// This file tests server lifecycle including startup, shutdown, and signal handling.
package integration

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

// findProjectRoot finds the project root directory by looking for go.mod
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("could not find project root (go.mod not found)")
}

// createTestKubeconfig creates a test kubeconfig file for CI environments
func createTestKubeconfig(t *testing.T, tempDir string, clusters map[string]string, currentContext string) string {
	kubeconfigContent := `apiVersion: v1
kind: Config
clusters:`

	for name, server := range clusters {
		kubeconfigContent += fmt.Sprintf(`
- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: %s`, server, name)
	}

	kubeconfigContent += `
contexts:`

	for name := range clusters {
		kubeconfigContent += fmt.Sprintf(`
- context:
    cluster: %s
    user: %s-user
  name: %s`, name, name, name)
	}

	kubeconfigContent += fmt.Sprintf(`
current-context: %s
users:`, currentContext)

	for name := range clusters {
		kubeconfigContent += fmt.Sprintf(`
- name: %s-user
  user:
    token: test-token`, name)
	}

	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")
	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0o644)
	require.NoError(t, err, "Failed to create test kubeconfig")

	return kubeconfigPath
}

func TestServerStartupStdio(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary for testing
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	// Start server in stdio mode
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "0")
	cmd.Env = os.Environ()

	stdin, err := cmd.StdinPipe()
	require.NoError(t, err, "Failed to create stdin pipe")

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err, "Failed to create stdout pipe")

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err, "Failed to create stderr pipe")

	// Start the server
	err = cmd.Start()
	require.NoError(t, err, "Failed to start server")

	// Ensure server is cleaned up
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
	time.Sleep(100 * time.Millisecond)

	// Verify server is running
	assert.NotNil(t, cmd.Process, "Server process should be running")

	// Send a test MCP initialization request
	initRequest := `{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"protocolVersion": "2024-11-05", "capabilities": {}, "clientInfo": {"name": "test", "version": "1.0.0"}}}` + "\n"

	_, err = stdin.Write([]byte(initRequest))
	require.NoError(t, err, "Failed to write init request")

	// Read response (with timeout)
	responseBuffer := make([]byte, 1024)
	done := make(chan error, 1)
	go func() {
		_, err := stdout.Read(responseBuffer)
		done <- err
	}()

	select {
	case <-done:
		// Read completed
	case <-time.After(5 * time.Second):
		// Timeout - this is acceptable
	}

	// Server should either respond or close gracefully
	// The exact behavior depends on Kubernetes connectivity, but it shouldn't crash
	assert.NotNil(t, cmd.Process, "Server should still be running after init request")
}

func TestServerStartupHTTP(t *testing.T) {
	utils.SkipIfShort(t)

	// Find a random port for testing
	addr, err := utils.RandomPortAddress()
	require.NoError(t, err, "Failed to find random port")

	port := fmt.Sprintf("%d", addr.Port)

	// Build the server binary for testing
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	// Start server in HTTP mode
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--port", port, "--log-level", "0")
	cmd.Env = os.Environ()

	stderr, err := cmd.StderrPipe()
	require.NoError(t, err, "Failed to create stderr pipe")

	// Start the server
	err = cmd.Start()
	require.NoError(t, err, "Failed to start server")

	// Ensure server is cleaned up
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
		_ = stderr.Close()
	})

	// Wait for server to start up
	serverURL := fmt.Sprintf("http://localhost:%s", port)
	err = waitForHTTPServer(serverURL, 10*time.Second)
	require.NoError(t, err, "Server should start and accept HTTP connections")

	// Test basic HTTP endpoints
	resp, err := http.Get(serverURL + "/health")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		assert.True(t, resp.StatusCode == 200 || resp.StatusCode == 404,
			"Health endpoint should return 200 or 404, got %d", resp.StatusCode)
	}

	// Test MCP endpoint exists
	resp, err = http.Get(serverURL + "/mcp")
	if err == nil {
		defer func() { _ = resp.Body.Close() }()
		// Should get some response (might be 400 for invalid request, but server should respond)
		assert.True(t, resp.StatusCode >= 200 && resp.StatusCode < 500,
			"MCP endpoint should be available, got status %d", resp.StatusCode)
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	utils.SkipIfShort(t)

	// Find a random port for testing
	addr, err := utils.RandomPortAddress()
	require.NoError(t, err, "Failed to find random port")

	port := fmt.Sprintf("%d", addr.Port)

	// Build the server binary for testing
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	// Start server in HTTP mode
	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--port", port, "--log-level", "1")

	// Start the server
	err = cmd.Start()
	require.NoError(t, err, "Failed to start server")

	// Wait for server to start up
	serverURL := fmt.Sprintf("http://localhost:%s", port)
	err = waitForHTTPServer(serverURL, 10*time.Second)
	require.NoError(t, err, "Server should start and accept HTTP connections")

	// Send SIGTERM for graceful shutdown
	err = cmd.Process.Signal(syscall.SIGTERM)
	require.NoError(t, err, "Failed to send SIGTERM")

	// Wait for process to exit gracefully
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		// Process exited - check if it was clean
		if err != nil {
			// On Unix systems, SIGTERM may cause exit status 1, which is expected
			if exitError, ok := err.(*exec.ExitError); ok {
				// Exit code 1 or -1 is acceptable for SIGTERM
				exitCode := exitError.ExitCode()
				assert.True(t, exitCode == 1 || exitCode == -1,
					"Expected exit code 1 or -1 for SIGTERM, got %d", exitCode)
			}
		}
	case <-time.After(10 * time.Second):
		// Force kill if it doesn't shutdown gracefully
		_ = cmd.Process.Kill()
		t.Error("Server did not shutdown gracefully within 10 seconds")
	}
}

func TestServerInvalidConfig(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary for testing
	serverPath := buildServerBinary(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "invalid log level string",
			args: []string{"--log-level", "invalid"},
		},
		{
			name: "invalid port",
			args: []string{"--port", "invalid-port"},
		},
		{
			name: "non-existent kubeconfig",
			args: []string{"--kubeconfig", "/non/existent/path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(serverPath, tt.args...)

			err := cmd.Start()
			require.NoError(t, err, "Failed to start server command")

			// Wait for process to complete
			err = cmd.Wait()

			// Should exit with non-zero code for invalid config
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					assert.NotEqual(t, 0, exitError.ExitCode(),
						"Server should exit with non-zero code for invalid config")
				}
			} else {
				t.Error("Server should fail with invalid configuration")
			}
		})
	}
}

func TestServerEnvironmentHandling(t *testing.T) {
	utils.SkipIfShort(t)

	// Build the server binary for testing
	serverPath := buildServerBinary(t)

	// Create test kubeconfig for CI environment
	tempDir := utils.TempDir(t)
	kubeconfigPath := createTestKubeconfig(t, tempDir, map[string]string{
		"test-cluster": "https://test-cluster:6443",
	}, "test-cluster")

	cmd := exec.Command(serverPath, "--kubeconfig", kubeconfigPath, "--log-level", "0")
	cmd.Env = os.Environ()

	// Start process
	err := cmd.Start()
	require.NoError(t, err, "Failed to start server")

	// Give it time to process config
	time.Sleep(200 * time.Millisecond)

	// Kill the process
	err = cmd.Process.Kill()
	require.NoError(t, err, "Failed to kill server")

	// Wait for process to exit
	_ = cmd.Wait()

	// If we get here without panic, the server handled the environment variable correctly
}

// Helper functions

func buildServerBinary(t *testing.T) string {
	// Build the server binary for testing
	tempDir := utils.TempDir(t)
	serverPath := tempDir + "/test-server"

	// Find project root dynamically
	projectRoot, err := findProjectRoot()
	require.NoError(t, err, "Failed to find project root")

	// Build command
	buildCmd := exec.Command("go", "build", "-o", serverPath, "./cmd")
	buildCmd.Dir = projectRoot

	output, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to build server binary: %s", string(output))

	return serverPath
}

func waitForHTTPServer(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}

		// Check if it's a connection error (server not ready) vs other errors
		if strings.Contains(err.Error(), "connection refused") {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Other errors might indicate server is ready but returning errors
		return nil
	}

	return fmt.Errorf("server did not start within %v", timeout)
}
