package dependencies_test

// Keep transitive golang.org/x modules direct so Dependabot updates them.
// Test-only imports keep these packages out of production binaries.
import (
	_ "golang.org/x/oauth2"
	_ "golang.org/x/term"
	_ "golang.org/x/text/transform"
	_ "golang.org/x/tools/go/ast/astutil"
	_ "golang.org/x/xerrors"
)
