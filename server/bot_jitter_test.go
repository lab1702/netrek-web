package server

import (
	"math"
	"testing"
)

func TestRandomJitterRad(t *testing.T) {
	// Test that jitter is within expected range
	const numTests = 1000
	const maxExpectedRad = maxJitterDeg * math.Pi / 180 // Convert max degrees to radians

	for i := 0; i < numTests; i++ {
		jitter := randomJitterRad()

		// Check that jitter is within the expected range
		if math.Abs(jitter) > maxExpectedRad {
			t.Errorf("Jitter %f radians (%f degrees) exceeds maximum of %f radians (%f degrees)",
				jitter, jitter*180/math.Pi, maxExpectedRad, maxJitterDeg)
		}
	}

	// Test that we get different values (not all zeros)
	var values []float64
	for i := 0; i < 10; i++ {
		values = append(values, randomJitterRad())
	}

	// Check that we have at least some non-zero values
	hasNonZero := false
	for _, val := range values {
		if val != 0 {
			hasNonZero = true
			break
		}
	}

	if !hasNonZero {
		t.Error("All jitter values were zero - randomization may not be working")
	}

	t.Logf("Jitter range test passed with %d samples, max allowed: ±%.1f°", numTests, maxJitterDeg)
}
