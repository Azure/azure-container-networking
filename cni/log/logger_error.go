package log

type ErrorWithoutStackTrace struct {
	Err error
}

func (se ErrorWithoutStackTrace) Error() string {
	if se.Err == nil {
		return ""
	}
	return se.Err.Error()
}
