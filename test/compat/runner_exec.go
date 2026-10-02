package compat

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// varPattern matches $name variable references embedded in a string.
var varPattern = regexp.MustCompile(`\$([A-Za-z_][A-Za-z0-9_]*)`)

// braceVarPattern matches ${name} references, which the ES format uses for
// values substituted into headers.
var braceVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// exactVarPattern matches a string that is nothing but a single $name or
// ${name} reference. Such a string resolves to the captured value itself,
// preserving its type; embedded references are stringified instead.
var exactVarPattern = regexp.MustCompile(`^\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?$`)

// FailureDetail records the details of a single test step failure.
type FailureDetail struct {
	TestName  string
	StepIndex int
	Err       string
}

// FileResult summarises the outcome of running one test file.
type FileResult struct {
	Passed   int
	Failed   int
	Skipped  int
	Failures []FailureDetail
}

// RunFile executes every test section in file for target. Per the ES format,
// setup runs before each section and teardown after it, and indices are cleared
// between sections.
func RunFile(ctx context.Context, file *TestFile, executor *Executor, target Target) FileResult {
	var result FileResult

	if fileGate(file, target) {
		result.Skipped++

		return result
	}

	for _, section := range file.Tests {
		if !runWithSetup(ctx, file, section, executor, target, &result) {
			continue
		}
	}

	return result
}

// runWithSetup runs setup, the section, teardown and cleanup. It reports
// whether the section ran (false when setup failed).
func runWithSetup(
	ctx context.Context,
	file *TestFile,
	section TestSection,
	executor *Executor,
	target Target,
	result *FileResult,
) bool {
	setupErr := executeSteps(ctx, file.Setup, executor, target.Flavour)
	if setupErr != nil {
		result.Failed++
		result.Failures = append(result.Failures, FailureDetail{
			TestName: section.Name + "/setup",
			Err:      setupErr.Error(),
		})

		_ = executeSteps(ctx, file.Teardown, executor, target.Flavour)
		clearIndices(ctx, executor, target.Flavour)

		return false
	}

	runSection(ctx, section, executor, target, result)

	_ = executeSteps(ctx, file.Teardown, executor, target.Flavour)
	clearIndices(ctx, executor, target.Flavour)

	return true
}

// clearIndices deletes all indices between sections, matching the ES format.
// The target may not support it; failures are ignored.
func clearIndices(ctx context.Context, executor *Executor, flavour string) {
	_, _ = executor.Execute(ctx, &DoAction{
		Name:   ActionIndicesDelete,
		Params: map[string]any{ParamIndex: "_all"},
	}, flavour)
}

// fileGate reports whether setup or teardown carries a requires/skip gate that
// skips the whole file.
func fileGate(file *TestFile, target Target) bool {
	for _, steps := range [][]Step{file.Setup, file.Teardown} {
		if stepsGate(steps, target) {
			return true
		}
	}

	return false
}

func stepsGate(steps []Step, target Target) bool {
	for _, step := range steps {
		if step.Requires != nil && !target.SatisfiesRequires(step.Requires) {
			return true
		}

		if step.Skip != nil && target.ShouldSkip(step.Skip) {
			return true
		}
	}

	return false
}

func runSection(ctx context.Context, section TestSection, executor *Executor, target Target, result *FileResult) {
	state := sectionState{vars: map[string]any{}}

	for idx, step := range section.Steps {
		if state.runStep(ctx, section.Name, idx, step, executor, target, result) {
			return
		}

		if state.skipped {
			result.Skipped++

			return
		}
	}

	result.Passed++
}

// sectionState carries the variables and last response across a section's steps.
type sectionState struct {
	vars         map[string]any
	lastResponse any
	skipped      bool
}

// runStep executes one step and reports whether the section should stop early.
func (st *sectionState) runStep(
	ctx context.Context,
	name string,
	idx int,
	step Step,
	executor *Executor,
	target Target,
	result *FileResult,
) bool {
	switch {
	case step.Skip != nil:
		if target.ShouldSkip(step.Skip) {
			result.Skipped++

			return true
		}

		return false
	case step.Requires != nil:
		if !target.SatisfiesRequires(step.Requires) {
			result.Skipped++

			return true
		}

		return false
	case step.Do != nil:
		return st.runDo(ctx, name, idx, step.Do, executor, target.Flavour, result)
	case step.Set != nil:
		return st.runSet(name, idx, step.Set, result)
	case step.Assert != nil:
		return st.runAssert(name, idx, step.Assert, result)
	default:
		return false
	}
}

