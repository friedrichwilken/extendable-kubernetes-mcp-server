// Package integration contains integration tests that verify Kubernetes integration
// using envtest to run against a real Kubernetes API server.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/test/utils"
)

var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	// Note: envtest requires etcd and kube-apiserver binaries
	// These are automatically downloaded by controller-runtime/pkg/envtest on first use
	// Set KUBEBUILDER_ASSETS if you have them installed elsewhere

	// For now, we'll skip envtest if the binaries aren't available
	// In a full CI setup, these would be installed

	// Run the tests
	m.Run()
}

func TestKubernetesClientIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kubernetes integration tests in short mode")
	}

	// Setup envtest environment
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{},
		ErrorIfCRDPathMissing: false,
		BinaryAssetsDirectory: "", // Will use default or KUBEBUILDER_ASSETS
	}

	cfg, err := testEnv.Start()
	if err != nil {
		// Skip if envtest binaries are not available
		t.Skipf("Skipping Kubernetes integration test - envtest not available: %v", err)
	}

	defer func() {
		if testEnv != nil {
			_ = testEnv.Stop()
		}
	}()

	require.NotNil(t, cfg, "Should have valid Kubernetes config")

	// Test basic Kubernetes client functionality
	t.Run("basic_client_operations", func(t *testing.T) {
		testBasicClientOperations(t, cfg)
	})

	t.Run("namespace_operations", func(t *testing.T) {
		testNamespaceOperations(t, cfg)
	})

	t.Run("pod_operations", func(t *testing.T) {
		testPodOperations(t, cfg)
	})
}

func testBasicClientOperations(t *testing.T, cfg *rest.Config) {
	// Create Kubernetes client
	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err, "Failed to create Kubernetes client")

	// Test server version (should work with envtest)
	version, err := client.Discovery().ServerVersion()
	require.NoError(t, err, "Failed to get server version")
	assert.NotEmpty(t, version.GitVersion, "Server version should not be empty")

	// Test API resources
	resources, err := client.Discovery().ServerResourcesForGroupVersion("v1")
	require.NoError(t, err, "Failed to get v1 resources")
	assert.NotEmpty(t, resources.APIResources, "Should have v1 API resources")

	// Verify common resources exist
	resourceNames := make([]string, len(resources.APIResources))
	for i := range resources.APIResources {
		resourceNames[i] = resources.APIResources[i].Name
	}

	expectedResources := []string{"pods", "services", "namespaces", "configmaps", "secrets"}
	for _, expected := range expectedResources {
		assert.Contains(t, resourceNames, expected, "Should have %s resource", expected)
	}
}

func testNamespaceOperations(t *testing.T, cfg *rest.Config) {
	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err, "Failed to create Kubernetes client")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test namespace
	testNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ek8sms-integration",
		},
	}

	createdNS, err := client.CoreV1().Namespaces().Create(ctx, testNS, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create test namespace")
	assert.Equal(t, testNS.Name, createdNS.Name, "Created namespace should have correct name")

	// Cleanup namespace
	defer func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), testNS.Name, metav1.DeleteOptions{})
	}()

	// List namespaces
	namespaces, err := client.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "Failed to list namespaces")

	// Find our test namespace
	found := false
	for i := range namespaces.Items {
		if namespaces.Items[i].Name == testNS.Name {
			found = true
			break
		}
	}
	assert.True(t, found, "Test namespace should be in namespace list")

	// Get specific namespace
	retrievedNS, err := client.CoreV1().Namespaces().Get(ctx, testNS.Name, metav1.GetOptions{})
	require.NoError(t, err, "Failed to get test namespace")
	assert.Equal(t, testNS.Name, retrievedNS.Name, "Retrieved namespace should have correct name")
}

