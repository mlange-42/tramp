package track

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrUnsupported is returned for files of an unsupported format.
var ErrUnsupported = errors.New("unsupported file format")

// ReadFile reads a track file, with the format determined by the file extension.
func ReadFile(path string) (*File, error) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".gpx" {
		return nil, fmt.Errorf("%s: %w", path, ErrUnsupported)
	}

	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	f, err := ReadGPX(r)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	f.Path = path
	return f, nil
}
