package compat

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
// Task 3 will replace this stub with the real YAML-reading implementation.
func LoadSkiplist(_ string) (Skiplist, error) {
	return Skiplist{}, nil
}
