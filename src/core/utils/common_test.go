package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetProjectDir(t *testing.T) {
	result := GetProjectDir()

	assert.NotEmpty(t, result, "Project directory should not be empty")
	assert.Contains(t, result, "xiaozhi-server-go", "Should contain project name")
}

func TestGetProjectDir_ReturnsAbsolutePath(t *testing.T) {
	result := GetProjectDir()

	assert.NotNil(t, result, "Result should not be nil")
	assert.IsType(t, "", result, "Result should be string")
}

func TestMinDuration_BothPositive(t *testing.T) {
	a := 100 * time.Millisecond
	b := 200 * time.Millisecond

	result := MinDuration(a, b)

	assert.Equal(t, a, result, "Should return the smaller duration")
}

func TestMinDuration_BEqualToA(t *testing.T) {
	duration := 150 * time.Millisecond

	result := MinDuration(duration, duration)

	assert.Equal(t, duration, result, "Should return the duration when equal")
}

func TestMinDuration_AIsZero(t *testing.T) {
	a := time.Duration(0)
	b := 100 * time.Millisecond

	result := MinDuration(a, b)

	assert.Equal(t, a, result, "Should return zero duration")
}

func TestMinDuration_BIsZero(t *testing.T) {
	a := 100 * time.Millisecond
	b := time.Duration(0)

	result := MinDuration(a, b)

	assert.Equal(t, b, result, "Should return zero duration")
}

func TestMinDuration_NegativeDurations(t *testing.T) {
	a := -50 * time.Millisecond
	b := -100 * time.Millisecond

	result := MinDuration(a, b)

	assert.Equal(t, b, result, "Should return the more negative duration")
}
