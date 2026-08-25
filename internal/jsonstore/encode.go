// Lightweight shim so the jsonstore test can round-trip values
// without pulling encoding/json into the package's import
// surface. The real Save helper uses encoding/json internally.
package jsonstore

import "encoding/json"

// jsonUnmarshal is a thin alias that lets tests verify Save's
// output round-trips without exporting the unmarshal call site.
func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
