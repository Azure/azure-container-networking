package types

import (
	"errors"
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
	values          *JobValues
	Description     string
	Steps           []*StepWrapper
	BackgroundSteps map[string]*StepWrapper
	Scenarios       map[*StepWrapper]*Scenario
}

// A StepWrapper is a coupling of a step and it's options
type StepWrapper struct {
	Step Step
	Opts *StepOptions
}

// A Scenario is a logical grouping of steps, used to describe a scenario such as "test drop metrics"
// which will require port forwarding, exec'ing, scraping, etc.
type Scenario struct {
	Description string
	Steps       []*StepWrapper
	values      *JobValues
}

func NewScenario(description string, steps ...*StepWrapper) *Scenario {
	return &Scenario{
		Description: description,
		Steps:       steps,
		values:      &JobValues{kv: make(map[string]string)},
	}
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
		values: &JobValues{
			kv: make(map[string]string),
		},
		BackgroundSteps: make(map[string]*StepWrapper),
		Scenarios:       make(map[*StepWrapper]*Scenario),
		Description:     description,
	}
}

func (j *Job) AddScenario(scenario *Scenario) {
	for i, step := range scenario.Steps {
		j.Steps = append(j.Steps, step)
		j.Scenarios[scenario.Steps[i]] = scenario
	}
}

func (j *Job) AddStep(step Step, opts *StepOptions) {
	stepw := &StepWrapper{
		Step: step,
		Opts: opts,
	}
	j.Steps = append(j.Steps, stepw)
}

func (j *Job) GetValue(stepw *StepWrapper, key string) (string, bool) {

	// if step exists in a scenario, use the scenario's values
	// if the value isn't in the scenario's values, get the root job's value
	if scenario, exists := j.Scenarios[stepw]; exists {
		if scenario.values.Contains(key) {
			return scenario.values.Get(key), true
		}
	}
	if j.values.Contains(key) {
		return j.values.Get(key), true
	}

	return "", false
}

func (j *Job) SetStepValues(stepw *StepWrapper, key, value string) (string, error) {
	// if top level step parameter is set, and scenario step is not, inherit
	// if top level step parameter is not set, and scenario step is, use scenario step
	// if top level step parameter is set, and scenario step is set, warn and use scenario step

	// check if scenario exists, if it does, check if the value is in the scenario's values
	if scenario, exists := j.Scenarios[stepw]; exists {
		scenarioValue, err := scenario.values.SetGet(key, value)
		if err != nil && !errors.Is(err, ErrEmptyValue) {
			return "", err
		}
		if scenarioValue != "" {
			return scenarioValue, nil
		}
		fmt.Printf("parameter %s not found in scenario values, using top level value\n", key)
	}

	return j.values.SetGet(key, value)
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

				// if top level step parameter is set, and scenario step is not, inherit
				// if top level step parameter is not set, and scenario step is, use scenario step
				// if top level step parameter is set, and scenario step is set, warn and use scenario step

				storedvalue, err := j.SetStepValues(stepw, parameter, value)
				if err != nil {
					return fmt.Errorf("error setting parameter %s: %w", parameter, err)
				}

				// don't use log format since this is technically preexecution and easier to read
				fmt.Println(stepName, "setting stored value for parameter", parameter, "set as", storedvalue)
				val.Field(i).SetString(storedvalue)
			}
		}
	}
	return nil
}
