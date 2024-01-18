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
)

type Job struct {
	Values      *JobValues
	Description string
	Steps       []*StepWrapper
}

type StepWrapper struct {
	Step Step
	Opts *StepOptions
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
		Description: description,
	}
}

func (j *Job) AddScenario(steps ...StepWrapper) {
	for _, step := range steps {
		j.AddStep(step.Step, step.Opts)
	}
}

func (j *Job) AddStep(step Step, opts *StepOptions) {
	j.Steps = append(j.Steps, &StepWrapper{
		Step: step,
		Opts: opts,
	})
}

func (j *Job) Run() error {
	if j.Description == "" {
		return ErrEmptyDescription
	}

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
		log.Printf("INFO: step options provided: %+v\n", wrapper.Opts)
		err := wrapper.Step.Run()
		if wrapper.Opts.ExpectError && err == nil {
			return fmt.Errorf("expected error from step %s but got nil: %w", reflect.TypeOf(wrapper.Step).Elem().Name(), ErrNilError)
		} else if !wrapper.Opts.ExpectError && err != nil {
			return fmt.Errorf("did not expect error from step %s but got error: %w", reflect.TypeOf(wrapper.Step).Elem().Name(), err)
		}
	}

	for _, wrapper := range j.Steps {
		err := wrapper.Step.Postvalidate()
		if err != nil {
			return err //nolint:wrapcheck // don't wrap error, wouldn't provide any more context than the error itself
		}
	}
	return nil
}

func (j *Job) Validate() error {
	for _, wrapper := range j.Steps {
		err := j.validateStep(wrapper)
		if err != nil {
			return err
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
				if value != "" {
					if stepw.Opts.SaveParametersToJob {
						fmt.Printf("%s setting parameter %s in job context to %s\n", stepName, parameter, value)
						j.Values.Set(parameter, value)
					}
					continue
				}
				return fmt.Errorf("missing parameter %s for step %s: %w", parameter, stepName, ErrMissingParameter)

			}

			if value != "" {
				return fmt.Errorf("parameter %s for step %s is already set from previous step: %w", parameter, stepName, ErrParameterAlreadySet)
			}

			// don't use log format since this is technically preexecution and easier to read
			fmt.Println(stepName, "using previously stored value for parameter", parameter, "set as", j.Values.Get(parameter))
			val.Field(i).SetString(storedValue)
		}
	}

	return nil
}
