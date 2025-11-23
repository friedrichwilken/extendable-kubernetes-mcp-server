// Package mcp provides an extended MCP server with resource support
package mcp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	authenticationapiv1 "k8s.io/api/authentication/v1"
	"k8s.io/utils/ptr"

	k8sapi "github.com/containers/kubernetes-mcp-server/pkg/api"
	"github.com/containers/kubernetes-mcp-server/pkg/config"
	internalk8s "github.com/containers/kubernetes-mcp-server/pkg/kubernetes"
	k8smcp "github.com/containers/kubernetes-mcp-server/pkg/mcp"
	"github.com/containers/kubernetes-mcp-server/pkg/output"
	"github.com/containers/kubernetes-mcp-server/pkg/toolsets"
	"github.com/containers/kubernetes-mcp-server/pkg/version"
	localapi "github.com/friedrichwilken/extendable-kubernetes-mcp-server/pkg/api"
)

type ContextKey string

const TokenScopesContextKey = ContextKey("TokenScopesContextKey")

// Configuration wraps k8sms configuration
type Configuration struct {
	*config.StaticConfig
	listOutput output.Output
	toolsets   []k8sapi.Toolset
}

func (c *Configuration) Toolsets() []k8sapi.Toolset {
	if c.toolsets == nil {
		for _, toolset := range c.StaticConfig.Toolsets {
			c.toolsets = append(c.toolsets, toolsets.ToolsetFromString(toolset))
		}
	}
	return c.toolsets
}

func (c *Configuration) ListOutput() output.Output {
	if c.listOutput == nil {
		c.listOutput = output.FromString(c.StaticConfig.ListOutput)
	}
	return c.listOutput
}

func (c *Configuration) isToolApplicable(tool *k8sapi.ServerTool) bool {
	if c.ReadOnly && !ptr.Deref(tool.Tool.Annotations.ReadOnlyHint, false) {
		return false
	}
	if c.DisableDestructive && ptr.Deref(tool.Tool.Annotations.DestructiveHint, false) {
		return false
	}
	if c.EnabledTools != nil && !slices.Contains(c.EnabledTools, tool.Tool.Name) {
		return false
	}
	if c.DisabledTools != nil && slices.Contains(c.DisabledTools, tool.Tool.Name) {
		return false
	}
	return true
}

// Server is an extended MCP server with resource support
type Server struct {
	configuration *Configuration
	server        *mcp.Server
	enabledTools  []string
	p             internalk8s.Provider
}

// NewExtendableServer creates a new MCP server with both tool and resource support
func NewExtendableServer(k8sConfig k8smcp.Configuration) (*Server, error) {
	// Wrap the configuration
	cfg := &Configuration{
		StaticConfig: k8sConfig.StaticConfig,
	}

	s := &Server{
		configuration: cfg,
		server: mcp.NewServer(
			&mcp.Implementation{
				Name: version.BinaryName, Title: version.BinaryName, Version: version.Version,
			},
			&mcp.ServerOptions{
				HasResources: true,
				HasPrompts:   false,
				HasTools:     true,
			}),
	}

	// Add middlewares (copied from k8sms)
	s.server.AddReceivingMiddleware(authHeaderPropagationMiddleware)
	s.server.AddReceivingMiddleware(toolCallLoggingMiddleware)
	if cfg.RequireOAuth && false { // TODO: Disabled scope auth validation for now
		s.server.AddReceivingMiddleware(toolScopedAuthorizationMiddleware)
	}

	// Register resources from ResourceProvider toolsets
	if err := s.registerResources(); err != nil {
		return nil, fmt.Errorf("failed to register resources: %w", err)
	}

	// Initialize Kubernetes provider and tools
	if err := s.reloadKubernetesClusterProvider(); err != nil {
		return nil, err
	}
	s.p.WatchTargets(s.reloadKubernetesClusterProvider)

	return s, nil
}