func testPodOperations(t *testing.T, cfg *rest.Config) {
	client, err := kubernetes.NewForConfig(cfg)
	require.NoError(t, err, "Failed to create Kubernetes client")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create test namespace first
	testNS := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ek8sms-pods",
		},
	}

	_, err = client.CoreV1().Namespaces().Create(ctx, testNS, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create test namespace for pods")

	// Cleanup namespace (and all pods within)
	defer func() {
		_ = client.CoreV1().Namespaces().Delete(context.Background(), testNS.Name, metav1.DeleteOptions{})
	}()

	// Create test pod
	testPod := utils.CreateTestPod("test-pod", testNS.Name)

	createdPod, err := client.CoreV1().Pods(testNS.Name).Create(ctx, testPod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create test pod")
	assert.Equal(t, testPod.Name, createdPod.Name, "Created pod should have correct name")
	assert.Equal(t, testNS.Name, createdPod.Namespace, "Created pod should be in correct namespace")

	// List pods in namespace
	pods, err := client.CoreV1().Pods(testNS.Name).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "Failed to list pods")
	assert.Len(t, pods.Items, 1, "Should have exactly one pod in test namespace")
	assert.Equal(t, testPod.Name, pods.Items[0].Name, "Listed pod should be our test pod")

	// Get specific pod
	retrievedPod, err := client.CoreV1().Pods(testNS.Name).Get(ctx, testPod.Name, metav1.GetOptions{})
	require.NoError(t, err, "Failed to get test pod")
	assert.Equal(t, testPod.Name, retrievedPod.Name, "Retrieved pod should have correct name")

	// Update pod (add a label)
	retrievedPod.Labels = map[string]string{"test": "updated"}
	updatedPod, err := client.CoreV1().Pods(testNS.Name).Update(ctx, retrievedPod, metav1.UpdateOptions{})
	require.NoError(t, err, "Failed to update test pod")
	assert.Equal(t, "updated", updatedPod.Labels["test"], "Updated pod should have new label")

	// Delete pod
	err = client.CoreV1().Pods(testNS.Name).Delete(ctx, testPod.Name, metav1.DeleteOptions{})
	require.NoError(t, err, "Failed to delete test pod")

	// Verify pod is deleted (with retry for async deletion)
	deleted := false
	for i := 0; i < 10; i++ {
		_, err = client.CoreV1().Pods(testNS.Name).Get(ctx, testPod.Name, metav1.GetOptions{})
		if err != nil {
			deleted = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	assert.True(t, deleted, "Pod should be deleted")
}

func TestKubernetesAuthentication(t *testing.T) {
	utils.SkipIfShort(t)

	t.Run("kubeconfig_file", func(t *testing.T) {
		// Create a temporary kubeconfig file
		kubeconfigPath := utils.CreateTestKubeconfig(t, "https://localhost:6443")

		// Test that kubeconfig can be parsed
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			// This is expected if we can't connect to the server
			// but the config should be parseable
			assert.Contains(t, err.Error(), "connection refused",
				"Should fail with connection error, not parsing error")
		} else {
			assert.NotNil(t, config, "Should have valid config")
			assert.Equal(t, "https://localhost:6443", config.Host, "Should have correct host")
		}
	})

	t.Run("invalid_kubeconfig", func(t *testing.T) {
		tempDir := utils.TempDir(t)
		invalidPath := utils.WriteTestFile(t, tempDir, "invalid-kubeconfig", "invalid yaml content")

		// Should fail to parse invalid kubeconfig
		_, err := clientcmd.BuildConfigFromFlags("", invalidPath)
		assert.Error(t, err, "Should fail to parse invalid kubeconfig")
	})
}

// Mock server tests (when envtest is not available)
func TestKubernetesMockServer(t *testing.T) {
	// Create mock Kubernetes server
	mockServer := utils.NewMockKubernetesServer()
	defer mockServer.Close()

	// Add handlers for basic resources
	testPod := utils.CreateTestPod("mock-pod", "default")
	mockServer.AddHandler(utils.PodListHandler(testPod))

	// Test client against mock server
	client, err := kubernetes.NewForConfig(mockServer.GetConfig())
	require.NoError(t, err, "Failed to create client for mock server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test listing pods (should get our mock pod)
	pods, err := client.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "Failed to list pods from mock server")
	assert.Len(t, pods.Items, 1, "Should have one pod from mock server")
	assert.Equal(t, "mock-pod", pods.Items[0].Name, "Should get mock pod")
}
