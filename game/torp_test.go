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
			name:     "Galaxy",
			shipType: ShipGalaxy,
			expected: 13 * 20 * 35, // TorpSpeed=13, TorpUnitFactor=20, TorpFuse=35 = 9100
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
			expected:     int(5120 * 0.85), // 4352
		},
		{
			name:         "Cruiser with default safety",
			shipType:     ShipCruiser,
			safetyMargin: DefaultTorpSafety,
			expected:     int(9600 * 0.85), // 8160
		},
		{
			name:         "Starbase with default safety",
			shipType:     ShipStarbase,
			safetyMargin: DefaultTorpSafety,
			expected:     int(8400 * 0.85), // 7140
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
			expected: int(5120 * DefaultTorpSafety), // 4352
		},
		{
			name:     "Cruiser",
			shipType: ShipCruiser,
			expected: int(9600 * DefaultTorpSafety), // 8160
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
