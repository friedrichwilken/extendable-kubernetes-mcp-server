// Package main provides the entry point for the extendable Kubernetes MCP server.
// This server replicates all kubernetes-mcp-server functionality while providing
// a clean foundation for building custom extensions.
package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericiooptions"

	"github.com/friedrichwilken/extendable-kubernetes-mcp-server/pkg/cmd"
)

func main() {
	flags := pflag.NewFlagSet("extendable-k8s-mcp", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := cmd.NewExtendableMCPServer(genericiooptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
