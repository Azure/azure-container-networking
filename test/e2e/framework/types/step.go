package types

var DefaultOpts = StepOptions{
	ExpectError:         false,
	SaveParametersToJob: true,
}

type Step interface {
	Prevalidate() error
	Run() error
	Postvalidate() error
}

type StepOptions struct {
	ExpectError bool

	// Generally set this to false when you want to reuse
	// a step, but you don't want to save the parameters
	// ex: Sleep for 15 seconds, then Sleep for 10 seconds,
	// you don't want to save the parameters
	SaveParametersToJob bool
}
