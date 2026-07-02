// Package command enforces a read-only allow/deny policy for the live kubectl
// diagnostics the agent runs against a retained failing cluster. The agent must
// never mutate cluster state, so Validate gates every command before execution.
package command

import (
	"fmt"
	"strings"
)

const kubectl = "kubectl"

// allowedVerbs are the read-only kubectl subcommands the agent may run.
var allowedVerbs = map[string]struct{}{
	"get":      {},
	"describe": {},
	"logs":     {},
	"events":   {},
}

// deniedVerbs are mutating or interactive subcommands. They are listed
// explicitly so a violation produces a clear "denied" reason; anything not in
// allowedVerbs is rejected regardless.
var deniedVerbs = map[string]struct{}{
	"apply":        {},
	"patch":        {},
	"delete":       {},
	"exec":         {},
	"port-forward": {},
	"cp":           {},
	"scale":        {},
	"rollout":      {},
	"edit":         {},
	"replace":      {},
	"create":       {},
	"run":          {},
	"attach":       {},
	"drain":        {},
	"cordon":       {},
	"uncordon":     {},
	"annotate":     {},
	"label":        {},
	"set":          {},
	"taint":        {},
	"debug":        {},
}

// deniedFlags would stream output indefinitely and hang the collector.
var deniedFlags = map[string]struct{}{
	"-w":       {},
	"--watch":  {},
	"-f":       {},
	"--follow": {},
}

// Validate reports whether argv is a read-only kubectl command the agent is
// permitted to run. It is pure: it inspects argv and returns an error describing
// the first violation, or nil when the command is allowed.
func Validate(argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("empty command")
	}
	if argv[0] != kubectl {
		return fmt.Errorf("only kubectl commands are permitted, got %q", argv[0])
	}
	if len(argv) < 2 {
		return fmt.Errorf("no kubectl subcommand provided")
	}

	verb := argv[1]
	if strings.HasPrefix(verb, "-") {
		return fmt.Errorf("expected a kubectl subcommand, got flag %q", verb)
	}
	if _, denied := deniedVerbs[verb]; denied {
		return fmt.Errorf("kubectl %q is denied (mutating or interactive)", verb)
	}
	if _, ok := allowedVerbs[verb]; !ok {
		return fmt.Errorf("kubectl %q is not in the read-only allow list", verb)
	}

	for _, arg := range argv[2:] {
		if _, bad := deniedFlags[flagName(arg)]; bad {
			return fmt.Errorf("flag %q is denied (would stream output indefinitely)", flagName(arg))
		}
	}
	return nil
}

// flagName strips an inline value so "--follow=true" matches "--follow".
func flagName(arg string) string {
	if i := strings.IndexByte(arg, '='); i >= 0 {
		return arg[:i]
	}
	return arg
}
