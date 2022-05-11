package internal

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	MissingFields []string
}

func (v ValidationError) Error() string {
	return fmt.Sprintf("missing fields: %s", strings.Join(v.MissingFields, ", "))
}

func (v ValidationError) IsEmpty() bool {
	return len(v.MissingFields) == 0
}
