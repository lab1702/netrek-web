package game

import (
	"testing"
)

func TestMaxPlasmaRange(t *testing.T) {
	tests := []struct {
		name              string
		fuseTicks         int
		speedUnitsPerTick float64
		expectedRange     float64
	}{
		{
			name:              "Destroyer plasma",
			fuseTicks:         30,
			speedUnitsPerTick: 300.0, // 15 * 20
			expectedRange:     9000.0,
		},
		{
			name:              "Cruiser/Battleship plasma",
			fuseTicks:         35,
			speedUnitsPerTick: 300.0, // 15 * 20
			expectedRange:     10500.0,
		},
		{
			name:              "Starbase plasma",
			fuseTicks:         25,
			speedUnitsPerTick: 300.0, // 15 * 20
			expectedRange:     7500.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaxPlasmaRange(tt.fuseTicks, tt.speedUnitsPerTick)
			if result != tt.expectedRange {
				t.Errorf("MaxPlasmaRange(%d, %.1f) = %.1f, want %.1f",
					tt.fuseTicks, tt.speedUnitsPerTick, result, tt.expectedRange)
			}
		})
	}
}

func TestMaxPlasmaRangeForShip(t *testing.T) {
	tests := []struct {
		name          string
		ship          ShipType
		expectedRange float64
	}{
		{
			name:          "Scout (no plasma)",
			ship:          ShipScout,
			expectedRange: 0.0,
		},
		{
			name:          "Destroyer",
			ship:          ShipDestroyer,
			expectedRange: 9000.0, // 30 ticks * 300 units/tick
		},
		{
			name:          "Cruiser",
			ship:          ShipCruiser,
			expectedRange: 10500.0, // 35 ticks * 300 units/tick
		},
		{
			name:          "Battleship",
			ship:          ShipBattleship,
			expectedRange: 10500.0, // 35 ticks * 300 units/tick
		},
		{
			name:          "Assault (no plasma)",
			ship:          ShipAssault,
			expectedRange: 0.0,
		},
		{
			name:          "Starbase",
			ship:          ShipStarbase,
			expectedRange: 7500.0, // 25 ticks * 300 units/tick
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MaxPlasmaRangeForShip(tt.ship)
			if result != tt.expectedRange {
				t.Errorf("MaxPlasmaRangeForShip(%v) = %.1f, want %.1f",
					tt.ship, result, tt.expectedRange)
			}
		})
	}
}

func TestEffectivePlasmaRange(t *testing.T) {
	tests := []struct {
		name          string
		ship          ShipType
		safetyFactor  float64
		expectedRange float64
	}{
		{
			name:          "Scout (no plasma)",
			ship:          ShipScout,
			safetyFactor:  0.9,
			expectedRange: 0.0,
		},
		{
			name:          "Destroyer with 90% safety",
			ship:          ShipDestroyer,
			safetyFactor:  0.9,
			expectedRange: 8100.0, // 9000 * 0.9
		},
		{
			name:          "Cruiser with 85% safety",
			ship:          ShipCruiser,
			safetyFactor:  0.85,
			expectedRange: 8925.0, // 10500 * 0.85
		},
		{
			name:          "Starbase with 95% safety",
			ship:          ShipStarbase,
			safetyFactor:  0.95,
			expectedRange: 7125.0, // 7500 * 0.95
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EffectivePlasmaRange(tt.ship, tt.safetyFactor)
			if result != tt.expectedRange {
				t.Errorf("EffectivePlasmaRange(%v, %.2f) = %.1f, want %.1f",
					tt.ship, tt.safetyFactor, result, tt.expectedRange)
			}
		})
	}
}

func TestGetPlasmaRangeTable(t *testing.T) {
	ranges := GetPlasmaRangeTable()

	// Test that all ship types are included
	expectedShips := []ShipType{ShipScout, ShipDestroyer, ShipCruiser, ShipBattleship, ShipAssault, ShipStarbase}
	for _, ship := range expectedShips {
		if _, exists := ranges[ship]; !exists {
			t.Errorf("Missing ship type %v in range table", ship)
		}
	}

	// Test some specific expected values
	if ranges[ShipDestroyer] != 9000.0 {
		t.Errorf("Expected Destroyer range 9000.0, got %.1f", ranges[ShipDestroyer])
	}

	if ranges[ShipCruiser] != 10500.0 {
		t.Errorf("Expected Cruiser range 10500.0, got %.1f", ranges[ShipCruiser])
	}

	if ranges[ShipScout] != 0.0 {
		t.Errorf("Expected Scout range 0.0 (no plasma), got %.1f", ranges[ShipScout])
	}

	if ranges[ShipAssault] != 0.0 {
		t.Errorf("Expected Assault range 0.0 (no plasma), got %.1f", ranges[ShipAssault])
	}
}
