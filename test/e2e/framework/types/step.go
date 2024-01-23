package types

var DefaultOpts = StepOptions{
	ExpectError:               false,
	SkipSavingParamatersToJob: false,
}

type Step interface {
	Prevalidate() error
	Run() error
	Postvalidate() error
	Stop() error
}

type StepOptions struct {
	ExpectError bool

	// Generally set this to false when you want to reuse
	// a step, but you don't want to save the parameters
	// ex: Sleep for 15 seconds, then Sleep for 10 seconds,
	// you don't want to save the parameters
	SkipSavingParamatersToJob bool

	// Will save this step to the job's steps
	// and then later on when Stop is called with job name,
	// it will call Stop() on the step
	RunInBackgroundWithID string
}
