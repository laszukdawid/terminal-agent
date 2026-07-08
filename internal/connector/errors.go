package connector

import "errors"

// ErrNoProviderResponse indicates a provider returned a response with no usable
// content (for example zero choices or candidates, or a candidate whose content
// was withheld). Connectors return it instead of indexing into an empty slice
// or dereferencing a nil content pointer, so a malformed or filtered provider
// response surfaces as an error rather than crashing the agent.
var ErrNoProviderResponse = errors.New("provider returned no response")
