package compat

import (
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

var (
	errNotDocument = errors.New("expected YAML document node")
	errNotMapping  = errors.New("expected top-level YAML mapping")
	errNotSequence = errors.New("expected step sequence")
	errBadStep     = errors.New("step must be a single-key mapping")
	errBadDoAction = errors.New("do value must be a non-empty mapping")
	errBadSet      = errors.New("set value must be a single-entry mapping")
	errUnknownStep = errors.New("unknown step type")
)

const (
	stepDo           = "do"
	stepSet          = "set"
	stepSkip         = "skip"
	stepCatch        = "catch"
	stepHeaders      = "headers"
	stepWarnings     = "warnings"
	stepAllowedWarn  = "allowed_warnings"
	stepNodeSelector = "node_selector"
	stepTransformSet = "transform_and_set"
	keyRequires      = "requires"
	keySetup         = "setup"
	keyTeardown      = "teardown"
)

// DoAction captures the ES action name, its parameters, optional request
// headers, and an optional catch pattern. A non-empty Catch means the step
// expects the request to fail with an error matching the pattern.
type DoAction struct {
	Name            string
	Params          map[string]any
	Catch           string
	Headers         map[string]string
	Warnings        []string
	AllowedWarnings []string
	NodeSelector    any
}

// Assertion captures an assertion type and its expected value.
type Assertion struct {
	Type  string // one of the Assert* constants
	Value any
}

// SetStep captures a value from the previous response into a named variable,
// which later steps reference as $name. A non-empty Transform applies a
// transformation (e.g. base64EncodeCredentials) instead of a plain path lookup.
type SetStep struct {
	Path      string // dot-path into the previous response
	Name      string // variable name
	Transform string // optional transform expression, for transform_and_set
}

// SkipStep is a section-level skip gate. The runner evaluates the conditions
// against the target; Reason is diagnostic.
type SkipStep struct {
	Features           any          `yaml:"features"`
	TestRunnerFeatures any          `yaml:"test_runner_features"`
	Capabilities       []Capability `yaml:"capabilities"`
	ClusterFeatures    any          `yaml:"cluster_features"`
	AllNodes           bool         `yaml:"all_nodes"`
	KnownIssues        []KnownIssue `yaml:"known_issues"`
	AwaitsFix          string       `yaml:"awaits_fix"`
	OS                 any          `yaml:"os"`
	Version            string       `yaml:"version"`
	Reason             string       `yaml:"reason"`
}

// Step is one element in a test section — exactly one of Do, Assert, Set,
// Skip, or Requires is non-nil.
type Step struct {
	Do       *DoAction
	Assert   *Assertion
	Set      *SetStep
	Skip     *SkipStep
	Requires *Requires
}

// TestSection is a named test with its ordered steps.
type TestSection struct {
	Name  string
	Steps []Step
}

// Requires is a file- or section-level `requires:` gate. A nil or empty field
// does not constrain that dimension.
type Requires struct {
	Stack              *bool        `yaml:"stack"`
	Serverless         *bool        `yaml:"serverless"`
	Capabilities       []Capability `yaml:"capabilities"`
	ClusterFeatures    any          `yaml:"cluster_features"`
	TestRunnerFeatures any          `yaml:"test_runner_features"`
	Features           any          `yaml:"features"`
	OS                 any          `yaml:"os"`
	Version            string       `yaml:"version"`
}

// AppliesTo reports whether a file with these requirements should run against
// the given target flavour. The runtime flavours (es8/es9/os2/os3) are Stack
// backends; "serverless" selects Serverless-available files. A file that does
// not constrain the relevant dimension runs everywhere.
func (r *Requires) AppliesTo(flavour string) bool {
	if flavour == "serverless" {
		return r.Serverless == nil || *r.Serverless
	}

	return r.Stack == nil || *r.Stack
}

// TestFile is a parsed ES YAML compatibility test file.
type TestFile struct {
	Requires Requires
	Setup    []Step
	Teardown []Step
	Tests    []TestSection
}

// ParseTestFile parses an ES client YAML test file from r. Test files are
// multi-document YAML (`---`-separated): a `requires:` document, optional
// `setup`/`teardown` documents, and a document of named test sections.
func ParseTestFile(r io.Reader) (TestFile, error) {
	var tf TestFile

	dec := yaml.NewDecoder(r)

	for {
		var root yaml.Node

		err := dec.Decode(&root)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return TestFile{}, fmt.Errorf("decode yaml: %w", err)
		}

		if root.Kind != yaml.DocumentNode {
			return TestFile{}, errNotDocument
		}

		if len(root.Content) == 0 {
			continue
		}

		mapping := root.Content[0]
		if mapping.Kind != yaml.MappingNode {
			return TestFile{}, errNotMapping
		}

		err = mergeDocument(&tf, mapping)
		if err != nil {
			return TestFile{}, err
		}
	}

	return tf, nil
}

