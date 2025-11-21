package functions

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FunctionClient provides operations for Function custom resources
type FunctionClient struct {
	client    client.Client
	namespace string
}

// NewFunctionClient creates a new client for Function resources
func NewFunctionClient(c client.Client, namespace string) *FunctionClient {
	return &FunctionClient{
		client:    c,
		namespace: namespace,
	}
}

// Create creates a new Function resource
func (c *FunctionClient) Create(ctx context.Context, obj *Function) error {
	return c.client.Create(ctx, obj)
}