// registerResources registers MCP resources from ResourceProvider toolsets
func (s *Server) registerResources() error {
	for _, toolset := range s.configuration.Toolsets() {
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
				s.server.AddResource(resource, resourceHandler)
				return nil
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// reloadKubernetesClusterProvider reloads the Kubernetes provider and tools (copied from k8sms)
func (s *Server) reloadKubernetesClusterProvider() error {
	ctx := context.Background()
	p, err := internalk8s.NewProvider(s.configuration.StaticConfig)
	if err != nil {
		return err
	}

	// close the old provider
	if s.p != nil {
		s.p.Close()
	}

	s.p = p

	targets, err := p.GetTargets(ctx)
	if err != nil {
		return err
	}

	filter := CompositeFilter(
		s.configuration.isToolApplicable,
		ShouldIncludeTargetListTool(p.GetTargetParameterName(), targets),
	)

	mutator := WithTargetParameter(
		p.GetDefaultTarget(),
		p.GetTargetParameterName(),
		targets,
	)

	// Track previously enabled tools
	previousTools := s.enabledTools

	// Build new list of applicable tools
	applicableTools := make([]*k8sapi.ServerTool, 0)
	s.enabledTools = make([]string, 0)
	for _, toolset := range s.configuration.Toolsets() {
		for _, tool := range toolset.GetTools(p) {
			tool := mutator(tool)
			if !filter(&tool) {
				continue
			}

			applicableTools = append(applicableTools, &tool)
			s.enabledTools = append(s.enabledTools, tool.Tool.Name)
		}
	}

	// Remove tools that are no longer applicable
	toolsToRemove := make([]string, 0)
	for _, oldTool := range previousTools {
		if !slices.Contains(s.enabledTools, oldTool) {
			toolsToRemove = append(toolsToRemove, oldTool)
		}
	}
	s.server.RemoveTools(toolsToRemove...)

	for _, tool := range applicableTools {
		goSdkTool, goSdkToolHandler, err := ServerToolToGoSdkTool(s, tool)
		if err != nil {
			return fmt.Errorf("failed to convert tool %s: %v", tool.Tool.Name, err)
		}
		s.server.AddTool(goSdkTool, goSdkToolHandler)
	}

	// start new watch
	s.p.WatchTargets(s.reloadKubernetesClusterProvider)
	return nil
}

// ServeStdio serves the MCP server over stdio
func (s *Server) ServeStdio() error {
	ctx := context.Background()
	return s.server.Run(ctx, &mcp.LoggingTransport{Transport: &mcp.StdioTransport{}, Writer: os.Stderr})
}

// ServeSse returns an SSE handler
func (s *Server) ServeSse() *mcp.SSEHandler {
	return mcp.NewSSEHandler(func(request *http.Request) *mcp.Server {
		return s.server
	}, &mcp.SSEOptions{})
}

// ServeHTTP returns an HTTP handler
func (s *Server) ServeHTTP() *mcp.StreamableHTTPHandler {
	return mcp.NewStreamableHTTPHandler(func(request *http.Request) *mcp.Server {
		return s.server
	}, &mcp.StreamableHTTPOptions{
		Stateless: false,
	})
}

// KubernetesApiVerifyToken verifies a token
func (s *Server) KubernetesApiVerifyToken(ctx context.Context, cluster, token, audience string) (*authenticationapiv1.UserInfo, []string, error) {
	if s.p == nil {
		return nil, nil, fmt.Errorf("kubernetes cluster provider is not initialized")
	}
	return s.p.VerifyToken(ctx, cluster, token, audience)
}

// GetTargetParameterName returns the target parameter name
func (s *Server) GetTargetParameterName() string {
	if s.p == nil {
		return ""
	}
	return s.p.GetTargetParameterName()
}

// GetEnabledTools returns the list of enabled tools
func (s *Server) GetEnabledTools() []string {
	return s.enabledTools
}

// Close closes the server
func (s *Server) Close() {
	if s.p != nil {
		s.p.Close()
	}
}

// NewTextResult creates a text result (copied from k8sms)
func NewTextResult(content string, err error) *mcp.CallToolResult {
	if err != nil {
		return &mcp.CallToolResult{
			IsError: true,
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: err.Error(),
				},
			},
		}
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: content,
			},
		},
	}
}
