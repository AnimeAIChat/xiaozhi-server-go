package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetSystemMemoryUsage(t *testing.T) {
	percent, err := GetSystemMemoryUsage()

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, percent, float64(0), "Memory usage should be non-negative")
	assert.LessOrEqual(t, percent, float64(100), "Memory usage should not exceed 100%")
}

func TestGetSystemMemoryUsage_ValidRange(t *testing.T) {
	percent, err := GetSystemMemoryUsage()

	assert.NoError(t, err)
	assert.True(t, percent >= 0 && percent <= 100, "Memory usage percentage should be between 0 and 100")
}

func TestGetSystemCPUUsage(t *testing.T) {
	percent, err := GetSystemCPUUsage()

	assert.NoError(t, err)
	assert.GreaterOrEqual(t, percent, float64(0), "CPU usage should be non-negative")
	assert.LessOrEqual(t, percent, float64(100), "CPU usage should not exceed 100%")
}

func TestGetSystemCPUUsage_ValidRange(t *testing.T) {
	percent, err := GetSystemCPUUsage()

	assert.NoError(t, err)
	assert.True(t, percent >= 0 && percent <= 100, "CPU usage percentage should be between 0 and 100")
}
