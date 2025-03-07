package time

import (
	"encoding/json"
	"time"
)

const Second = time.Second //nolint:revive // it's not a suffix

type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String()) //nolint:wrapcheck // ignore
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err //nolint:wrapcheck // ignore
	}
	duration, err := time.ParseDuration(s)
	if err != nil {
		return err //nolint:wrapcheck // ignore
	}
	d.Duration = duration
	return nil
}
