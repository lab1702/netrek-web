package server

import (
	"math"
	"testing"
)

const (
	// Test tolerance for floating point comparisons
	angleTolerance = 0.017 // ~1 degree in radians
	timeTolerance  = 0.1   // 0.1 tick tolerance
	distTolerance  = 10.0  // 10 units tolerance
)

// TestCase represents a single intercept test case
type TestCase struct {
	name          string
	shooterPos    Point2D
	targetPos     Point2D
	targetVel     Vector2D
	projSpeed     float64
	expectedDir   float64 // Expected direction in radians
	expectedTime  float64 // Expected time to intercept
	shouldSucceed bool    // Whether intercept should be possible
	description   string  // Description of what this tests
}

// TestInterceptDirection_Comprehensive runs all test cases
func TestInterceptDirection_Comprehensive(t *testing.T) {
	testCases := []TestCase{
		{
			name:          "StationaryTarget",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 0, Y: 0},
			projSpeed:     50.0,
			expectedDir:   0.0, // Firing east
			expectedTime:  2.0, // 100 units / 50 speed = 2 ticks
			shouldSucceed: true,
			description:   "Stationary target directly east",
		},
		{
			name:          "HeadOnApproach",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: -25, Y: 0}, // Moving toward shooter
			projSpeed:     50.0,
			expectedDir:   0.0,                   // Fire directly at target
			expectedTime:  100.0 / (50.0 + 25.0), // Relative closing speed
			shouldSucceed: true,
			description:   "Target moving directly toward shooter",
		},
		{
			name:          "PerpendicularCrossing",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 0, Y: 30}, // Moving north
			projSpeed:     50.0,
			expectedDir:   math.Atan(30.0 / 40.0), // Lead angle
			expectedTime:  2.5,                    // Calculated intercept time
			shouldSucceed: true,
			description:   "Target crossing perpendicular to line of sight",
		},
		{
			name:          "ChasingFastTarget",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 40, Y: 0}, // Moving away fast
			projSpeed:     50.0,
			expectedDir:   0.0,  // Chase shot
			expectedTime:  10.0, // 100/(50-40) = 10 ticks
			shouldSucceed: true,
			description:   "Chasing fast target moving away",
		},
		{
			name:          "ImpossibleIntercept_TooFast",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 60, Y: 0}, // Faster than projectile
			projSpeed:     50.0,
			shouldSucceed: false,
			description:   "Target moving away faster than projectile",
		},
		{
			name:          "ImpossibleIntercept_Perpendicular",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 0, Y: 60}, // Too fast perpendicular
			projSpeed:     50.0,
			shouldSucceed: false,
			description:   "Target moving too fast perpendicular",
		},
		{
			name:          "DiagonalMotion",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 100},
			targetVel:     Vector2D{X: 20, Y: -20}, // Moving SE
			projSpeed:     60.0,
			shouldSucceed: true,
			description:   "Target with diagonal motion",
		},
		{
			name:          "ZeroDistance",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 0, Y: 0}, // Same position
			targetVel:     Vector2D{X: 10, Y: 10},
			projSpeed:     50.0,
			expectedDir:   0.0,
			expectedTime:  -1.0, // Use -1 to indicate we don't check time for this case
			shouldSucceed: true,
			description:   "Target at same position as shooter",
		},
		{
			name:          "CircularMotion_Tangent",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: 0, Y: 31.4}, // Tangential velocity (π*10 ≈ 31.4)
			projSpeed:     50.0,
			shouldSucceed: true,
			description:   "Target in circular motion (tangent velocity)",
		},
		{
			name:          "HighSpeedIntercept",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 1000, Y: 0},
			targetVel:     Vector2D{X: 0, Y: 100}, // Fast crossing
			projSpeed:     141.42,                 // sqrt(2) * 100, should just work
			shouldSucceed: true,
			description:   "High speed intercept at edge of capability",
		},
		{
			name:          "LinearCase_SameSpeed",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 100, Y: 0},
			targetVel:     Vector2D{X: -50, Y: 0}, // Same speed as projectile, toward shooter
			projSpeed:     50.0,
			expectedDir:   0.0, // Direct shot
			expectedTime:  1.0, // 100/(50+50) in linear approximation
			shouldSucceed: true,
			description:   "Linear case where speeds are equal",
		},
		{
			name:          "PrecisionEdgeCase",
			shooterPos:    Point2D{X: 0, Y: 0},
			targetPos:     Point2D{X: 1, Y: 1}, // Very close
			targetVel:     Vector2D{X: 0.1, Y: 0.1},
			projSpeed:     10.0,
			shouldSucceed: true,
			description:   "Precision test with small numbers",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			solution, ok := InterceptDirection(tc.shooterPos, tc.targetPos, tc.targetVel, tc.projSpeed)

			// Check if result matches expectation
			if ok != tc.shouldSucceed {
				t.Errorf("Expected success=%v, got success=%v", tc.shouldSucceed, ok)
				return
			}

			if !tc.shouldSucceed {
				// For impossible cases, just verify it correctly returned false
				return
			}

			// For successful cases, verify the solution
			if solution == nil {
				t.Error("Got nil solution but expected success")
				return
			}

			// Verify time is positive
			if solution.TimeToIntercept <= 0 {
				t.Errorf("Expected positive time, got %f", solution.TimeToIntercept)
			}

			// For cases with expected values, check them
			if tc.expectedDir != 0 || tc.name == "StationaryTarget" || tc.name == "HeadOnApproach" {
				angleDiff := AngleDifference(solution.Direction, tc.expectedDir)
				if angleDiff > angleTolerance {
					t.Errorf("Direction error too large: expected %f rad (%.1f°), got %f rad (%.1f°), diff %.1f°",
						tc.expectedDir, tc.expectedDir*180/math.Pi,
						solution.Direction, solution.Direction*180/math.Pi,
						angleDiff*180/math.Pi)
				}
			}

			if tc.expectedTime > 0 {
				timeDiff := math.Abs(solution.TimeToIntercept - tc.expectedTime)
				if timeDiff > timeTolerance {
					t.Errorf("Time error too large: expected %f, got %f, diff %f",
						tc.expectedTime, solution.TimeToIntercept, timeDiff)
				}
			}

			// Verify the solution by simulation
			verifyInterceptSolution(t, tc, solution)
		})
	}
}

