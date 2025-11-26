# Multi-stage Dockerfile for extendable-kubernetes-mcp-server
# Build optimized static binary for secure, minimal container deployment

# Stage 1: Build stage
FROM golang:1.25 AS builder

WORKDIR /workspace

# Copy go module files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY pkg/ pkg/

# Build static binary with optimizations
# CGO_ENABLED=0 for static linking
# -ldflags="-s -w" to strip debug info and reduce size
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" \
    -o extendable-k8s-mcp \
    ./cmd

# Stage 2: Runtime stage
FROM gcr.io/distroless/static-debian12:nonroot

# Use distroless for minimal attack surface
# - No shell, package manager, or unnecessary binaries
# - Automatic non-root user (65532:65532)
# - Minimal CVE exposure

WORKDIR /app

# Copy only the binary from builder
COPY --from=builder /workspace/extendable-k8s-mcp /app/extendable-k8s-mcp

# Use non-root user (already default in distroless:nonroot)
USER nonroot:nonroot

# Expose HTTP port (required for Kubernetes deployment)
EXPOSE 3000

# Set entrypoint and default command
# HTTP mode is required for kmcp deployment
ENTRYPOINT ["/app/extendable-k8s-mcp"]
CMD ["--port", "3000"]
