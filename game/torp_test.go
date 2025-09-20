package game

import (
	"testing"
)

func TestMaxTorpRange(t *testing.T) {
	tests := []struct {
		name     string
		shipType ShipType
		expected int
	}{
		{
			name:     "Scout",
			shipType: ShipScout,
			expected: 16 * 20 * 16, // TorpSpeed=16, TorpUnitFactor=20, TorpFuse=16 = 5120
		},
		{
			name:     "Destroyer",
			shipType: ShipDestroyer,
			expected: 14 * 20 * 30, // TorpSpeed=14, TorpUnitFactor=20, TorpFuse=30 = 8400
		},
		{
			name:     "Cruiser",
			shipType: ShipCruiser,
			expected: 12 * 20 * 40, // TorpSpeed=12, TorpUnitFactor=20, TorpFuse=40 = 9600
		},
		{
			name:     "Battleship",
			shipType: ShipBattleship,
			expected: 12 * 20 * 40, // TorpSpeed=12, TorpUnitFactor=20, TorpFuse=40 = 9600
		},
		{
			name:     "Assault",
			shipType: ShipAssault,
			expected: 16 * 20 * 30, // TorpSpeed=16, TorpUnitFactor=20, TorpFuse=30 = 9600
		},
		{
			name:     "Starbase",
			shipType: ShipStarbase,
			expected: 14 * 20 * 30, // TorpSpeed=14, TorpUnitFactor=20, TorpFuse=30 = 8400
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shipStats := ShipData[tt.shipType]
			result := MaxTorpRange(shipStats)
			if result != tt.expected {
				t.Errorf("MaxTorpRange(%s) = %d, expected %d", tt.name, result, tt.expected)
			}
		})
	}
}

func TestEffectiveTorpRange(t *testing.T) {
	tests := []struct {
		name         string
		shipType     ShipType
		safetyMargin float64
		expected     int
	}{
		{
			name:         "Scout with default safety",
			shipType:     ShipScout,
			safetyMargin: DefaultTorpSafety,
			expected:     int(5120 * DefaultTorpSafety), // Now 0.70: 3584
		},
		{
			name:         "Cruiser with default safety",
			shipType:     ShipCruiser,
			safetyMargin: DefaultTorpSafety,
			expected:     int(9600 * DefaultTorpSafety), // Now 0.70: 6720
		},
		{
			name:         "Starbase with default safety",
			shipType:     ShipStarbase,
			safetyMargin: DefaultTorpSafety,
			expected:     int(8400 * DefaultTorpSafety), // Now 0.70: 5880
		},
		{
			name:         "Scout with custom safety",
			shipType:     ShipScout,
			safetyMargin: 0.75,
			expected:     int(5120 * 0.75), // 3840
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shipStats := ShipData[tt.shipType]
			result := EffectiveTorpRange(shipStats, tt.safetyMargin)
			if result != tt.expected {
				t.Errorf("EffectiveTorpRange(%s, %.2f) = %d, expected %d", tt.name, tt.safetyMargin, result, tt.expected)
			}
		})
	}
}

func TestEffectiveTorpRangeDefault(t *testing.T) {
	tests := []struct {
		name     string
		shipType ShipType
		expected int
	}{
		{
			name:     "Scout",
			shipType: ShipScout,
			expected: int(5120 * DefaultTorpSafety), // With new 0.70 default: 3584
		},
		{
			name:     "Cruiser",
			shipType: ShipCruiser,
			expected: int(9600 * DefaultTorpSafety), // With new 0.70 default: 6720
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shipStats := ShipData[tt.shipType]
			result := EffectiveTorpRangeDefault(shipStats)
			if result != tt.expected {
				t.Errorf("EffectiveTorpRangeDefault(%s) = %d, expected %d", tt.name, result, tt.expected)
			}
		})
	}
}