// verifyInterceptSolution verifies that the calculated solution actually works
func verifyInterceptSolution(t *testing.T, tc TestCase, solution *InterceptSolution) {
	// Simulate projectile and target motion
	time := solution.TimeToIntercept

	// Final projectile position
	projX := tc.shooterPos.X + tc.projSpeed*math.Cos(solution.Direction)*time
	projY := tc.shooterPos.Y + tc.projSpeed*math.Sin(solution.Direction)*time

	// Final target position
	targX := tc.targetPos.X + tc.targetVel.X*time
	targY := tc.targetPos.Y + tc.targetVel.Y*time

	// Check if they meet within tolerance
	distance := math.Sqrt((projX-targX)*(projX-targX) + (projY-targY)*(projY-targY))
	if distance > distTolerance {
		t.Errorf("Projectile and target don't meet: distance=%f, tolerance=%f", distance, distTolerance)
		t.Errorf("  Projectile final pos: (%.2f, %.2f)", projX, projY)
		t.Errorf("  Target final pos: (%.2f, %.2f)", targX, targY)
		t.Errorf("  Time: %.2f, Direction: %.3f rad (%.1f°)",
			time, solution.Direction, solution.Direction*180/math.Pi)
	}
}

// TestInterceptDirectionSimple tests the simplified interface
func TestInterceptDirectionSimple(t *testing.T) {
	// Test successful case
	dir, ok := InterceptDirectionSimple(
		Point2D{X: 0, Y: 0},
		Point2D{X: 100, Y: 0},
		Vector2D{X: 0, Y: 0},
		50.0,
	)
	if !ok {
		t.Error("Expected success for stationary target")
	}
	if math.Abs(dir) > angleTolerance {
		t.Errorf("Expected direction ~0, got %f", dir)
	}

	// Test impossible case - should return direct shot
	dir, ok = InterceptDirectionSimple(
		Point2D{X: 0, Y: 0},
		Point2D{X: 100, Y: 0},
		Vector2D{X: 60, Y: 0}, // Too fast
		50.0,
	)
	if ok {
		t.Error("Should have failed for impossible intercept")
	}
	// Should still return a direction (direct shot)
	expectedDirect := math.Atan2(0, 100) // 0 radians
	if math.Abs(dir-expectedDirect) > angleTolerance {
		t.Errorf("Expected fallback direction %f, got %f", expectedDirect, dir)
	}
}

