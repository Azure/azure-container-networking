package buildinfo

// this will be populate by the Go compiler via
// the -ldflags, which insert dynamic information
// into the binary at build time
var Version string

var (
	LogLevel         string
	OutputPaths      string // comma separated list of paths
	ErrorOutputPaths string // comma separated list of paths
)
