// Package mcp provides extensions to kubernetes-mcp-server for resource support
package mcp

import (
	"context"

	k8sapi "github.com/containers/kubernetes-mcp-server/pkg/api"
	localapi "github.com/friedrichwilken/extendable-kubernetes-mcp-server/pkg/api"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// RegisterToolsetResources registers MCP resources from toolsets that implement ResourceProvider
func RegisterToolsetResources(mcpServer *mcp.Server, toolsets []k8sapi.Toolset) error {
	for _, toolset := range toolsets {
		if resourceProvider, ok := toolset.(localapi.ResourceProvider); ok {
			err := resourceProvider.RegisterResources(func(uri, name, mimeType string, handler func(context.Context) (string, error)) error {
				resource := &mcp.Resource{
					URI:      uri,
					Name:     name,
					MIMEType: mimeType,
				}
				resourceHandler := func(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
					content, err := handler(ctx)
					if err != nil {
						return nil, err
					}
					return &mcp.ReadResourceResult{
						Contents: []*mcp.ResourceContents{
							{
								URI:      uri,
								MIMEType: mimeType,
								Text:     content,
							},
						},
					}, nil
				}
				mcpServer.AddResource(resource, resourceHandler)
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}
