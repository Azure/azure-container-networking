package types

import (
	"fmt"
	"log"
	"reflect"
)

var (
	ErrEmptyDescription    = fmt.Errorf("job description is empty")
	ErrNonNilError         = fmt.Errorf("expected error to be non-nil")
	ErrNilError            = fmt.Errorf("expected error to be nil")
	ErrMissingParameter    = fmt.Errorf("missing parameter")
	ErrParameterAlreadySet = fmt.Errorf("parameter already set")
	ErrOrphanSteps         = fmt.Errorf("background steps with no corresponding stop")
	ErrCannotStopStep      = fmt.Errorf("cannot stop step")
	ErrMissingBackroundID  = fmt.Errorf("missing background id")
)

// A Job is a logical grouping of steps, options and values
type Job struct {
	Values          *JobValues
	Description     string
	Steps           []*StepWrapper
	BackgroundSteps map[string]*StepWrapper
}

// A StepWrapper is a coupling of a step and it's options
type StepWrapper struct {
	Step Step
	Opts *StepOptions
}

// A Scenario is a logical grouping of steps, used to describe a scenario such as "test drop metrics"
// which will require port forwarding, exec'ing, scraping, etc.
type Scenario struct {
	Steps []*StepWrapper
}

func responseDivider(jobname string) {
	totalWidth := 100
	start := 20
	i := 0
	for ; i < start; i++ {
		fmt.Print("#")
	}
	mid := fmt.Sprintf(" %s ", jobname)
	fmt.Print(mid)
	for ; i < totalWidth-(start+len(mid)); i++ {
		fmt.Print("#")
	}
	fmt.Println()
}

func NewJob(description string) *Job {
	return &Job{
		Values: &JobValues{
			kv: make(map[string]string),
		},
		BackgroundSteps: make(map[string]*StepWrapper),
		Description:     description,
	}
}

func (j *Job) AddScenario(scenario *Scenario) {
	for _, step := range scenario.Steps {
		j.AddStep(step.Step, step.Opts)
	}
}

func (j *Job) AddStep(step Step, opts *StepOptions) {
	stepw := &StepWrapper{
		Step: step,
		Opts: opts,
	}
	j.Steps = append(j.Steps, stepw)
}

func (j *Job) Run() error {
	if j.Description == "" {
		return ErrEmptyDescription
	}

	// validate all steps in the job, making sure parameters are set/validated etc.
	err := j.Validate()
	if err != nil {
		return err // nolint:wrapcheck // don't wrap error, wouldn't provide any more context than the error itself
	}

	for _, wrapper := range j.Steps {
		err := wrapper.Step.Prevalidate()
		if err != nil {
			return err //nolint:wrapcheck // don't wrap error, wouldn't provide any more context than the error itself
		}
	}

	for _, wrapper := range j.Steps {
		responseDivider(reflect.TypeOf(wrapper.Step).Elem().Name())
		err := wrapper.Step.Run()
		if wrapper.Opts.ExpectError && err == nil {
			return fmt.Errorf("expected error from step %s but got nil: %w", reflect.TypeOf(wrapper.Step).Elem().Name(), ErrNilError)
		} else if !wrapper.Opts.ExpectError && err != nil {
			return fmt.Errorf("did not expect error from step %s but got error: %w", reflect.TypeOf(wrapper.Step).Elem().Name(), err)
		}
	}

	return nil
}

func (j *Job) Validate() error {
	// ensure that there are no background steps left after running

	for _, wrapper := range j.Steps {
		err := j.validateStep(wrapper)
		if err != nil {
			return err
		}

	}

	err := j.validateBackgroundSteps()
	if err != nil {
		return err
	}

	return nil
}

func (j *Job) validateBackgroundSteps() error {
	stoppedBackgroundSteps := make(map[string]bool)

	for _, stepw := range j.Steps {
		switch s := stepw.Step.(type) {
		case *Stop:
			if s.BackgroundID == "" {
				return fmt.Errorf("cannot stop step with empty background id; %w", ErrMissingBackroundID)
			}

			if j.BackgroundSteps[s.BackgroundID] == nil {
				return fmt.Errorf("cannot stop step %s, as it won't be started by this time; %w", s.BackgroundID, ErrCannotStopStep)
			}
			if stopped := stoppedBackgroundSteps[s.BackgroundID]; stopped {
				return fmt.Errorf("cannot stop step %s, as it has already been stopped; %w", s.BackgroundID, ErrCannotStopStep)
			}

			// track for later on if the stop step is called
			stoppedBackgroundSteps[s.BackgroundID] = true

			// set the stop step within the step
			s.Step = j.BackgroundSteps[s.BackgroundID].Step

		default:
			if stepw.Opts.RunInBackgroundWithID != "" {
				if _, exists := j.BackgroundSteps[stepw.Opts.RunInBackgroundWithID]; exists {
					log.Fatalf("step with id %s already exists", stepw.Opts.RunInBackgroundWithID)
				}
				j.BackgroundSteps[stepw.Opts.RunInBackgroundWithID] = stepw
				stoppedBackgroundSteps[stepw.Opts.RunInBackgroundWithID] = false
			}
		}
	}

	for stepName, stopped := range stoppedBackgroundSteps {
		if !stopped {
			return fmt.Errorf("step %s was not stopped; %w", stepName, ErrOrphanSteps)
		}
	}

	return nil
}

func (j *Job) validateStep(stepw *StepWrapper) error {
	stepName := reflect.TypeOf(stepw.Step).Elem().Name()
	val := reflect.ValueOf(stepw.Step).Elem()

	// set default options if none are provided
	if stepw.Opts == nil {
		stepw.Opts = &DefaultOpts
	}

	switch stepw.Step.(type) {
	case *Stop:
		// don't validate stop steps
		return nil

	case *Sleep:
		// don't validate sleep steps
		return nil

	default:
		for i, f := range reflect.VisibleFields(val.Type()) {

			// skip saving unexported fields
			if !f.IsExported() {
				continue
			}

			k := reflect.Indirect(val.Field(i)).Kind()

			if k == reflect.String {
				parameter := val.Type().Field(i).Name
				value := val.Field(i).Interface().(string)
				storedValue := j.Values.Get(parameter)

				if storedValue == "" {

					switch {
					case stepw.Opts.SkipSavingParamatersToJob:
						continue
					case value != "":
						fmt.Printf("\"%s\" setting parameter \"%s\" in job context to \"%s\"\n", stepName, parameter, value)
						j.Values.Set(parameter, value)
					default:
						return fmt.Errorf("missing parameter \"%s\" for step \"%s\": %w", parameter, stepName, ErrMissingParameter)
					}
					continue
				}

				if value != "" {
					return fmt.Errorf("parameter %s for step %s is already set from previous step: %w", parameter, stepName, ErrParameterAlreadySet)
				}

				// don't use log format since this is technically preexecution and easier to read
				fmt.Println(stepName, "using previously stored value for parameter", parameter, "set as", j.Values.Get(parameter))
				val.Field(i).SetString(storedValue)
			}
		}
	}
	return nil
}
