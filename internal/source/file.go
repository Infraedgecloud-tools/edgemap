package source

import (
	"fmt"
	"io"
	"os"
)

// ReadRegularFile rejects symlinks and special files, then bound the opened regular-file read.
// ParseHCL/ParseJSON are used instead of their unbounded File convenience APIs.
func ReadRegularFile(filename string) ([]byte, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, fmt.Errorf("cannot stat input")
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file (symlinks are skipped)")
	}
	f, err := os.Open(filename) // #nosec G304 -- user-selected configuration; regular-file checks and bounded read below.
	if err != nil {
		return nil, fmt.Errorf("cannot open input")
	}
	defer func() { _ = f.Close() }()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input must be a regular file")
	}
	const limit = 16 << 20
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read input")
	}
	if len(data) > limit {
		return nil, fmt.Errorf("exceeds 16 MiB input limit")
	}
	return data, nil
}
