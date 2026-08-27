package agentd

import "io"

// readAll is a small wrapper kept in its own file so the
// admin_test.go file doesn't need to import io directly
// (readability of the test list).
func readAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}
