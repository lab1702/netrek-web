package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestBotPlasmaFiringDecisions(t *testing.T) {
	// Create server
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Test cases for different ship types and distances
	tests := []struct {
		name           string
		ship           game.ShipType
		targetDistance float64
		shouldFire     bool
		description    string
	}{
		{
			name:           "Destroyer within range",
			ship:           game.ShipDestroyer,
			targetDistance: 8000, // Within 9000 max range
			shouldFire:     true,
			description:    "Destroyer should fire at 8000 units (within 9000 max)",
		},
		{
			name:           "Destroyer beyond range",
			ship:           game.ShipDestroyer,
			targetDistance: 10000, // Beyond 9000 max range
			shouldFire:     false,
			description:    "Destroyer should NOT fire at 10000 units (beyond 9000 max)",
		},
		{
			name:           "Cruiser within range",
			ship:           game.ShipCruiser,
			targetDistance: 10000, // Within 10500 max range
			shouldFire:     true,
			description:    "Cruiser should fire at 10000 units (within 10500 max)",
		},
		{
			name:           "Cruiser beyond range",
			ship:           game.ShipCruiser,
			targetDistance: 11000, // Beyond 10500 max range
			shouldFire:     false,
			description:    "Cruiser should NOT fire at 11000 units (beyond 10500 max)",
		},
		{
			name:           "Starbase within range",
			ship:           game.ShipStarbase,
			targetDistance: 7000, // Within 7500 max range
			shouldFire:     true,
			description:    "Starbase should fire at 7000 units (within 7500 max)",
		},
		{
			name:           "Starbase beyond range",
			ship:           game.ShipStarbase,
			targetDistance: 8000, // Beyond 7500 max range
			shouldFire:     false,
			description:    "Starbase should NOT fire at 8000 units (beyond 7500 max)",
		},
		{
			name:           "Scout (no plasma)",
			ship:           game.ShipScout,
			targetDistance: 5000,
			shouldFire:     false,
			description:    "Scout should never fire plasma (no plasma capability)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset game state
			server.gameState = game.NewGameState()

			// Set up shooter bot
			shooter := server.gameState.Players[0]
			shooter.Status = game.StatusAlive
			shooter.Ship = tt.ship
			shooter.X = 50000
			shooter.Y = 50000
			shooter.Fuel = 10000
			shooter.NumPlasma = 0
			shooter.Cloaked = false
			shooter.Repairing = false
			shooter.Team = game.TeamFed

			// Set up target
			target := server.gameState.Players[1]
			target.Status = game.StatusAlive
			target.Ship = game.ShipCruiser
			target.X = shooter.X + tt.targetDistance // Place at specific distance
			target.Y = shooter.Y
			target.Team = game.TeamKli // Different team

			// Record initial plasma count
			initialPlasmaCount := len(server.gameState.Plasmas)

			// Attempt to fire plasma
			server.fireBotPlasma(shooter, target)

			// Check result
			finalPlasmaCount := len(server.gameState.Plasmas)
			plasmaWasFired := finalPlasmaCount > initialPlasmaCount

			if plasmaWasFired != tt.shouldFire {
				t.Errorf("%s: expected shouldFire=%v, got plasmaWasFired=%v",
					tt.description, tt.shouldFire, plasmaWasFired)

				// Additional debugging info
				maxRange := game.MaxPlasmaRangeForShip(tt.ship)
				t.Logf("Ship: %v, Distance: %.0f, MaxRange: %.0f, HasPlasma: %v",
					tt.ship, tt.targetDistance, maxRange, game.ShipData[tt.ship].HasPlasma)
			}
		})
	}
}

func TestBotPlasmaMaxRangeCalculations(t *testing.T) {
	// Verify our understanding of plasma max ranges matches the actual calculations
	expectedRanges := map[game.ShipType]float64{
		game.ShipScout:      0,     // No plasma
		game.ShipDestroyer:  9000,  // 30 ticks * 300 units/tick
		game.ShipCruiser:    10500, // 35 ticks * 300 units/tick
		game.ShipBattleship: 10500, // 35 ticks * 300 units/tick
		game.ShipAssault:    0,     // No plasma
		game.ShipStarbase:   7500,  // 25 ticks * 300 units/tick
	}

	for ship, expectedRange := range expectedRanges {
		actualRange := game.MaxPlasmaRangeForShip(ship)
		if actualRange != expectedRange {
			t.Errorf("Ship %v: expected max range %.0f, got %.0f", ship, expectedRange, actualRange)
		}
	}
}

func TestBotPlasmaFiring95Percent(t *testing.T) {
	// Integration test: plasma should reach target at 95% of max range
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Test with Cruiser (10500 max range)
	shooter := server.gameState.Players[0]
	shooter.Status = game.StatusAlive
	shooter.Ship = game.ShipCruiser
	shooter.X = 50000
	shooter.Y = 50000
	shooter.Fuel = 10000
	shooter.NumPlasma = 0
	shooter.Team = game.TeamFed

	// Place target at 95% of max range (10500 * 0.95 = 9975)
	target := server.gameState.Players[1]
	target.Status = game.StatusAlive
	target.Ship = game.ShipDestroyer
	target.X = 50000 + 9975 // 95% of max range
	target.Y = 50000
	target.Speed = 0 // Stationary target
	target.Team = game.TeamKli

	// Fire plasma
	server.fireBotPlasma(shooter, target)

	// Verify plasma was created
	if len(server.gameState.Plasmas) != 1 {
		t.Fatalf("Expected 1 plasma to be fired, got %d", len(server.gameState.Plasmas))
	}

	plasma := server.gameState.Plasmas[0]

	// Simulate plasma movement until it either hits target or expires
	hitTarget := false
	for plasma.Fuse > 0 && !hitTarget {
		// Move plasma
		server.updatePlasmas()

		// Check if plasma still exists (it gets removed from slice when it hits or expires)
		if len(server.gameState.Plasmas) == 0 {
			// Plasma was removed - check if target was hit
			if target.Damage > 0 {
				hitTarget = true
			}
			break
		}

		// Check for hit
		if len(server.gameState.Plasmas) > 0 {
			currentPlasma := server.gameState.Plasmas[0]
			dist := game.Distance(currentPlasma.X, currentPlasma.Y, target.X, target.Y)
			if dist < 1500 { // Plasma explosion radius
				hitTarget = true
				break
			}
		}
	}

	if !hitTarget {
		t.Error("Plasma fired at 95% of max range should have hit the target before fuse expiry")
		t.Logf("Final target damage: %d", target.Damage)
		t.Logf("Plasmas remaining: %d", len(server.gameState.Plasmas))
		if len(server.gameState.Plasmas) > 0 {
			t.Logf("Final plasma fuse: %d", server.gameState.Plasmas[0].Fuse)
		}
	}
}
