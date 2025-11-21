# Testing Strategy

This project implements a comprehensive 3-layer testing architecture to ensure reliability and maintainability.

## Testing Layers

### Layer 1: Unit Tests (`test/unit/`)
Unit tests focus on individual components and CLI compatibility validation. These tests verify that command-line interfaces, flags, and help output match the reference implementation exactly. They run quickly and provide immediate feedback during development.

### Layer 2: Integration Tests (`test/integration/`)
Integration tests validate interactions between components and external systems. This includes server lifecycle management (startup/shutdown), Kubernetes API integration, and MCP protocol compliance. These tests use real Kubernetes environments via envtest to ensure authentic behavior.

### Layer 3: End-to-End Tests (`test/e2e/`)
End-to-end tests cover complete user workflows and production scenarios. This includes full MCP request/response cycles, performance benchmarks, multi-cluster operations, and stress testing. These tests simulate real-world usage patterns and validate the entire system under various conditions.

## Running Tests

```bash
# Run all tests
make test

# Run specific test layers
make test-unit
make test-integration
make test-e2e
```

## Test Coverage

The project maintains high test coverage across all layers, with particular emphasis on:
- CLI compatibility with the reference implementation
- MCP protocol compliance
- Kubernetes API integration
- Performance characteristics
- Multi-cluster scenarios

## Writing Tests

When adding new functionality:
1. Start with unit tests for individual components
2. Add integration tests for system interactions
3. Include e2e tests for complete user workflows
4. Ensure CLI behavior matches the reference implementation exactly