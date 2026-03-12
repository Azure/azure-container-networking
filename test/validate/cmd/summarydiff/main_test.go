package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAggregate(t *testing.T) {
	summary := validationSummary{
		Checks: []validationCheckEntry{
			{
				ExpectedCount:  3,
				ActualCount:    3,
				ValidationPass: true,
			},
			{
				ExpectedCount:  2,
				ActualCount:    3,
				MissingIPs:     []string{"10.0.0.2"},
				UnexpectedIPs:  []string{"10.0.0.9"},
				DuplicateIPs:   []string{"10.0.0.9"},
				ValidationPass: false,
			},
		},
	}

	stats := aggregate(summary)
	assert.Equal(t, 2, stats.TotalChecks)
	assert.Equal(t, 1, stats.FailedChecks)
	assert.Equal(t, 1, stats.MissingIPs)
	assert.Equal(t, 1, stats.UnexpectedIPs)
	assert.Equal(t, 1, stats.DuplicateIPs)
	assert.Equal(t, 5, stats.ExpectedIPsSum)
	assert.Equal(t, 6, stats.ActualIPsSum)
}
