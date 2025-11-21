package functions

// FunctionToolset provides MCP tools for managing Function custom resources
type FunctionToolset struct{}

// GetName returns the name of this toolset
func (t *FunctionToolset) GetName() string {
	return "functions"
}

// GetDescription returns the description of this toolset
func (t *FunctionToolset) GetDescription() string {
	return "Tools for managing Function custom resources"
}
