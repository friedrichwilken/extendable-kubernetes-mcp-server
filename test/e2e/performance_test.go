// Package e2e contains performance benchmark tests for the extendable Kubernetes MCP server.
// These tests measure throughput, latency, memory usage, and concurrent client handling.
package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

// BenchmarkMCPInitialization benchmarks the MCP initialization process
func BenchmarkMCPInitialization(b *testing.B) {
	serverPath := buildServerBinary(b)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Start server
		cmd := exec.Command(serverPath, "--log-level", "0")
		stdin, stdout, stderr := startServerWithPipes(b, cmd)

		// Initialize
		initRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "initialize",
			"params": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"clientInfo": map[string]any{
					"name":    "benchmark-client",
					"version": "1.0.0",
				},
			},
		}

		startTime := time.Now()
		_ = sendJSONRPCRequest(b, stdin, initRequest)
		_ = readJSONRPCResponse(b, stdout, 5*time.Second)

		b.ReportMetric(float64(time.Since(startTime).Nanoseconds()), "ns/init")

		// Cleanup
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}
}

// BenchmarkToolsList benchmarks the tools/list operation
func BenchmarkToolsList(b *testing.B) {
	serverPath := buildServerBinary(b)

	// Start server once for all benchmark iterations
	cmd := exec.Command(serverPath, "--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(b, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize server
	initRequest := map[string]any{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "benchmark-client",
				"version": "1.0.0",
			},
		},
	}
	_ = sendJSONRPCRequest(b, stdin, initRequest)
	_ = readJSONRPCResponse(b, stdout, 5*time.Second)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		listToolsRequest := map[string]any{
			"jsonrpc": "2.0",
			"id":      i + 1,
			"method":  "tools/list",
			"params":  map[string]any{},
		}

		startTime := time.Now()
		_ = sendJSONRPCRequest(b, stdin, listToolsRequest)
		response := readJSONRPCResponse(b, stdout, 5*time.Second)

		duration := time.Since(startTime)
		b.ReportMetric(float64(duration.Nanoseconds()), "ns/tools-list")

		// Verify we got a valid response
		if response != "" {
			var parsed map[string]any
			if json.Unmarshal([]byte(response), &parsed) == nil {
				if result, ok := parsed["result"]; ok {
					if resultMap, ok := result.(map[string]any); ok {
						if tools, ok := resultMap["tools"].([]any); ok {
							b.ReportMetric(float64(len(tools)), "tools/response")
						}
					}
				}
			}
		}
	}
}

// TestHighConcurrencyLoad tests server behavior under high concurrent load
func TestHighConcurrencyLoad(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping high concurrency test in short mode")
	}

	serverPath := buildServerBinary(t)

	concurrency := []int{10, 25, 50}

	for _, numClients := range concurrency {
		t.Run(fmt.Sprintf("concurrent_clients_%d", numClients), func(t *testing.T) {
			testConcurrentClientLoad(t, serverPath, numClients)
		})
	}
}

