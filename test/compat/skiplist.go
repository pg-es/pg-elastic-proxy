package compat

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

var (
	errReadSkiplist  = errors.New("failed to read skiplist file")
	errParseSkiplist = errors.New("failed to parse skiplist YAML")
)

// SkipEntry is one entry in the skiplist.
type SkipEntry struct {
	File   string `yaml:"file"`
	Reason string `yaml:"reason"`
}

// Skiplist holds the set of test names and files to skip.
type Skiplist struct {
	Skips []SkipEntry `yaml:"skips"`
}

// ShouldSkip reports whether the given name should be skipped.
// Returns (true, reason) if matched, (false, "") otherwise.
func (sl Skiplist) ShouldSkip(name string) (skip bool, reason string) {
	for _, entry := range sl.Skips {
		if entry.File == name {
			return true, entry.Reason
		}
	}

	return false, ""
}

// LoadSkiplist reads a skiplist YAML file from the given path.
func LoadSkiplist(path string) (result Skiplist, err error) {
	data, readErr := os.ReadFile(filepath.Clean(path))
	if readErr != nil {
		return Skiplist{}, fmt.Errorf("%w: %w", errReadSkiplist, readErr)
	}

	var sl Skiplist

	parseErr := yaml.Unmarshal(data, &sl)
	if parseErr != nil {
		return Skiplist{}, fmt.Errorf("%w: %w", errParseSkiplist, parseErr)
	}

	return sl, nil
}
