package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// ErrSkip is returned by Execute when the action is not supported.
var ErrSkip = errors.New("action not supported")

var (
	errBuildBody    = errors.New("failed to build request body")
	errBuildRequest = errors.New("failed to build HTTP request")
	errDoRequest    = errors.New("failed to execute HTTP request")
	errReadBody     = errors.New("failed to read response body")
	errDecodeBody   = errors.New("failed to decode response body")
)

const (
	actionBulk = "bulk"
)

// Executor translates DoAction values into HTTP requests against an ES-compatible proxy.
type Executor struct {
	baseURL string
	client  *http.Client
}

// NewExecutor creates an Executor targeting baseURL with the given HTTP client.
func NewExecutor(baseURL string, client *http.Client) *Executor {
	return &Executor{baseURL: baseURL, client: client}
}

// Execute translates action into an HTTP request, sends it, and returns the parsed
// JSON response body, HTTP status code, and any error. Unknown actions return ErrSkip.
func (ex *Executor) Execute(ctx context.Context, action DoAction) (response map[string]any, statusCode int, execErr error) {
	method, reqPath, ok := actionRoute(action)
	if !ok {
		return nil, 0, ErrSkip
	}

	body, err := buildBody(action)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", errBuildBody, err)
	}

	return ex.doHTTP(ctx, method, ex.baseURL+reqPath, body, action.Name == actionBulk)
}

func (ex *Executor) doHTTP(
	ctx context.Context,
	method, reqURL string,
	body []byte,
	ndjson bool,
) (response map[string]any, statusCode int, err error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", errBuildRequest, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", jsonContentType(ndjson))
	}

	resp, err := ex.client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %w", errDoRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if method == http.MethodHead {
		return map[string]any{}, resp.StatusCode, nil
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: %w", errReadBody, err)
	}

	var result map[string]any

	err = json.Unmarshal(raw, &result)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("%w: %w", errDecodeBody, err)
	}

	return result, resp.StatusCode, nil
}

func actionRoute(action DoAction) (method, path string, ok bool) {
	params := action.Params
	idx, _ := params["index"].(string)
	docID, _ := params["id"].(string)

	switch action.Name {
	case "index":
		return http.MethodPut, docPath(idx, docID), true
	case "get":
		return http.MethodGet, docPath(idx, docID), true
	case "delete":
		return http.MethodDelete, docPath(idx, docID), true
	case "search":
		if idx == "" {
			return http.MethodPost, "/_search", true
		}

		return http.MethodPost, "/" + idx + "/_search", true
	case "indices.create":
		return http.MethodPut, "/" + idx, true
	case "indices.delete":
		return http.MethodDelete, "/" + idx, true
	case "indices.exists":
		return http.MethodHead, "/" + idx, true
	case actionBulk:
		return http.MethodPost, "/_bulk", true
	default:
		return "", "", false
	}
}

func buildBody(action DoAction) ([]byte, error) {
	switch action.Name {
	case actionBulk:
		return marshalNDJSON(action.Params["body"])
	case "index", "search":
		return marshalJSON(action.Params["body"])
	default:
		return nil, nil
	}
}

func marshalJSON(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}

	return data, nil
}

func marshalNDJSON(body any) ([]byte, error) {
	if body == nil {
		return nil, nil
	}

	ops, ok := body.([]any)
	if !ok {
		return nil, fmt.Errorf("bulk body must be a sequence: %w", errBuildBody)
	}

	var buf bytes.Buffer

	for _, op := range ops {
		line, err := json.Marshal(op)
		if err != nil {
			return nil, fmt.Errorf("marshal ndjson line: %w", err)
		}

		buf.Write(line)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

func docPath(index, id string) string {
	return "/" + index + "/_doc/" + id
}

func jsonContentType(ndjson bool) string {
	if ndjson {
		return "application/x-ndjson"
	}

	return "application/json"
}
