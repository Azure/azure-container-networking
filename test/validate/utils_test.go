package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompareIPsDetailed_Match(t *testing.T) {
	expected := map[string]string{
		"10.0.0.1": "pod-a",
		"10.0.0.2": "pod-b",
	}
	actual := []string{"10.0.0.1", "10.0.0.2"}

	result := compareIPsDetailed(expected, actual)
	assert.Equal(t, 2, result.ExpectedCount)
	assert.Equal(t, 2, result.ActualCount)
	assert.Empty(t, result.MissingIPs)
	assert.Empty(t, result.UnexpectedIPs)
	assert.Empty(t, result.DuplicateIPs)
	assert.False(t, result.HasMismatch())
}

func TestCompareIPsDetailed_MissingUnexpectedDuplicate(t *testing.T) {
	expected := map[string]string{
		"10.0.0.1": "pod-a",
		"10.0.0.2": "pod-b",
	}
	actual := []string{"10.0.0.1", "10.0.0.9", "10.0.0.9"}

	result := compareIPsDetailed(expected, actual)
	assert.Equal(t, 2, result.ExpectedCount)
	assert.Equal(t, 3, result.ActualCount)
	assert.Equal(t, []string{"10.0.0.2"}, result.MissingIPs)
	assert.Equal(t, []string{"10.0.0.9"}, result.UnexpectedIPs)
	assert.Equal(t, []string{"10.0.0.9"}, result.DuplicateIPs)
	assert.True(t, result.HasMismatch())
}
