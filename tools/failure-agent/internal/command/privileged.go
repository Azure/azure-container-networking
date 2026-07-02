package command

import "fmt"

// privilegedVerbs are kubectl subcommands permitted only when the user
// explicitly opts in with --privileged. They allow node-level log collection
// via ephemeral debug pods.
var privilegedVerbs = map[string]struct{}{
	"debug": {},
	"exec":  {},
}

// ValidatePrivileged is like Validate but additionally permits debug and exec
// verbs. It is used only when the operator explicitly requests privileged log
// collection (--privileged flag). Commands still may not use streaming flags.
func ValidatePrivileged(argv []string) error {
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
	if _, ok := allowedVerbs[verb]; ok {
		return validateFlags(argv[2:])
	}
	if _, ok := privilegedVerbs[verb]; ok {
		return validateFlags(argv[2:])
	}
	return fmt.Errorf("kubectl %q is not permitted in privileged mode", verb)
}

func validateFlags(args []string) error {
	for _, arg := range args {
		if _, bad := deniedFlags[flagName(arg)]; bad {
			return fmt.Errorf("flag %q is denied (would stream output indefinitely)", flagName(arg))
		}
	}
	return nil
}