func mergeDocument(tf *TestFile, mapping *yaml.Node) error {
	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		keyNode := mapping.Content[idx]
		valNode := mapping.Content[idx+1]

		if keyNode.Value == keyRequires {
			err := valNode.Decode(&tf.Requires)
			if err != nil {
				return fmt.Errorf("parse requires: %w", err)
			}

			continue
		}

		steps, err := parseSteps(valNode)
		if err != nil {
			return fmt.Errorf("parse section %q: %w", keyNode.Value, err)
		}

		switch keyNode.Value {
		case keySetup:
			tf.Setup = append(tf.Setup, steps...)
		case keyTeardown:
			tf.Teardown = append(tf.Teardown, steps...)
		default:
			tf.Tests = append(tf.Tests, TestSection{Name: keyNode.Value, Steps: steps})
		}
	}

	return nil
}

func parseSteps(seqNode *yaml.Node) ([]Step, error) {
	if seqNode.Kind != yaml.SequenceNode {
		return nil, errNotSequence
	}

	steps := make([]Step, 0, len(seqNode.Content))

	for _, stepNode := range seqNode.Content {
		step, err := parseSingleStep(stepNode)
		if err != nil {
			return nil, err
		}

		steps = append(steps, step)
	}

	return steps, nil
}

func parseSingleStep(stepNode *yaml.Node) (Step, error) {
	key, val, ok := stepParts(stepNode)
	if !ok {
		return Step{}, errBadStep
	}

	if isAssertionKey(key) {
		as, err := parseAssertionStep(key, val)
		if err != nil {
			return Step{}, err
		}

		return Step{Assert: &as}, nil
	}

	switch key {
	case stepDo:
		da, err := parseDoStep(val)
		if err != nil {
			return Step{}, err
		}

		return Step{Do: &da}, nil
	case stepSet:
		ss, err := parseSetStep(val)
		if err != nil {
			return Step{}, err
		}

		return Step{Set: &ss}, nil
	case stepTransformSet:
		ss, err := parseTransformSetStep(val)
		if err != nil {
			return Step{}, err
		}

		return Step{Set: &ss}, nil
	case stepSkip:
		sk, err := parseSkipStep(val)
		if err != nil {
			return Step{}, err
		}

		return Step{Skip: &sk}, nil
	case keyRequires:
		var req Requires

		err := val.Decode(&req)
		if err != nil {
			return Step{}, fmt.Errorf("decode requires: %w", err)
		}

		return Step{Requires: &req}, nil
	default:
		return Step{}, fmt.Errorf("%w: %q", errUnknownStep, key)
	}
}

func stepParts(stepNode *yaml.Node) (key string, val *yaml.Node, ok bool) {
	if stepNode.Kind != yaml.MappingNode || len(stepNode.Content) < 2 {
		return "", nil, false
	}

	return stepNode.Content[0].Value, stepNode.Content[1], true
}

