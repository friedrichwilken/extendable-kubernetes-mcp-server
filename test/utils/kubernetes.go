// Package utils provides Kubernetes testing utilities.
// These utilities help create mock Kubernetes servers and test fixtures.
package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
)

// MockKubernetesServer provides a mock Kubernetes API server for testing.
// Adapted from k8sms mock server utilities.
type MockKubernetesServer struct {
	server       *httptest.Server
	config       *rest.Config
	restHandlers []http.HandlerFunc
}

// NewMockKubernetesServer creates a new mock Kubernetes server.
func NewMockKubernetesServer() *MockKubernetesServer {
	ms := &MockKubernetesServer{}
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	ms.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		for _, handler := range ms.restHandlers {
			handler(w, req)
		}
	}))

	ms.config = &rest.Config{
		Host:    ms.server.URL,
		APIPath: "/api",
		ContentConfig: rest.ContentConfig{
			NegotiatedSerializer: codecs,
			ContentType:          runtime.ContentTypeJSON,
			GroupVersion:         &v1.SchemeGroupVersion,
		},
	}

	ms.restHandlers = make([]http.HandlerFunc, 0)
	return ms
}

// GetConfig returns the rest.Config for connecting to this mock server.
func (ms *MockKubernetesServer) GetConfig() *rest.Config {
	return ms.config
}

// AddHandler adds a REST API handler to the mock server.
func (ms *MockKubernetesServer) AddHandler(handler http.HandlerFunc) {
	ms.restHandlers = append(ms.restHandlers, handler)
}

// Close shuts down the mock server.
func (ms *MockKubernetesServer) Close() {
	if ms.server != nil {
		ms.server.Close()
	}
}

// CreateTestKubeconfig creates a temporary kubeconfig file for testing.
func CreateTestKubeconfig(t *testing.T, serverURL string) string {
	config := &api.Config{
		Clusters: map[string]*api.Cluster{
			"test-cluster": {
				Server:                serverURL,
				InsecureSkipTLSVerify: true,
			},
		},
		AuthInfos: map[string]*api.AuthInfo{
			"test-user": {
				Token: "test-token",
			},
		},
		Contexts: map[string]*api.Context{
			"test-context": {
				Cluster:  "test-cluster",
				AuthInfo: "test-user",
			},
		},
		CurrentContext: "test-context",
	}

	tempDir := TempDir(t)
	kubeconfigPath := WriteTestFile(t, tempDir, "kubeconfig", "")

	err := clientcmd.WriteToFile(*config, kubeconfigPath)
	require.NoError(t, err, "Failed to write kubeconfig")

	return kubeconfigPath
}

// CreateTestPod creates a test Pod object for use in tests.
func CreateTestPod(name, namespace string) *v1.Pod {
	return &v1.Pod{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Pod",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "test-container",
					Image: "nginx:latest",
				},
			},
		},
		Status: v1.PodStatus{
			Phase: v1.PodRunning,
		},
	}
}

// CreateTestService creates a test Service object for use in tests.
func CreateTestService(name, namespace string) *v1.Service {
	return &v1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.ServiceSpec{
			Selector: map[string]string{
				"app": name,
			},
			Ports: []v1.ServicePort{
				{
					Port: 80,
				},
			},
		},
	}
}

// PodListHandler creates an HTTP handler that returns a list of pods.
func PodListHandler(pods ...*v1.Pod) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/pods" && r.URL.Path != "/api/v1/namespaces/default/pods" {
			return
		}

		podList := &v1.PodList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "PodList",
			},
			Items: make([]v1.Pod, len(pods)),
		}

		for i, pod := range pods {
			podList.Items[i] = *pod
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(podList)
	}
}

// ServiceListHandler creates an HTTP handler that returns a list of services.
func ServiceListHandler(services ...*v1.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/services" && r.URL.Path != "/api/v1/namespaces/default/services" {
			return
		}

		serviceList := &v1.ServiceList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ServiceList",
			},
			Items: make([]v1.Service, len(services)),
		}

		for i, service := range services {
			serviceList.Items[i] = *service
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(serviceList)
	}
}