// TestAngleUtilities tests the angle utility functions
func TestAngleUtilities(t *testing.T) {
	// Test NormalizeAngle
	testAngles := []struct {
		input, expected float64
	}{
		{0, 0},
		{math.Pi, math.Pi},
		{-math.Pi, math.Pi}, // -π should become π
		{2 * math.Pi, 0},
		{-2 * math.Pi, 0},
		{3 * math.Pi, math.Pi},
		{-3 * math.Pi, math.Pi},
	}

	for _, test := range testAngles {
		result := NormalizeAngle(test.input)
		if math.Abs(result-test.expected) > 1e-10 {
			t.Errorf("NormalizeAngle(%f): expected %f, got %f", test.input, test.expected, result)
		}
	}

	// Test AngleDifference
	diffTests := []struct {
		a1, a2, expected float64
	}{
		{0, 0, 0},
		{0, math.Pi, math.Pi},
		{0, -math.Pi, math.Pi},
		{math.Pi / 4, -math.Pi / 4, math.Pi / 2},
		{math.Pi * 0.9, -math.Pi * 0.9, math.Pi * 0.2}, // Should wrap around
	}

	for _, test := range diffTests {
		result := AngleDifference(test.a1, test.a2)
		if math.Abs(result-test.expected) > angleTolerance {
			t.Errorf("AngleDifference(%f, %f): expected %f, got %f",
				test.a1, test.a2, test.expected, result)
		}
	}
}

// BenchmarkInterceptDirection benchmarks the intercept calculation
func BenchmarkInterceptDirection(b *testing.B) {
	shooterPos := Point2D{X: 0, Y: 0}
	targetPos := Point2D{X: 1000, Y: 500}
	targetVel := Vector2D{X: 20, Y: 30}
	projSpeed := 100.0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = InterceptDirection(shooterPos, targetPos, targetVel, projSpeed)
	}
}

// TestEdgeCases tests various edge cases and error conditions
func TestEdgeCases(t *testing.T) {
	// Zero projectile speed
	_, ok := InterceptDirection(
		Point2D{X: 0, Y: 0},
		Point2D{X: 100, Y: 0},
		Vector2D{X: 0, Y: 0},
		0.0, // Zero speed
	)
	if ok {
		t.Error("Should fail with zero projectile speed")
	}

	// Negative projectile speed
	_, ok = InterceptDirection(
		Point2D{X: 0, Y: 0},
		Point2D{X: 100, Y: 0},
		Vector2D{X: 0, Y: 0},
		-10.0, // Negative speed
	)
	if ok {
		t.Error("Should fail with negative projectile speed")
	}

	// Very small distance
	solution, ok := InterceptDirection(
		Point2D{X: 0, Y: 0},
		Point2D{X: 1e-10, Y: 1e-10}, // Essentially zero distance
		Vector2D{X: 1, Y: 1},
		10.0,
	)
	if !ok {
		t.Error("Should succeed for very small distance")
	}
	if solution.TimeToIntercept <= 0 || solution.TimeToIntercept > 1e-5 {
		t.Errorf("Expected very small positive time for zero distance, got %f", solution.TimeToIntercept)
	}

	// Very large numbers
	_, ok = InterceptDirection(
		Point2D{X: 0, Y: 0},
		Point2D{X: 1e6, Y: 1e6},
		Vector2D{X: 1000, Y: 1000},
		5000.0,
	)
	if !ok {
		t.Error("Should handle large numbers")
	}
}