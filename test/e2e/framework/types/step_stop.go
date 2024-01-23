package types

type Stop struct {
	BackgroundID string
	Step         Step
}

func (c *Stop) Run() error {
	c.Step.Stop()
	return nil
}

func (c *Stop) Stop() error {
	return nil
}

func (c *Stop) Prevalidate() error {
	return nil
}

func (c *Stop) Postvalidate() error {
	return nil
}
