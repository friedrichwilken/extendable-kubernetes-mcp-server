# Extendable Kubernetes MCP Server

A clean foundation Model Context Protocol (MCP) server that replicates all [kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) functionality. This serves as a starting point for building custom extensions while maintaining full compatibility with the original server.

## Features

- **Complete k8s-mcp-server Compatibility**: Provides identical functionality and CLI interface to kubernetes-mcp-server
- **All Base Kubernetes Tools**: All standard Kubernetes tools (pods, deployments, services, namespaces, etc.)
- **Helm Integration**: Full Helm chart management support
- **Multi-cluster Support**: Multi-cluster operations with target parameter injection
- **Dual Transport Support**: Supports both stdio (default) and HTTP transports with SSE endpoints
- **Extensible Architecture**: Clean foundation ready for custom toolset development

## Architecture

This server provides:
1. **Identical Interface**: Same CLI flags and behavior as kubernetes-mcp-server
2. **Same Toolsets**: All base toolsets (core, config, helm, kiali)
3. **Same Transport**: Identical stdio and HTTP/SSE server implementation
4. **Extension Ready**: Clean architecture for adding custom toolsets

## Available Tools

The server provides all the same tools as kubernetes-mcp-server:

### Core Kubernetes Tools
- Pod operations (list, get, delete, logs, exec, run, top)
- Generic resource operations (create, get, list, delete)
- Namespace and event management
- Node operations and metrics

### Helm Tools
- Chart installation and management
- Release listing and operations

### Configuration Tools
- Kubeconfig viewing and management

### Kiali Tools (if available)
- Service mesh observability integration

## Installation

### Prerequisites

- Go 1.25 or later
- Access to a Kubernetes cluster
- kubectl configured with appropriate permissions
- [kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) as a sibling directory

### Build from Source

```bash
# Clone both repositories
git clone https://github.com/containers/kubernetes-mcp-server.git
git clone https://github.com/friedrichwilken/extendable-kubernetes-mcp-server.git
cd extendable-kubernetes-mcp-server

# Build the server
make build
```

## Usage

The server provides identical usage to kubernetes-mcp-server:

### STDIO Mode (Default)

```bash
# Start server in stdio mode
./build/extendable-k8s-mcp

# With specific kubeconfig
./build/extendable-k8s-mcp --kubeconfig ~/.kube/config

# With specific toolsets
./build/extendable-k8s-mcp --toolsets core,helm
```

### HTTP Mode

```bash
# Start HTTP server on port 8080
./build/extendable-k8s-mcp --port 8080

# With public HTTPS host
./build/extendable-k8s-mcp --port 8443 --sse-base-url https://example.com:8443
```

### Configuration Options

All kubernetes-mcp-server flags are supported:

- `--kubeconfig`: Path to kubeconfig file
- `--port`: HTTP server port (enables HTTP mode)
- `--sse-base-url`: Public base URL for SSE endpoints
- `--toolsets`: Comma-separated list of toolsets to use
- `--list-output`: Output format (yaml, table)
- `--read-only`: Enable read-only mode
- `--disable-destructive`: Disable destructive operations
- `--disable-multi-cluster`: Disable multi-cluster tools
- `--log-level`: Set log level (0-9)

## Testing

This project uses a comprehensive 3-layer testing strategy to ensure reliability and compatibility. For detailed information about the testing approach, see [TESTING.md](TESTING.md).

## Development

### Project Structure

```
extendable-kubernetes-mcp-server/
├── cmd/                    # Main application entry point
├── pkg/cmd/               # CLI command structure
├── test/                  # Comprehensive testing infrastructure
├── Makefile              # Build and development tasks
├── go.mod                # Go module definition
└── README.md             # This file
```

### Building Extensions

This server provides a clean foundation for building custom MCP toolsets:

1. **Add Custom Toolsets**: Implement the `api.Toolset` interface from kubernetes-mcp-server
2. **Register Toolsets**: Use `toolsets.Register()` in your initialization code
3. **Follow Patterns**: Use the same patterns as existing kubernetes-mcp-server toolsets

### Dependencies

- Based on kubernetes-mcp-server via go.mod replace directive
- Uses the same Kubernetes client libraries (v0.34.2)
- Uses Model Context Protocol SDK (v1.1.0)

## Compatibility

This server is designed to be a drop-in replacement for kubernetes-mcp-server:

- ✅ Same CLI interface and flags
- ✅ Same MCP tools and functionality
- ✅ Same transport protocols (stdio, HTTP/SSE)
- ✅ Same authentication and authorization
- ✅ Same multi-cluster support
- ✅ Same output formats and behavior

## License

MIT License - see [LICENSE](LICENSE) for details.

## Acknowledgments

This project is based on and extends [kubernetes-mcp-server](https://github.com/containers/kubernetes-mcp-server) by the Containers organization. The original project is licensed under the Apache License 2.0. We gratefully acknowledge their excellent work that serves as the foundation for this extendable version.

Key contributions from the original project:
- Core MCP protocol implementation
- Kubernetes client integration patterns
- Toolset architecture and interfaces
- Multi-cluster support design
- HTTP/SSE transport implementation

## Contributing

This project serves as a foundation for extending kubernetes-mcp-server. When ready to add custom functionality:

1. Implement custom toolsets following kubernetes-mcp-server patterns
2. Register new toolsets in the initialization code
3. Maintain compatibility with the base kubernetes-mcp-server interface
4. Test thoroughly against real Kubernetes clusters