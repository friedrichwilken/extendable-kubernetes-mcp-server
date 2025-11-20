#!/bin/bash

# Script to download and setup envtest binaries for Kubernetes integration tests
# Uses the setup-envtest tool from controller-runtime for reliable binary downloads

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

# Configuration
ENVTEST_K8S_VERSION="1.28.0"
SETUP_ENVTEST_VERSION="v0.15.0"

# Setup directories
ENVTEST_DIR="${PROJECT_ROOT}/test/envtest"
BIN_DIR="${ENVTEST_DIR}/bin"
TOOL_DIR="${ENVTEST_DIR}/tools"

echo "Setting up envtest binaries using setup-envtest tool..."
echo "  Kubernetes Version: $ENVTEST_K8S_VERSION"
echo "  Setup-envtest Version: $SETUP_ENVTEST_VERSION"
echo "  Install Directory: $BIN_DIR"

# Create directories
mkdir -p "$BIN_DIR" "$TOOL_DIR"

# Download setup-envtest tool
SETUP_ENVTEST_PATH="${TOOL_DIR}/setup-envtest"

if [ ! -f "$SETUP_ENVTEST_PATH" ]; then
    echo ""
    echo "Downloading setup-envtest tool..."

    # Use go install to get the setup-envtest tool
    export GOBIN="$TOOL_DIR"
    if ! go install "sigs.k8s.io/controller-runtime/tools/setup-envtest@${SETUP_ENVTEST_VERSION}"; then
        echo "Failed to install setup-envtest tool" >&2
        exit 1
    fi

    echo "✅ setup-envtest tool downloaded"
fi

# Use setup-envtest to download binaries
echo ""
echo "Downloading Kubernetes binaries..."

# Run setup-envtest to download and install binaries
if ! "$SETUP_ENVTEST_PATH" use "$ENVTEST_K8S_VERSION" --bin-dir "$BIN_DIR" -p path > /dev/null; then
    echo "Failed to download envtest binaries" >&2
    exit 1
fi

echo "✅ Kubernetes binaries downloaded"

# Verify binaries
echo ""
echo "Verifying binaries..."

# Check what we actually got
BINARIES_PATH=$("$SETUP_ENVTEST_PATH" use "$ENVTEST_K8S_VERSION" --bin-dir "$BIN_DIR" -p path)
echo "Binaries installed in: $BINARIES_PATH"

if [ -f "$BINARIES_PATH/kube-apiserver" ]; then
    if "$BINARIES_PATH/kube-apiserver" --version >/dev/null 2>&1; then
        echo "✅ kube-apiserver is working"
    else
        echo "❌ kube-apiserver verification failed" >&2
        exit 1
    fi
else
    echo "❌ kube-apiserver not found" >&2
    exit 1
fi

if [ -f "$BINARIES_PATH/etcd" ]; then
    if "$BINARIES_PATH/etcd" --version >/dev/null 2>&1; then
        echo "✅ etcd is working"
    else
        echo "❌ etcd verification failed" >&2
        exit 1
    fi
else
    echo "❌ etcd not found" >&2
    exit 1
fi

# Create environment file for tests
cat > "${ENVTEST_DIR}/env.sh" << EOF
#!/bin/bash
# Environment variables for envtest binaries
export KUBEBUILDER_ASSETS="$BINARIES_PATH"
export PATH="$BINARIES_PATH:\$PATH"
EOF

chmod +x "${ENVTEST_DIR}/env.sh"

echo ""
echo "🎉 Envtest setup complete!"
echo ""
echo "To use envtest binaries in tests, run:"
echo "  source ${ENVTEST_DIR}/env.sh"
echo ""
echo "Or set the environment variable:"
echo "  export KUBEBUILDER_ASSETS=$BINARIES_PATH"
echo ""
echo "Binaries available:"
ls -la "$BINARIES_PATH"