package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestBlendWithSeparation(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  float64
		sep      SeparationVector
		divisor  float64
		maxWt    float64
		expected float64 // approximate expected direction
		tolerDeg float64 // tolerance in degrees
	}{
		{
			name:    "Zero magnitude returns baseDir unchanged",
			baseDir: math.Pi / 4,
			sep:     SeparationVector{x: 1, y: 0, magnitude: 0},
			divisor: 300, maxWt: 0.5,
			expected: math.Pi / 4, tolerDeg: 0.01,
		},
		{
			name:    "Full weight caps at maxWeight",
			baseDir: 0,                                              // east
			sep:     SeparationVector{x: 0, y: 1, magnitude: 10000}, // north, huge magnitude
			divisor: 300, maxWt: 0.5,
			expected: math.Atan2(0.5, 0.5), // blended 50/50 -> ~45 degrees
			tolerDeg: 1,
		},
		{
			name:    "Low magnitude gives small deflection",
			baseDir: 0,                                           // east
			sep:     SeparationVector{x: 0, y: 1, magnitude: 30}, // north, small
			divisor: 300, maxWt: 0.5,
			expected: 0, // 30/300=0.1 weight, mostly east
			tolerDeg: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := blendWithSeparation(tt.baseDir, tt.sep, tt.divisor, tt.maxWt)
			diff := math.Abs(result - tt.expected)
			if diff > math.Pi {
				diff = 2*math.Pi - diff
			}
			tolerRad := tt.tolerDeg * math.Pi / 180
			if diff > tolerRad {
				t.Errorf("blendWithSeparation() = %f rad, want ~%f rad (±%f°), got diff %f°",
					result, tt.expected, tt.tolerDeg, diff*180/math.Pi)
			}
		})
	}
}

func TestCalculateClearanceWithPlanets(t *testing.T) {
	// Create a player in the middle of the galaxy
	p := &game.Player{
		X: game.GalaxyWidth / 2,
		Y: game.GalaxyHeight / 2,
	}

	t.Run("No planets - clearance limited by walls", func(t *testing.T) {
		clearance := calculateClearanceWithPlanets(p, 0, nil)
		// Test point is 5000 units east of center. Wall clearance should
		// be min of all wall distances from that point.
		testX := p.X + 5000
		testY := p.Y
		expectedMin := math.Min(testX, game.GalaxyWidth-testX)
		expectedMin = math.Min(expectedMin, testY)
		expectedMin = math.Min(expectedMin, game.GalaxyHeight-testY)
		if math.Abs(clearance-expectedMin) > 1 {
			t.Errorf("clearance = %f, want %f", clearance, expectedMin)
		}
	})

	t.Run("Planet directly ahead reduces clearance", func(t *testing.T) {
		// Place planet 5000 units east (right where test point lands)
		planets := []planetPos{{x: p.X + 5000, y: p.Y}}
		clearance := calculateClearanceWithPlanets(p, 0, planets)
		// Planet is at test point -> distance = 0, clearance = max(0 - 2000, 0) = 0
		if clearance != 0 {
			t.Errorf("clearance = %f, want 0 (planet at test point)", clearance)
		}
	})

	t.Run("Planet far from test direction does not affect clearance", func(t *testing.T) {
		// Planet far south-west, we're testing east — planet clearance exceeds wall clearance
		planets := []planetPos{{x: p.X - 40000, y: p.Y + 40000}}
		clearanceWithPlanet := calculateClearanceWithPlanets(p, 0, planets)
		clearanceWithout := calculateClearanceWithPlanets(p, 0, nil)
		if clearanceWithPlanet != clearanceWithout {
			t.Errorf("distant planet affected clearance: %f vs %f", clearanceWithPlanet, clearanceWithout)
		}
	})
}

func TestGetOptimalSpeed(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	tests := []struct {
		name     string
		ship     game.ShipType
		dist     float64
		minSpeed float64
		maxSpeed float64
	}{
		{"Very close returns minimum", game.ShipDestroyer, 100, 2, 2},
		{"Starbase always returns 2", game.ShipStarbase, 50000, 2, 2},
		{"Medium distance returns moderate speed", game.ShipCruiser, 5000, 2, 9},
		{"Long distance returns higher speed", game.ShipScout, 20000, 5, 12},
		{"Short distance returns low speed", game.ShipBattleship, 500, 2, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := gs.Players[0]
			p.Ship = tt.ship
			p.Status = game.StatusAlive

			speed := server.getOptimalSpeed(p, tt.dist)
			if speed < tt.minSpeed || speed > tt.maxSpeed {
				t.Errorf("getOptimalSpeed(%s, %.0f) = %f, want [%f, %f]",
					game.ShipData[tt.ship].Name, tt.dist, speed, tt.minSpeed, tt.maxSpeed)
			}
		})
	}
}

func TestGetOptimalCombatSpeed(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	p := gs.Players[0]
	p.Ship = game.ShipDestroyer
	p.Status = game.StatusAlive
	maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)

	t.Run("Far distance returns max speed", func(t *testing.T) {
		speed := server.getOptimalCombatSpeed(p, 7000)
		if speed != maxSpeed {
			t.Errorf("got %f, want %f (max speed at long range)", speed, maxSpeed)
		}
	})

	t.Run("Close distance returns reduced speed", func(t *testing.T) {
		speed := server.getOptimalCombatSpeed(p, 1000)
		if speed >= maxSpeed*0.5 {
			t.Errorf("got %f, want < %f (reduced speed at close range)", speed, maxSpeed*0.5)
		}
	})

	t.Run("Speed decreases as distance decreases", func(t *testing.T) {
		far := server.getOptimalCombatSpeed(p, 5000)
		mid := server.getOptimalCombatSpeed(p, 2000)
		close := server.getOptimalCombatSpeed(p, 1000)
		if !(far >= mid && mid >= close) {
			t.Errorf("speed should decrease with distance: far=%f, mid=%f, close=%f", far, mid, close)
		}
	})
}

func TestGetEvasionSpeed(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	p := gs.Players[0]
	p.Ship = game.ShipDestroyer
	p.Status = game.StatusAlive
	maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)

	t.Run("High threat returns max speed", func(t *testing.T) {
		threats := CombatThreat{threatLevel: 8}
		speed := server.getEvasionSpeed(p, threats)
		if speed != maxSpeed {
			t.Errorf("got %f, want %f (max speed under high threat)", speed, maxSpeed)
		}
	})

	t.Run("Medium threat returns at least 60% max speed", func(t *testing.T) {
		threats := CombatThreat{threatLevel: 3}
		speed := server.getEvasionSpeed(p, threats)
		if speed < maxSpeed*0.6 || speed > maxSpeed {
			t.Errorf("got %f, want [%f, %f] (variable speed under medium threat)",
				speed, maxSpeed*0.6, maxSpeed)
		}
	})

	t.Run("Low threat returns combat speed", func(t *testing.T) {
		threats := CombatThreat{threatLevel: 1}
		speed := server.getEvasionSpeed(p, threats)
		if speed <= 0 || speed > maxSpeed {
			t.Errorf("got %f, want (0, %f] (combat speed under low threat)", speed, maxSpeed)
		}
	})
}
