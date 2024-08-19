package log

import (
	"fmt"
	"io"
)

type ErrorWithoutStackTrace struct {
	error
}

func (l *ErrorWithoutStackTrace) Error() string {
	if l.error == nil {
		return ""
	}
	return l.error.Error()
}
func (l *ErrorWithoutStackTrace) Format(s fmt.State, verb rune) {
	// if the error is nil, nothing should happen
	if l.error == nil {
		return
	}
	v := verb
	// replace uses of %v with %s
	if v == 'v' {
		v = 's'
	}
	// if the error implements formatter (which it should)
	if fm, ok := l.error.(fmt.Formatter); ok {
		fm.Format(s, v)
	} else {
		io.WriteString(s, l.error.Error())
	}
}
func (l *ErrorWithoutStackTrace) Unwrap() error {
	return l.error
}

func NewErrorWithoutStackTrace(err error) *ErrorWithoutStackTrace {
	return &ErrorWithoutStackTrace{err}
}