// isAssertionKey reports whether key names an assertion operator.
func isAssertionKey(key string) bool {
	switch key {
	case AssertMatch, AssertIsTrue, AssertIsFalse, AssertLength, AssertGTE, AssertLTE, AssertGT, AssertLT,
		AssertExists, AssertContains, AssertIsAfter, AssertCloseTo, AssertMatchesInAnyOrder:
		return true
	default:
		return false
	}
}

// parseDoStep reads a `do` mapping. Alongside the single action key it may carry
// a `catch` pattern and request `headers`.
func parseDoStep(val *yaml.Node) (DoAction, error) {
	if val.Kind != yaml.MappingNode || len(val.Content) < 2 {
		return DoAction{}, errBadDoAction
	}

	var (
		action DoAction
		found  bool
	)

	for idx := 0; idx+1 < len(val.Content); idx += 2 {
		key := val.Content[idx].Value
		node := val.Content[idx+1]

		handled, err := action.applySubKey(key, node)
		if err != nil {
			return DoAction{}, err
		}

		if handled {
			continue
		}

		if found {
			return DoAction{}, fmt.Errorf("%w: multiple action keys", errBadDoAction)
		}

		setErr := action.setAction(key, node)
		if setErr != nil {
			return DoAction{}, setErr
		}

		found = true
	}

	if !found {
		return DoAction{}, errBadDoAction
	}

	return action, nil
}

func (a *DoAction) applySubKey(key string, node *yaml.Node) (bool, error) {
	switch key {
	case stepCatch:
		return true, decodeInto(node, &a.Catch, "catch")
	case stepWarnings:
		return true, decodeInto(node, &a.Warnings, "warnings")
	case stepAllowedWarn:
		return true, decodeInto(node, &a.AllowedWarnings, "allowed_warnings")
	case stepNodeSelector:
		return true, decodeInto(node, &a.NodeSelector, "node_selector")
	case stepHeaders:
		headers, err := parseHeaders(node)
		if err != nil {
			return true, err
		}

		a.Headers = headers

		return true, nil
	default:
		return false, nil
	}
}

func (a *DoAction) setAction(key string, node *yaml.Node) error {
	var params map[string]any

	err := node.Decode(&params)
	if err != nil {
		return fmt.Errorf("decode action params: %w", err)
	}

	a.Name = key
	a.Params = params

	return nil
}

func decodeInto(node *yaml.Node, target any, field string) error {
	err := node.Decode(target)
	if err != nil {
		return fmt.Errorf("decode %s: %w", field, err)
	}

	return nil
}

func parseHeaders(node *yaml.Node) (map[string]string, error) {
	var raw map[string]any

	err := node.Decode(&raw)
	if err != nil {
		return nil, fmt.Errorf("decode headers: %w", err)
	}

	headers := make(map[string]string, len(raw))
	for key, val := range raw {
		headers[key] = fmt.Sprint(val)
	}

	return headers, nil
}

func parseSetStep(val *yaml.Node) (SetStep, error) {
	if val.Kind != yaml.MappingNode || len(val.Content) < 2 {
		return SetStep{}, errBadSet
	}

	return SetStep{
		Path: val.Content[0].Value,
		Name: val.Content[1].Value,
	}, nil
}

func parseTransformSetStep(val *yaml.Node) (SetStep, error) {
	if val.Kind != yaml.MappingNode || len(val.Content) < 2 {
		return SetStep{}, errBadSet
	}

	return SetStep{Name: val.Content[0].Value, Transform: val.Content[1].Value}, nil
}

func parseSkipStep(val *yaml.Node) (SkipStep, error) {
	var sk SkipStep

	err := val.Decode(&sk)
	if err != nil {
		return SkipStep{}, fmt.Errorf("decode skip: %w", err)
	}

	return sk, nil
}

func parseAssertionStep(assertType string, val *yaml.Node) (Assertion, error) {
	var decoded any

	err := val.Decode(&decoded)
	if err != nil {
		return Assertion{}, fmt.Errorf("decode assertion value: %w", err)
	}

	return Assertion{Type: assertType, Value: decoded}, nil
}
