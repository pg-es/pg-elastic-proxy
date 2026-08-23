package utils

import (
	"fmt"
	"net/http"
)

// ElasticError is a basic interface for any kind of errors produced by pg-elastic
// and can be represented as a JSON error report.
type ElasticError interface {
	error
	Type() string
	Reason() string
	StatusCode() int
	FormatErrorResponse() any
}

// ElasticGeneralError represents a response format for JSON error report 'cause' part.
type ElasticGeneralError struct {
	TypeVal   string `json:"type"`
	ReasonVal string `json:"reason"`
}

func (err *ElasticGeneralError) Error() string {
	return fmt.Sprintf("Error type: %s, Reason: %s", err.Type(), err.Reason())
}

// StatusCode returns the HTTP status code for this error.
func (err *ElasticGeneralError) StatusCode() int {
	return http.StatusInternalServerError
}

// Type returns name of the error type.
func (err *ElasticGeneralError) Type() string {
	return err.TypeVal
}

// Reason returns the error reason.
func (err *ElasticGeneralError) Reason() string {
	return err.ReasonVal
}

// FormatErrorResponse generates a JSON output for an error.
func (err *ElasticGeneralError) FormatErrorResponse() any {
	output := make(map[string]any)
	errorDesc := elasticGeneralErrorResponse{
		RootCause: []ElasticGeneralError{
			{TypeVal: err.Type(), ReasonVal: err.Reason()},
		},
		TypeVal:   err.Type(),
		ReasonVal: err.Reason(),
	}
	output["error"] = errorDesc
	output["status"] = err.StatusCode()

	return output
}

// elasticGeneralErrorResponse represents a response format for JSON error report.
type elasticGeneralErrorResponse struct {
	RootCause []ElasticGeneralError `json:"root_cause"`
	TypeVal   string                `json:"type"`
	ReasonVal string                `json:"reason"`
}

// ElasticBulkError represents a response format for JSON error report for bulk requests.
type ElasticBulkError struct {
	ElasticGeneralError

	Index     string `json:"index"`
	Shard     string `json:"shard"`
	IndexUUID string `json:"index_uuid"`
}

// NewElasticBulkError creates a new instance of ElasticBulkError.
func NewElasticBulkError(err ElasticError, index, shard, indexUUID string) *ElasticBulkError {
	output := &ElasticBulkError{}
	output.TypeVal = err.Type()
	output.ReasonVal = err.Reason()
	output.Index = index
	output.IndexUUID = indexUUID
	output.Shard = shard

	return output
}

// FormatErrorResponse generates a JSON output for a bulk error.
func (err *ElasticBulkError) FormatErrorResponse() any {
	output := make(map[string]any)
	output["error"] = err

	return output
}

// JSONWrongFormatError is error caused by illegal JSON input.
type JSONWrongFormatError struct {
	ElasticGeneralError
}

// NewJSONWrongFormatError creates a new instance of JSONWrongFormatError.
func NewJSONWrongFormatError(reason string) *JSONWrongFormatError {
	return &JSONWrongFormatError{ElasticGeneralError{TypeVal: "json_parse_exception", ReasonVal: reason}}
}

// StatusCode returns 400 for malformed JSON input.
func (err *JSONWrongFormatError) StatusCode() int {
	return http.StatusBadRequest
}

// DBQueryError is error caused by any internal error of the database.
type DBQueryError struct {
	ElasticGeneralError
}

// NewDBQueryError creates a new instance of DBQueryError.
func NewDBQueryError(reason string) *DBQueryError {
	return &DBQueryError{ElasticGeneralError{TypeVal: "db_query_exception", ReasonVal: reason}}
}

// InternalIOError is error caused by any other IO operations.
type InternalIOError struct {
	ElasticGeneralError
}

// NewInternalIOError creates a new instance of InternalIOError.
func NewInternalIOError(reason string) *InternalIOError {
	return &InternalIOError{ElasticGeneralError{TypeVal: "internal_io_exception", ReasonVal: reason}}
}

// InternalError is error caused by pg-elastic itself.
type InternalError struct {
	ElasticGeneralError
}

// NewInternalError creates a new instance of InternalError.
func NewInternalError(reason string) *InternalError {
	return &InternalError{ElasticGeneralError{TypeVal: "internal_exception", ReasonVal: reason}}
}

// IllegalQueryError is error caused by misconstructed query.
type IllegalQueryError struct {
	ElasticGeneralError
}

// NewIllegalQueryError creates a new instance of IllegalQueryError.
func NewIllegalQueryError(reason string) *IllegalQueryError {
	return &IllegalQueryError{ElasticGeneralError{TypeVal: "illegal_argument_exception", ReasonVal: reason}}
}

// StatusCode returns 400 for illegal query input.
func (err *IllegalQueryError) StatusCode() int {
	return http.StatusBadRequest
}

// ResourceAlreadyExistsError is returned when an index or type already exists.
type ResourceAlreadyExistsError struct {
	ElasticGeneralError
}

// NewResourceAlreadyExistsError creates a new instance of ResourceAlreadyExistsError.
func NewResourceAlreadyExistsError(reason string) *ResourceAlreadyExistsError {
	return &ResourceAlreadyExistsError{ElasticGeneralError{TypeVal: "resource_already_exists_exception", ReasonVal: reason}}
}

// StatusCode returns 400 for resource-already-exists errors.
func (err *ResourceAlreadyExistsError) StatusCode() int {
	return http.StatusBadRequest
}

// VersionConflictError is returned when a document version conflict occurs.
type VersionConflictError struct {
	ElasticGeneralError
}

// NewVersionConflictError creates a new instance of VersionConflictError.
func NewVersionConflictError(reason string) *VersionConflictError {
	return &VersionConflictError{ElasticGeneralError{TypeVal: "version_conflict_engine_exception", ReasonVal: reason}}
}

// StatusCode returns 409 for version conflict errors.
func (err *VersionConflictError) StatusCode() int {
	return http.StatusConflict
}
