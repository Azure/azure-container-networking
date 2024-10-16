package refresh

type Logger interface {
	Debugf(format string, v ...interface{})
	Printf(format string, v ...interface{})
	Warnf(format string, v ...interface{})
	Errorf(format string, v ...interface{})
}
