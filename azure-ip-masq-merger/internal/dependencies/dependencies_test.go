package dependencies_test

// Keep transitive golang.org/x modules direct so Dependabot updates them.
// Test-only imports keep these packages out of production binaries.
import (
	_ "golang.org/x/net/http2/hpack"
	_ "golang.org/x/sys/cpu"
	_ "golang.org/x/text/transform"
)
