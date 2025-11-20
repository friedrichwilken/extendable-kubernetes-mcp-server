// Package e2e contains shared helper functions for end-to-end tests.
package e2e

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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

	// Give server time to start
	time.Sleep(200 * time.Millisecond)

	return stdin, stdout, stderr
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
