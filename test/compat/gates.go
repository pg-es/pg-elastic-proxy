package compat

import (
	"fmt"
	"runtime"
	"strconv"
	"strings"
)

// Capability is one entry in a requires/skip capabilities check.
type Capability struct {
	Method       string   `yaml:"method"`
	Path         string   `yaml:"path"`
	Parameters   []string `yaml:"parameters"`
	Capabilities []string `yaml:"capabilities"`
}

// KnownIssue is one entry in a skip known_issues check.
type KnownIssue struct {
	ClusterFeature string `yaml:"cluster_feature"`
	FixedBy        string `yaml:"fixed_by"`
}

// Target describes the backend the suite runs against and what the runner can do.
type Target struct {
	Flavour string
	Version string
	OS      string
}

// supportsRunnerFeature reports whether the runner implements an ES
// test-runner feature named in a requires or skip gate.
func supportsRunnerFeature(name string) bool {
	switch name {
	case "headers", "catch_unauthorized", "contains", "regex", "close_to",
		"is_after", "matches_in_any_order", "transform_and_set",
		"stash_in_key", "stash_in_path", "skip_os", "default_shards",
		"warnings", "allowed_warnings", "arbitrary_key":
		return true
	default:
		return false
	}
}

// SatisfiesRequires reports whether every requirement of a file or section is
// met on this target. pg_es exposes no ES capabilities or cluster features, so
// those requirements are never satisfied.
func (t Target) SatisfiesRequires(requires *Requires) bool {
	if !requires.AppliesTo(t.Flavour) {
		return false
	}

	if len(requires.Capabilities) > 0 {
		return false
	}

	if len(asStrings(requires.ClusterFeatures)) > 0 {
		return false
	}

	if !t.supportsAll(asStrings(requires.TestRunnerFeatures)) || !t.supportsAll(asStrings(requires.Features)) {
		return false
	}

	oses := asStrings(requires.OS)
	if len(oses) > 0 && !t.matchesOS(oses) {
		return false
	}

	if requires.Version != "" && !versionInRange(t.Version, requires.Version) {
		return false
	}

	return true
}

// ShouldSkip reports whether a skip gate's condition applies on this target.
func (t Target) ShouldSkip(skip *SkipStep) bool {
	if skip.AwaitsFix != "" {
		return true
	}

	// No ES capabilities or cluster features exist here, so skip conditions
	// keyed on their presence never fire.
	if len(skip.Capabilities) > 0 || len(asStrings(skip.ClusterFeatures)) > 0 || len(skip.KnownIssues) > 0 {
		return false
	}

	if t.matchesOS(asStrings(skip.OS)) {
		return true
	}

	if skip.Version != "" && versionInRange(t.Version, skip.Version) {
		return true
	}

	return t.lacksAny(asStrings(skip.TestRunnerFeatures)) || t.lacksAny(asStrings(skip.Features))
}

func (t Target) supportsAll(features []string) bool {
	for _, feature := range features {
		if !supportsRunnerFeature(feature) {
			return false
		}
	}

	return true
}

func (t Target) lacksAny(features []string) bool {
	for _, feature := range features {
		if !supportsRunnerFeature(feature) {
			return true
		}
	}

	return false
}

func (t Target) matchesOS(candidates []string) bool {
	current := t.OS
	if current == "" {
		current = runtime.GOOS
	}

	for _, candidate := range candidates {
		if strings.EqualFold(candidate, current) {
			return true
		}
	}

	return false
}

// asStrings normalises a YAML scalar-or-list value into a string slice.
func asStrings(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return []string{typed}
	case []string:
		return typed
	case []any:
		out := make([]string, len(typed))
		for idx, item := range typed {
			out[idx] = fmt.Sprint(item)
		}

		return out
	default:
		return []string{fmt.Sprint(typed)}
	}
}

// versionInRange reports whether version falls within an ES version-range
// pattern such as "8.0.0 - 8.5.9", "- 7.2.0", "8.1.0 -" or an exact "8.2.3".
// An empty target version satisfies nothing.
func versionInRange(version, pattern string) bool {
	if version == "" {
		return false
	}

	low, high, isRange := strings.Cut(pattern, "-")
	if !isRange {
		return strings.TrimSpace(pattern) == version
	}

	low = strings.TrimSpace(low)
	high = strings.TrimSpace(high)

	if low != "" && compareVersion(version, low) < 0 {
		return false
	}

	if high != "" && compareVersion(version, high) > 0 {
		return false
	}

	return true
}

// compareVersion compares dotted numeric versions component by component.
func compareVersion(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	for idx := range max(len(aParts), len(bParts)) {
		var aPart, bPart string

		if idx < len(aParts) {
			aPart = aParts[idx]
		}

		if idx < len(bParts) {
			bPart = bParts[idx]
		}

		aNum, aErr := strconv.Atoi(aPart)
		bNum, bErr := strconv.Atoi(bPart)

		if aErr == nil && bErr == nil {
			if aNum != bNum {
				return cmpInt(aNum, bNum)
			}

			continue
		}

		if result := strings.Compare(aPart, bPart); result != 0 {
			return result
		}
	}

	return 0
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
