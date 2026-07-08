package connector

// requiredFields extracts the JSON-schema "required" property names from a tool
// input schema, tolerating the two slice shapes that occur in this codebase.
//
// Built-in tools construct their schema natively in Go and store "required" as a
// []string. MCP tool schemas instead pass through a JSON round-trip
// (utils.ConvertMCPSchemaToMap), and json.Unmarshal into map[string]any yields a
// []any, never a []string. Connectors that extract "required" with a bare
// .([]string) assertion therefore silently drop the required list for every MCP
// tool, advertising those tools with no mandatory parameters. Both shapes (and a
// missing key) must be handled.
func requiredFields(inputSchema map[string]any) []string {
	switch v := inputSchema["required"].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
