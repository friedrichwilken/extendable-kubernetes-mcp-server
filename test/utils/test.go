// Package utils provides common testing utilities for the extendable Kubernetes MCP server.
// These utilities are adapted from kubernetes-mcp-server test patterns while being updated
// for the newer MCP SDK and our specific testing needs.
package utils

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Must is a helper function that panics if an error is not nil.
// Useful for test setup where failures should immediately fail the test.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// ReadFile reads a file relative to the caller's location.
// This is useful for loading test fixtures and test data files.
func ReadFile(path ...string) string {
	_, file, _, _ := runtime.Caller(1)
	filePath := filepath.Join(append([]string{filepath.Dir(file)}, path...)...)
	fileBytes := Must(os.ReadFile(filePath))
	return string(fileBytes)
}

// RandomPortAddress finds a random available TCP port.
// Returns a TCPAddr that can be used for test servers.
func RandomPortAddress() (*net.TCPAddr, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find random port for HTTP server: %v", err)
	}
	defer func() { _ = ln.Close() }()
	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("failed to cast listener address to TCPAddr")
	}
	return tcpAddr, nil
}

// WaitForServer waits for a server to become available at the given address.
// Useful for integration tests that need to wait for server startup.
func WaitForServer(tcpAddr *net.TCPAddr) error {
	var conn *net.TCPConn
	var err error
	for i := 0; i < 10; i++ {
		conn, err = net.DialTCP("tcp", nil, tcpAddr)
		if err == nil {
			_ = conn.Close()
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return err
}

// SkipIfShort skips the test if running in short mode.
// Use this for longer-running integration and e2e tests.
func SkipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}
}

// TempDir creates a temporary directory for the test.
// The directory will be automatically cleaned up when the test completes.
func TempDir(t *testing.T) string {
	dir := Must(os.MkdirTemp("", "ek8sms-test-"))
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

// WriteTestFile writes content to a file in the given directory.
// Returns the full path to the created file.
func WriteTestFile(t *testing.T, dir, filename, content string) string {
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("Failed to write test file %s: %v", path, err)
	}
	return path
}