// TestEffectiveTorpRangeForShip verifies that ship-specific torpedo ranges
// are more conservative than the old default and prevent fuse expiry
func TestEffectiveTorpRangeForShip(t *testing.T) {
	testCases := []struct {
		ship     ShipType
		expected float64 // Expected safety factor
	}{
		{ShipScout, 0.65},
		{ShipDestroyer, 0.70},
		{ShipCruiser, 0.75},
		{ShipBattleship, 0.75},
		{ShipAssault, 0.65},
		{ShipStarbase, 0.80},
	}

	for _, tc := range testCases {
		t.Run(ShipData[tc.ship].Name, func(t *testing.T) {
			stats := ShipData[tc.ship]

			// Calculate ranges
			maxRange := MaxTorpRange(stats)
			oldEffectiveRange := EffectiveTorpRangeDefault(stats) // Uses 0.70 (reduced from 0.85)
			newEffectiveRange := EffectiveTorpRangeForShip(tc.ship, stats)

			// Expected range based on ship-specific safety factor
			expectedRange := int(float64(maxRange) * tc.expected)

			t.Logf("Ship: %s, Max: %d, Old: %d, New: %d, Expected: %d",
				stats.Name, maxRange, oldEffectiveRange, newEffectiveRange, expectedRange)

			// Verify new range matches expected ship-specific factor
			if newEffectiveRange != expectedRange {
				t.Errorf("Expected range %d, got %d", expectedRange, newEffectiveRange)
			}

			// Verify new range is more conservative than old 0.85 default
			old85Range := int(float64(maxRange) * 0.85) // What it used to be
			if newEffectiveRange >= old85Range {
				t.Errorf("New range %d should be less than old 0.85 range %d",
					newEffectiveRange, old85Range)
			}

			// Verify range is reasonable (not zero or negative)
			if newEffectiveRange <= 0 {
				t.Errorf("Range should be positive, got %d", newEffectiveRange)
			}

			// Verify range doesn't exceed maximum possible
			if newEffectiveRange > maxRange {
				t.Errorf("Effective range %d cannot exceed max range %d",
					newEffectiveRange, maxRange)
			}
		})
	}
}

// TestTorpedoSurvivalMargin tests that torpedoes have sufficient time to reach targets
// at the effective range before their fuse expires
func TestTorpedoSurvivalMargin(t *testing.T) {
	for shipType := ShipScout; shipType <= ShipStarbase; shipType++ {
		t.Run(ShipData[shipType].Name, func(t *testing.T) {
			stats := ShipData[shipType]
			effectiveRange := EffectiveTorpRangeForShip(shipType, stats)

			// Calculate travel time at effective range
			torpSpeed := float64(stats.TorpSpeed * TorpUnitFactor) // units/tick
			travelTime := float64(effectiveRange) / torpSpeed      // ticks
			fuseTime := float64(stats.TorpFuse)                    // ticks

			// Safety margin: torpedo should have at least 2 ticks remaining
			minSafetyTicks := 2.0
			remainingTime := fuseTime - travelTime

			t.Logf("Ship: %s, Range: %d, Travel: %.1f ticks, Fuse: %.0f ticks, Remaining: %.1f ticks",
				stats.Name, effectiveRange, travelTime, fuseTime, remainingTime)

			if remainingTime < minSafetyTicks {
				t.Errorf("Insufficient safety margin: %.1f ticks remaining, need at least %.1f",
					remainingTime, minSafetyTicks)
			}
		})
	}
}

// TestRangeComparison compares old vs new ranges to show the improvement
func TestRangeComparison(t *testing.T) {
	t.Log("=== Torpedo Range Comparison (Old 0.85 vs New Ship-Specific) ===")

	for shipType := ShipScout; shipType <= ShipStarbase; shipType++ {
		stats := ShipData[shipType]
		maxRange := MaxTorpRange(stats)
		old85Range := int(float64(maxRange) * 0.85)
		newRange := EffectiveTorpRangeForShip(shipType, stats)
		reduction := old85Range - newRange
		reductionPct := float64(reduction) / float64(old85Range) * 100

		t.Logf("%-12s: Max=%5d, Old=%5d, New=%5d, Reduction=%4d (%.1f%%)",
			stats.Name, maxRange, old85Range, newRange, reduction, reductionPct)
	}
}