func testConcurrentClientLoad(t *testing.T, serverPath string, numClients int) {
	// Use HTTP transport for easier concurrent testing
	addr, err := utils.RandomPortAddress()
	require.NoError(t, err)
	port := fmt.Sprintf("%d", addr.Port)

	// Start HTTP server
	cmd := exec.Command(serverPath, "--port", port, "--log-level", "0")
	err = cmd.Start()
	require.NoError(t, err)

	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	// Wait for server to start
	serverURL := fmt.Sprintf("http://localhost:%s", port)
	err = waitForHTTPServer(serverURL, 10*time.Second)
	require.NoError(t, err)

	// Performance metrics
	var wg sync.WaitGroup
	results := make(chan struct {
		clientID int
		success  bool
		duration time.Duration
		requests int
	}, numClients)

	startTime := time.Now()

	// Launch concurrent clients
	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			clientStartTime := time.Now()
			requestCount := 0
			successCount := 0

			// Each client makes multiple requests
			for j := 0; j < 5; j++ {
				requestCount++

				// Test MCP initialization via HTTP POST
				initRequest := utils.McpInitRequest()
				initRequest["id"] = clientID*100 + j

				requestBytes, _ := json.Marshal(initRequest)

				client := &http.Client{Timeout: 10 * time.Second}
				resp, err := client.Post(serverURL+"/mcp", "application/json", strings.NewReader(string(requestBytes)))

				if err == nil {
					_ = resp.Body.Close()
					if resp.StatusCode >= 200 && resp.StatusCode < 500 {
						successCount++
					}
				}
			}

			results <- struct {
				clientID int
				success  bool
				duration time.Duration
				requests int
			}{
				clientID: clientID,
				success:  successCount > 0,
				duration: time.Since(clientStartTime),
				requests: requestCount,
			}
		}(i)
	}

	wg.Wait()
	close(results)

	totalDuration := time.Since(startTime)

	// Collect and analyze results
	totalRequests := 0
	successfulClients := 0

	for result := range results {
		totalRequests += result.requests
		if result.success {
			successfulClients++
		}
		t.Logf("Client %d: %d requests in %v (success: %v)",
			result.clientID, result.requests, result.duration, result.success)
	}

	// Performance analysis
	successRate := float64(successfulClients) / float64(numClients)
	throughput := float64(totalRequests) / totalDuration.Seconds()

	t.Logf("Concurrency Test Results (%d clients):", numClients)
	t.Logf("  Total Duration: %v", totalDuration)
	t.Logf("  Total Requests: %d", totalRequests)
	t.Logf("  Success Rate: %.1f%% (%d/%d clients)", successRate*100, successfulClients, numClients)
	t.Logf("  Throughput: %.2f requests/second", throughput)

	// Performance assertions
	assert.True(t, successRate >= 0.8, "At least 80%% of clients should succeed")
	assert.True(t, throughput >= 1.0, "Throughput should be at least 1 request/second")

	// Check for memory leaks or resource issues
	if runtime.GOOS != "windows" {
		// Get process info (rough check)
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		t.Logf("  Memory Usage: %.2f MB", float64(m.Alloc)/1024/1024)
	}
}

// TestMemoryUsageStability tests memory usage over time
func TestMemoryUsageStability(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping memory stability test in short mode")
	}

	serverPath := buildServerBinary(t)

	// Start server
	cmd := exec.Command(serverPath, "--log-level", "0")
	stdin, stdout, stderr := startServerWithPipes(t, cmd)
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	// Initialize server
	initRequest := utils.McpInitRequest()
	_ = sendJSONRPCRequest(t, stdin, initRequest)
	_ = readJSONRPCResponse(t, stdout, 5*time.Second)

	// Memory measurements
	var memSnapshots []float64
	measurementInterval := 2 * time.Second
	totalDuration := 30 * time.Second
	measurements := int(totalDuration / measurementInterval)

	t.Log("Starting memory stability test...")

	for i := 0; i < measurements; i++ {
		// Generate some load
		for j := 0; j < 3; j++ {
			listRequest := map[string]any{
				"jsonrpc": "2.0",
				"id":      i*10 + j,
				"method":  "tools/list",
				"params":  map[string]any{},
			}
			_ = sendJSONRPCRequest(t, stdin, listRequest)
			_ = readJSONRPCResponse(t, stdout, 2*time.Second)
		}

		// Measure memory
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		memUsageMB := float64(m.Alloc) / 1024 / 1024
		memSnapshots = append(memSnapshots, memUsageMB)

		t.Logf("Memory usage at %v: %.2f MB", time.Duration(i)*measurementInterval, memUsageMB)

		time.Sleep(measurementInterval)
	}

	// Analyze memory stability
	if len(memSnapshots) >= 3 {
		initialMem := memSnapshots[0]
		finalMem := memSnapshots[len(memSnapshots)-1]
		maxMem := memSnapshots[0]

		for _, mem := range memSnapshots {
			if mem > maxMem {
				maxMem = mem
			}
		}

		memGrowth := finalMem - initialMem
		memGrowthPercent := (memGrowth / initialMem) * 100

		t.Logf("Memory Stability Analysis:")
		t.Logf("  Initial Memory: %.2f MB", initialMem)
		t.Logf("  Final Memory: %.2f MB", finalMem)
		t.Logf("  Peak Memory: %.2f MB", maxMem)
		t.Logf("  Growth: %.2f MB (%.1f%%)", memGrowth, memGrowthPercent)

		// Memory stability assertions
		assert.True(t, memGrowthPercent < 50, "Memory growth should be less than 50%% over test duration")
		assert.True(t, maxMem < 100, "Peak memory usage should be reasonable (< 100MB)")
	}
}

// Helper functions are now in helpers.go

func waitForHTTPServer(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}

		if strings.Contains(err.Error(), "connection refused") {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		return nil // Other errors might indicate server is ready
	}

	return fmt.Errorf("server did not start within %v", timeout)
}
