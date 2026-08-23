package compat

import (
	"context"
	"errors"
	"fmt"
)

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

// RunFile executes all tests in file against executor, skipping entries in skiplist.
// Setup runs first; teardown always runs even if tests fail.
func RunFile(ctx context.Context, file TestFile, skiplist Skiplist, executor *Executor) FileResult {
	var result FileResult

	setupErr := executeSteps(ctx, file.Setup, executor)
	if setupErr != nil {
		result.Failed++
		result.Failures = append(result.Failures, FailureDetail{
			TestName: "setup",
			Err:      setupErr.Error(),
		})

		// Teardown still runs on setup failure.
		_ = executeSteps(ctx, file.Teardown, executor)

		return result
	}

	defer func() {
		_ = executeSteps(ctx, file.Teardown, executor)
	}()

	for _, section := range file.Tests {
		if skip, _ := skiplist.ShouldSkip(section.Name); skip {
			result.Skipped++

			continue
		}

		runSection(ctx, section, executor, &result)
	}

	return result
}

func runSection(ctx context.Context, section TestSection, executor *Executor, result *FileResult) {
	var lastResponse map[string]any

	for idx, step := range section.Steps {
		if step.Do != nil {
			resp, _, err := executor.Execute(ctx, *step.Do)

			if errors.Is(err, ErrSkip) {
				continue
			}

			if err != nil {
				recordFailure(result, section.Name, idx, fmt.Sprintf("execute %q: %s", step.Do.Name, err))

				return
			}

			lastResponse = resp
		}

		if step.Assert != nil {
			assertErr := CheckAssertion(lastResponse, *step.Assert)
			if assertErr != nil {
				recordFailure(result, section.Name, idx, fmt.Sprintf("assert %q: %s", step.Assert.Type, assertErr))

				return
			}
		}
	}

	result.Passed++
}

func recordFailure(result *FileResult, testName string, stepIdx int, errMsg string) {
	result.Failed++
	result.Failures = append(result.Failures, FailureDetail{
		TestName:  testName,
		StepIndex: stepIdx,
		Err:       errMsg,
	})
}

func executeSteps(ctx context.Context, steps []Step, executor *Executor) error {
	for _, step := range steps {
		if step.Do == nil {
			continue
		}

		_, _, err := executor.Execute(ctx, *step.Do)
		if err != nil && !errors.Is(err, ErrSkip) {
			return fmt.Errorf("step %q: %w", step.Do.Name, err)
		}
	}

	return nil
}
