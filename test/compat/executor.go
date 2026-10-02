package compat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrSkip is returned by Execute when the action is not supported.
var ErrSkip = errors.New("action not supported")

var (
	errBuildBody    = errors.New("failed to build request body")
	errBuildRequest = errors.New("failed to build HTTP request")
	errDoRequest    = errors.New("failed to execute HTTP request")
	errReadBody     = errors.New("failed to read response body")
)

// Action names from the ES client test format used elsewhere in the package.
const (
	ActionIndex           = "index"
	ActionGet             = "get"
	ActionDelete          = "delete"
	ActionSearch          = "search"
	ActionIndicesCreate   = "indices.create"
	ActionIndicesDelete   = "indices.delete"
	ActionIndicesExists   = "indices.exists"
	ActionBulk            = "bulk"
	ActionMsearch         = "msearch"
	ActionMsearchTemplate = "msearch_template"
)

// Action parameter keys.
const (
	ParamIndex = "index"
	ParamID    = "id"
	ParamBody  = "body"
)

// ActionPath is one URL template an ES action accepts, with the HTTP methods it
// allows and the ordered path-parameter names embedded in the template.
type ActionPath struct {
	Template string
	Methods  []string
	Parts    []string
}

// ActionRoute maps an ES action name to the REST paths it accepts and the
// request parameters that become query-string arguments.
type ActionRoute struct {
	Paths       []ActionPath
	QueryParams []string
}

// Response is the outcome of executing one action: the parsed JSON body (nil
// when the body is not JSON, e.g. the cat APIs), the raw body text, the Warning
// headers, and the HTTP status code.
type Response struct {
	Body     any
	Raw      string
	Warnings []string
	Status   int
}

// Executor translates DoAction values into HTTP requests against an ES-compatible proxy.
type Executor struct {
	baseURL string
	client  *http.Client
}

// NewExecutor creates an Executor targeting baseURL with the given HTTP client.
func NewExecutor(baseURL string, client *http.Client) *Executor {
	if client == nil {
		client = http.DefaultClient
	}

	return &Executor{baseURL: baseURL, client: client}
}

// FlavourES8 and FlavourES9 select the action registry for the target's ES major.
const (
	FlavourES8 = "es8"
	FlavourES9 = "es9"
)

// routesFor returns the action registry for a flavour. Unknown or empty
// flavours fall back to the current major (es9).
func routesFor(flavour string) map[string]ActionRoute {
	if flavour == FlavourES8 {
		return actionRoutesES8
	}

	return actionRoutesES9
}

// Execute translates action into an HTTP request for the given flavour, sends
// it, and returns the response. Unknown actions return ErrSkip.
func (ex *Executor) Execute(ctx context.Context, action *DoAction, flavour string) (Response, error) {
	route, ok := routesFor(flavour)[action.Name]
	if !ok {
		return Response{}, ErrSkip
	}

	path, method := route.resolve(action.Params)
	reqURL := ex.baseURL + path

	if query := route.queryString(action.Params); query != "" {
		reqURL += "?" + query
	}

	body, err := buildBody(action)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %w", errBuildBody, err)
	}

	return ex.doHTTP(ctx, method, reqURL, action.Headers, body, isNDJSON(action.Name))
}

// resolve picks the path variant whose parameters are all present in params,
// preferring the most specific (most parts); it substitutes the parameter
// values into the template and returns the first allowed HTTP method.
func (r ActionRoute) resolve(params map[string]any) (path, method string) {
	chosen := r.Paths[0]
	chosenParts := -1

	for _, candidate := range r.Paths {
		if !candidate.applies(params) {
			continue
		}

		if len(candidate.Parts) > chosenParts {
			chosen = candidate
			chosenParts = len(candidate.Parts)
		}
	}

	if chosenParts == -1 {
		for _, candidate := range r.Paths {
			if len(candidate.Parts) < len(chosen.Parts) {
				chosen = candidate
			}
		}
	}

	path = chosen.Template
	for _, part := range chosen.Parts {
		path = strings.ReplaceAll(path, "{"+part+"}", pathPartValue(params[part]))
	}

	return path, chosen.Methods[0]
}

func (p ActionPath) applies(params map[string]any) bool {
	for _, part := range p.Parts {
		if pathPartValue(params[part]) == "" {
			return false
		}
	}

	return true
}

// queryString renders the route's query parameters that are present in params.
func (r ActionRoute) queryString(params map[string]any) string {
	values := url.Values{}

	for _, key := range r.QueryParams {
		raw, ok := params[key]
		if !ok {
			continue
		}

		for _, item := range queryValues(raw) {
			values.Add(key, item)
		}
	}

	return values.Encode()
}

func (ex *Executor) doHTTP(
	ctx context.Context,
	method, reqURL string,
	headers map[string]string,
	body []byte,
	ndjson bool,
) (Response, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %w", errBuildRequest, err)
	}

	if body != nil {
		req.Header.Set("Content-Type", jsonContentType(ndjson))
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := ex.client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("%w: %w", errDoRequest, err)
	}

	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return Response{Status: resp.StatusCode}, fmt.Errorf("%w: %w", errReadBody, err)
	}

	return Response{
		Body:     parseBody(raw),
		Raw:      string(raw),
		Warnings: resp.Header.Values("Warning"),
		Status:   resp.StatusCode,
	}, nil
}

// parseBody decodes a JSON object body; non-JSON bodies (cat APIs, HEAD
// responses) yield nil so callers fall back to the raw text.
func parseBody(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}

	var parsed any

	err := json.Unmarshal(raw, &parsed)
	if err != nil {
		return nil
	}

	return parsed
}

func buildBody(action *DoAction) ([]byte, error) {
	body := action.Params[ParamBody]

	if raw, ok := body.(string); ok {
		return []byte(raw), nil
	}

	if action.Name == ActionBulk {
		return marshalNDJSON(body)
	}

	return marshalJSON(body)
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

func pathPartValue(val any) string {
	switch typed := val.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, len(typed))
		for i, item := range typed {
			parts[i] = fmt.Sprint(item)
		}

		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(typed)
	}
}

func queryValues(val any) []string {
	switch typed := val.(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, len(typed))
		for i, item := range typed {
			out[i] = fmt.Sprint(item)
		}

		return out
	case bool:
		return []string{strconv.FormatBool(typed)}
	default:
		return []string{fmt.Sprint(typed)}
	}
}

func isNDJSON(action string) bool {
	switch action {
	case ActionBulk, ActionMsearch, ActionMsearchTemplate:
		return true
	default:
		return false
	}
}

func jsonContentType(ndjson bool) string {
	if ndjson {
		return "application/x-ndjson"
	}

	return "application/json"
}
