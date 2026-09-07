// Package enginegrant provides opt-in authorization for external provider apps.
// Built-in Emisell shipping does not depend on Apps Platform.
package enginegrant

import (
	"errors"
	"regexp"
)

var ErrDenied = errors.New("external app grant is not active")
var ErrUnavailable = errors.New("external app grant service unavailable")
var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,127}$`)
