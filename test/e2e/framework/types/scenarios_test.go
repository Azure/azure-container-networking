package types

import (
	"fmt"
	"testing"
)

// Test against a BYO cluster with Cilium and Hubble enabled,
// create a pod with a deny all network policy and validate
// that the drop metrics are present in the prometheus endpoint
func TestScenarioValues(t *testing.T) {
	job := NewJob("Validate that drop metrics are present in the prometheus endpoint")
	runner := NewRunner(t, job)
	defer runner.Run()

	job.AddStep(&DummyStep{
		Parameter1: "Top Level Step 1",
		Parameter2: "Top Level Step 2",
	}, nil)

	job.AddScenario(NewDummyScenario())

	job.AddStep(&DummyStep{}, nil)
}

func NewDummyScenario() *Scenario {
	return NewScenario("Dummy Scenario",
		&StepWrapper{
			Step: &DummyStep{
				Parameter2: "Scenario Parameter 2",
			},
		},
	)
}

type DummyStep struct {
	Parameter1 string
	Parameter2 string
}

func (d *DummyStep) Run() error {
	fmt.Printf("Running DummyStep with parameter 1 as: %s\n", d.Parameter1)
	fmt.Printf("Running DummyStep with parameter 2 as: %s\n", d.Parameter2)
	return nil
}
func (d *DummyStep) Stop() error {
	return nil
}
func (d *DummyStep) Prevalidate() error {
	return nil
}
