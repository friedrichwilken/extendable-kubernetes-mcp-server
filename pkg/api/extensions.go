package api

import (
	"context"

	"github.com/containers/kubernetes-mcp-server/pkg/api"
)

// ResourceProvider is an optional interface that toolsets can implement to expose MCP resources.
// Resources provide read-only contextual information that LLMs can access, such as CRD definitions,
// documentation, or configuration data.
type ResourceProvider interface {
	api.Toolset
	// RegisterResources registers MCP resources with the server.
	// This method is called during server initialization if the toolset implements this interface.
	RegisterResources(registerFunc func(uri, name, mimeType string, handler func(context.Context) (string, error)) error) error
}
