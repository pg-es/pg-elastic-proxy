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
	errUnknownStep = errors.New("unknown step type")
)

// DoAction captures the ES action name and its parameters.
type DoAction struct {
	Name   string
	Params map[string]any
}

// Assertion captures an assertion type and its expected value.
type Assertion struct {
	Type  string // "match", "is_true", "is_false", "length", "gte", "lte"
	Value any
}

// Step is one element in a test section — exactly one of Do or Assert is non-nil.
type Step struct {
	Do     *DoAction
	Assert *Assertion
}

// TestSection is a named test with its ordered steps.
type TestSection struct {
	Name  string
	Steps []Step
}

// TestFile is a parsed ES YAML compatibility test file.
type TestFile struct {
	Setup    []Step
	Teardown []Step
	Tests    []TestSection
}

// ParseTestFile parses an ES client YAML test file from r.
func ParseTestFile(r io.Reader) (TestFile, error) {
	var root yaml.Node

	dec := yaml.NewDecoder(r)

	err := dec.Decode(&root)
	if err != nil {
		return TestFile{}, fmt.Errorf("decode yaml: %w", err)
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return TestFile{}, errNotDocument
	}

	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return TestFile{}, errNotMapping
	}

	return buildTestFile(mapping)
}

func buildTestFile(mapping *yaml.Node) (TestFile, error) {
	var tf TestFile

	for idx := 0; idx+1 < len(mapping.Content); idx += 2 {
		keyNode := mapping.Content[idx]
		valNode := mapping.Content[idx+1]

		steps, err := parseSteps(valNode)
		if err != nil {
			return TestFile{}, fmt.Errorf("parse section %q: %w", keyNode.Value, err)
		}

		switch keyNode.Value {
		case "setup":
			tf.Setup = steps
		case "teardown":
			tf.Teardown = steps
		default:
			tf.Tests = append(tf.Tests, TestSection{Name: keyNode.Value, Steps: steps})
		}
	}

	return tf, nil
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
	if stepNode.Kind != yaml.MappingNode || len(stepNode.Content) < 2 {
		return Step{}, errBadStep
	}

	key := stepNode.Content[0].Value
	val := stepNode.Content[1]

	switch key {
	case "do":
		da, err := parseDoStep(val)
		if err != nil {
			return Step{}, err
		}

		return Step{Do: &da}, nil
	case "match", "is_true", "is_false", "length", "gte", "lte":
		as, err := parseAssertionStep(key, val)
		if err != nil {
			return Step{}, err
		}

		return Step{Assert: &as}, nil
	default:
		return Step{}, fmt.Errorf("%w: %q", errUnknownStep, key)
	}
}

func parseDoStep(val *yaml.Node) (DoAction, error) {
	if val.Kind != yaml.MappingNode || len(val.Content) < 2 {
		return DoAction{}, errBadDoAction
	}

	actionName := val.Content[0].Value
	paramsNode := val.Content[1]

	var params map[string]any

	err := paramsNode.Decode(&params)
	if err != nil {
		return DoAction{}, fmt.Errorf("decode action params: %w", err)
	}

	return DoAction{Name: actionName, Params: params}, nil
}

func parseAssertionStep(assertType string, val *yaml.Node) (Assertion, error) {
	var decoded any

	err := val.Decode(&decoded)
	if err != nil {
		return Assertion{}, fmt.Errorf("decode assertion value: %w", err)
	}

	return Assertion{Type: assertType, Value: decoded}, nil
}
