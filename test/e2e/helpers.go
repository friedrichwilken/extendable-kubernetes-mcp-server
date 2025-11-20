// Package e2e contains shared helper functions for end-to-end tests.
package e2e

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Helper functions shared across E2E tests

// findProjectRoot searches for the project root by looking for go.mod file
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree looking for go.mod
	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}

	return "", os.ErrNotExist
}

// tempDir creates a temporary directory for testing, works with both *testing.T and *testing.B
func tempDir(tb testing.TB) string {
	dir, err := os.MkdirTemp("", "ek8sms-e2e-test-")
	require.NoError(tb, err)

	// Register cleanup
	switch v := tb.(type) {
	case *testing.T:
		v.Cleanup(func() { _ = os.RemoveAll(dir) })
	case *testing.B:
		v.Cleanup(func() { _ = os.RemoveAll(dir) })
	}

	return dir
}

func buildServerBinary(tb testing.TB) string {
	tempDirectory := tempDir(tb)
	serverPath := tempDirectory + "/test-e2e-server"

	// Find project root by looking for go.mod file
	projectRoot, err := findProjectRoot()
	require.NoError(tb, err, "Failed to find project root")

	buildCmd := exec.Command("go", "build", "-o", serverPath, "./cmd") // #nosec G204 -- controlled paths for testing
	buildCmd.Dir = projectRoot

	output, err := buildCmd.CombinedOutput()
	require.NoError(tb, err, "Failed to build server binary: %s", string(output))

	return serverPath
}

func startServerWithPipes(tb testing.TB, cmd *exec.Cmd) (stdin io.WriteCloser, stdout, stderr io.ReadCloser) {
	var err error
	stdin, err = cmd.StdinPipe()
	require.NoError(tb, err, "Failed to create stdin pipe")

	stdout, err = cmd.StdoutPipe()
	require.NoError(tb, err, "Failed to create stdout pipe")

	stderr, err = cmd.StderrPipe()
	require.NoError(tb, err, "Failed to create stderr pipe")

	err = cmd.Start()
	require.NoError(tb, err, "Failed to start server")

	// Wait for server to start and validate it's still running
	// This helps catch early exits that cause "broken pipe" errors
	time.Sleep(500 * time.Millisecond)

	// Check if the process is still alive by sending it signal 0
	// This works cross-platform and doesn't actually send a signal
	if cmd.Process != nil {
		if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
			tb.Logf("Server process is not running, it may have exited early: %v", err)
			// Don't fail immediately, let the test try to continue
			// The actual communication failure will be caught in the test
		}
	}

	return stdin, stdout, stderr
}

// createTestKubeconfig creates a test kubeconfig file for CI testing
func createTestKubeconfig(tb testing.TB, tempDir string, clusters map[string]string, currentContext string) string {
	kubeconfigPath := filepath.Join(tempDir, "kubeconfig")

	// Build clusters section
	clustersYAML := []string{}
	contextsYAML := []string{}
	usersYAML := []string{}

	for name, server := range clusters {
		clustersYAML = append(clustersYAML, fmt.Sprintf(`- cluster:
    server: %s
    insecure-skip-tls-verify: true
  name: %s`, server, name))

		contextsYAML = append(contextsYAML, fmt.Sprintf(`- context:
    cluster: %s
    user: %s-user
  name: %s`, name, name, name))

		usersYAML = append(usersYAML, fmt.Sprintf(`- name: %s-user
  user:
    token: test-token-%s`, name, name))
	}

	kubeconfigContent := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
%s
contexts:
%s
current-context: %s
users:
%s
`, strings.Join(clustersYAML, "\n"),
		strings.Join(contextsYAML, "\n"),
		currentContext,
		strings.Join(usersYAML, "\n"))

	err := os.WriteFile(kubeconfigPath, []byte(kubeconfigContent), 0o600)
	if err != nil {
		tb.Fatalf("Failed to create test kubeconfig: %v", err)
	}

	return kubeconfigPath
}

func sendJSONRPCRequest(_ testing.TB, writer io.Writer, request map[string]any) error {
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return err
	}

	_, err = writer.Write(append(requestBytes, '\n'))
	return err
}

func readJSONRPCResponse(tb testing.TB, reader io.Reader, timeout time.Duration) string {
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
		tb.Logf("Error reading response: %v", err)
		return ""
	case <-time.After(timeout):
		return ""
	}
}