func (st *sectionState) runDo(
	ctx context.Context,
	name string,
	idx int,
	do *DoAction,
	executor *Executor,
	flavour string,
	result *FileResult,
) bool {
	resp, err := executor.Execute(ctx, substituteAction(do, st.vars), flavour)

	if errors.Is(err, ErrSkip) {
		if do.Catch != "" {
			result.Skipped++

			return true
		}

		st.skipped = true

		return false
	}

	if err != nil {
		recordFailure(result, name, idx, fmt.Sprintf("execute %q: %s", do.Name, err))

		return true
	}

	if do.Catch != "" {
		catchErr := checkCatch(resp, do.Catch)
		if catchErr != nil {
			recordFailure(result, name, idx, fmt.Sprintf("catch %q: %s", do.Catch, catchErr))

			return true
		}

		return false
	}

	warnErr := checkWarnings(resp, do)
	if warnErr != nil {
		recordFailure(result, name, idx, fmt.Sprintf("warnings: %s", warnErr))

		return true
	}

	st.lastResponse = resp.Body
	st.vars["body"] = bodyStash(resp)

	return false
}

// bodyStash is the value of $body: the parsed response when it is JSON, else
// the raw text (cat APIs and other non-JSON bodies).
func bodyStash(resp Response) any {
	if resp.Body != nil {
		return resp.Body
	}

	return resp.Raw
}

// checkWarnings verifies Warning headers: an explicit `warnings` list must
// match exactly; otherwise every returned warning must be in `allowed_warnings`.
func checkWarnings(resp Response, do *DoAction) error {
	if len(do.Warnings) > 0 {
		if !sameStrings(do.Warnings, resp.Warnings) {
			return fmt.Errorf("expected warnings %v, got %v: %w", do.Warnings, resp.Warnings, errValueMismatch)
		}

		return nil
	}

	for _, warning := range resp.Warnings {
		if !containsString(do.AllowedWarnings, warning) {
			return fmt.Errorf("unexpected warning %q: %w", warning, errValueMismatch)
		}
	}

	return nil
}

func sameStrings(want, got []string) bool {
	if len(want) != len(got) {
		return false
	}

	used := make([]bool, len(got))

	for _, target := range want {
		found := false

		for i, candidate := range got {
			if used[i] || candidate != target {
				continue
			}

			used[i] = true
			found = true

			break
		}

		if !found {
			return false
		}
	}

	return true
}

func containsString(list []string, target string) bool {
	return slices.Contains(list, target)
}

func (st *sectionState) runSet(name string, idx int, set *SetStep, result *FileResult) bool {
	if st.lastResponse == nil {
		result.Skipped++

		return true
	}

	val, found := setValue(set, st.lastResponse, st.vars)
	if !found {
		recordFailure(result, name, idx, fmt.Sprintf("set %q: %s", set.Path, errPathNotFound))

		return true
	}

	st.vars[set.Name] = val

	return false
}

// setValue resolves a set/transform_and_set step: a transform expression is
// applied, the _arbitrary_key_ suffix picks any key of a map, otherwise the
// path is looked up in the response.
func setValue(set *SetStep, response any, vars map[string]any) (any, bool) {
	if set.Transform != "" {
		return applyTransform(set.Transform, response, vars)
	}

	if parent, ok := strings.CutSuffix(set.Path, "._arbitrary_key_"); ok {
		return arbitraryKey(response, parent, vars)
	}

	return GetPath(response, set.Path, vars)
}

func arbitraryKey(response any, parent string, vars map[string]any) (any, bool) {
	node, found := GetPath(response, parent, vars)
	if !found {
		return nil, false
	}

	entries, ok := node.(map[string]any)
	if !ok {
		return nil, false
	}

	for key := range entries {
		return key, true
	}

	return nil, false
}

// applyTransform implements the transform_and_set expressions. Only
// base64EncodeCredentials(user,password) is defined by the ES format; any other
// expression is stashed as a plain path lookup.
func applyTransform(expr string, response any, vars map[string]any) (any, bool) {
	const prefix = "#base64EncodeCredentials("

	if strings.HasPrefix(expr, prefix) && strings.HasSuffix(expr, ")") {
		inner := strings.TrimSuffix(strings.TrimPrefix(expr, prefix), ")")
		fields := strings.Split(inner, ",")
		credentials := make([]string, 0, len(fields))

		for _, field := range fields {
			value, ok := GetPath(response, strings.TrimSpace(field), vars)
			if !ok {
				return nil, false
			}

			credentials = append(credentials, fmt.Sprint(value))
		}

		encoded := base64.StdEncoding.EncodeToString([]byte(strings.Join(credentials, ":")))

		return encoded, true
	}

	return GetPath(response, expr, vars)
}

func (st *sectionState) runAssert(name string, idx int, assertion *Assertion, result *FileResult) bool {
	assertErr := CheckAssertion(st.lastResponse, *assertion, st.vars)
	if assertErr != nil {
		recordFailure(result, name, idx, fmt.Sprintf("assert %q: %s", assertion.Type, assertErr))

		return true
	}

	return false
}

