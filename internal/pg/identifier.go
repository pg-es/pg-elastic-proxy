package pg

import (
	"errors"
	"fmt"
	"regexp"
)

// maxIdentifierLen is the ES 255-byte limit for index and type names.
const maxIdentifierLen = 255

// identifierRe accepts the ES character set and allows a leading underscore,
// which type names like `_doc` and combined index_type names need.
var identifierRe = regexp.MustCompile(`^[a-z0-9._][a-z0-9_.+-]*$`)

// indexRe also rejects a leading underscore: index names may not start with
// '-', '_', or '+'.
var indexRe = regexp.MustCompile(`^[a-z0-9.][a-z0-9_.+-]*$`)

// ErrInvalidIdentifier reports a name that cannot be embedded directly in DDL.
var ErrInvalidIdentifier = errors.New("invalid identifier")

func rejectInvalid(name string, re *regexp.Regexp) error {
	if name == "." || name == ".." {
		return fmt.Errorf("%w %q: must not be '.' or '..'", ErrInvalidIdentifier, name)
	}

	if len(name) > maxIdentifierLen {
		return fmt.Errorf("%w %q: exceeds 255 bytes", ErrInvalidIdentifier, name)
	}

	if !re.MatchString(name) {
		return fmt.Errorf("%w %q: must match %s", ErrInvalidIdentifier, name, re.String())
	}

	return nil
}

// ValidateIdentifier is the safety gate for names embedded directly in DDL
// (type names, and combined index_type table/sequence names). It allows a
// leading underscore because `_doc` types need one.
func ValidateIdentifier(name string) error {
	return rejectInvalid(name, identifierRe)
}

// ValidateIndexName checks an ES index name: lowercase, and no leading
// '-', '_', or '+'.
func ValidateIndexName(name string) error {
	return rejectInvalid(name, indexRe)
}
