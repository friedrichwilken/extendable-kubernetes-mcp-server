package mcp

// This file contains blank imports for all registered toolsets.
// The blank import triggers the init() function in each toolset package,
// which registers the toolset with the global registry.
//
// This file is automatically managed by mcp-toolgen when using the --register flag.
// You can also manually add imports here for custom toolsets.

// Base toolsets from kubernetes-mcp-server
import (
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/config"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/core"
	_ "github.com/containers/kubernetes-mcp-server/pkg/toolsets/helm"
)