// catchStatus returns the HTTP status a catch shorthand requires.
func catchStatus(pattern string) (int, bool) {
	switch pattern {
	case "bad_request":
		return http.StatusBadRequest, true
	case "unauthorized":
		return http.StatusUnauthorized, true
	case "forbidden":
		return http.StatusForbidden, true
	case "missing":
		return http.StatusNotFound, true
	case "request_timeout":
		return http.StatusRequestTimeout, true
	case "conflict":
		return http.StatusConflict, true
	case "unavailable":
		return http.StatusServiceUnavailable, true
	default:
		return 0, false
	}
}

// checkCatch verifies that a response is the error a catch pattern expects. A
// /regex/ pattern is matched against the serialized error; a named shorthand
// (missing, forbidden, ...) requires its status code; any other value (an ES
// error type, or "request") accepts any 4xx/5xx error.
func checkCatch(resp Response, pattern string) error {
	regex, isRegex := regexPattern(pattern)

	if isRegex {
		if resp.Status < http.StatusBadRequest {
			return fmt.Errorf("expected error matching %q but request succeeded: %w", pattern, errValueMismatch)
		}

		matched, err := regexp.MatchString(regex, errorText(resp))
		if err != nil {
			return fmt.Errorf("invalid catch pattern %q: %w", pattern, err)
		}

		if !matched {
			return fmt.Errorf("error %q does not match %q: %w", errorText(resp), pattern, errValueMismatch)
		}

		return nil
	}

	if status, ok := catchStatus(pattern); ok {
		if resp.Status != status {
			return fmt.Errorf("expected status %d for %q, got %d: %w", status, pattern, resp.Status, errValueMismatch)
		}

		return nil
	}

	code, codeErr := strconv.Atoi(pattern)
	if codeErr == nil {
		if resp.Status != code {
			return fmt.Errorf("expected status %d for %q, got %d: %w", code, pattern, resp.Status, errValueMismatch)
		}

		return nil
	}

	if resp.Status < http.StatusBadRequest {
		return fmt.Errorf("expected error for %q but request succeeded: %w", pattern, errValueMismatch)
	}

	return nil
}

// errorText joins the raw body and the error type/reason/root causes so a catch
// regex can match whatever part of the serialized error it targets.
func errorText(resp Response) string {
	parts := []string{resp.Raw}

	for _, path := range []string{"error.type", "error.reason", "error.root_cause"} {
		if val, ok := GetPath(resp.Body, path, nil); ok {
			parts = append(parts, fmt.Sprint(val))
		}
	}

	return strings.Join(parts, "\n")
}

func substituteAction(action *DoAction, vars map[string]any) *DoAction {
	if len(vars) == 0 {
		return action
	}

	if action.Params != nil {
		action.Params = substituteMap(action.Params, vars)
	}

	if action.Headers != nil {
		headers := make(map[string]string, len(action.Headers))
		for key, value := range action.Headers {
			headers[key] = substituteString(value, vars)
		}

		action.Headers = headers
	}

	return action
}

func substituteMap(params, vars map[string]any) map[string]any {
	out := make(map[string]any, len(params))

	for key, val := range params {
		out[substituteKey(key, vars)] = substituteValue(val, vars)
	}

	return out
}

func substituteKey(key string, vars map[string]any) string {
	if match := exactVarPattern.FindStringSubmatch(key); match != nil {
		if replacement, ok := vars[match[1]]; ok {
			return fmt.Sprint(replacement)
		}
	}

	return key
}

func substituteValue(val any, vars map[string]any) any {
	switch typed := val.(type) {
	case string:
		if match := exactVarPattern.FindStringSubmatch(typed); match != nil {
			if replacement, ok := vars[match[1]]; ok {
				return replacement
			}

			return typed
		}

		return substituteString(typed, vars)
	case map[string]any:
		return substituteMap(typed, vars)
	case []any:
		out := make([]any, len(typed))
		for idx, elem := range typed {
			out[idx] = substituteValue(elem, vars)
		}

		return out
	default:
		return val
	}
}

func substituteString(val string, vars map[string]any) string {
	val = braceVarPattern.ReplaceAllStringFunc(val, func(match string) string {
		replacement, ok := vars[braceVarPattern.FindStringSubmatch(match)[1]]
		if !ok {
			return match
		}

		return fmt.Sprint(replacement)
	})

	return varPattern.ReplaceAllStringFunc(val, func(match string) string {
		replacement, ok := vars[match[1:]]
		if !ok {
			return match
		}

		return fmt.Sprint(replacement)
	})
}

func recordFailure(result *FileResult, testName string, stepIdx int, errMsg string) {
	result.Failed++
	result.Failures = append(result.Failures, FailureDetail{
		TestName:  testName,
		StepIndex: stepIdx,
		Err:       errMsg,
	})
}

func executeSteps(ctx context.Context, steps []Step, executor *Executor, flavour string) error {
	for _, step := range steps {
		if step.Do == nil {
			continue
		}

		_, err := executor.Execute(ctx, step.Do, flavour)
		if err != nil && !errors.Is(err, ErrSkip) {
			return fmt.Errorf("step %q: %w", step.Do.Name, err)
		}
	}

	return nil
}
