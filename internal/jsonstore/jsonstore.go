// Package jsonstore centralises the "JSON-on-disk store" dance
// that every persistent store in the project previously
// re-implemented:
//   1. MkdirAll(filepath.Dir(path), 0755)
//   2. json.MarshalIndent(v, "", "  ")
//   3. os.WriteFile(path, data, perm)
//
// Four callers — internal/registry, internal/clientconfig,
// internal/serverd, and the audit-log open path in
// internal/agentd — were each carrying a copy. The shared
// helper preserves caller intent: marshal and write the value
// passed in, with a chosen file mode. Map key ordering is left
// to the caller (use jsonstore.Ordered to wrap a map before
// passing it in if you need stable diffs).
package jsonstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// Save writes v as pretty-printed JSON to path, creating the
// parent directory (mode 0755) if absent. The file is written
// with the given perm. The mode is only applied at create-time;
// existing files keep their existing permissions.
func Save(path string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

// Ordered returns the entries of m sorted by key, suitable for
// passing to Save when stable file diffs matter. Each value is
// the same pointer that lives in m.
func Ordered[V any](m map[string]V) map[string]V {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make(map[string]V, len(m))
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}
