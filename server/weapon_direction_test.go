package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestWeaponDirectionIndependence verifies that all bot weapon functions
// fire projectiles toward the target regardless of the ship's facing direction.
// This test ensures weapons have no facing restrictions.
func TestWeaponDirectionIndependence(t *testing.T) {
	// Create server and game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Test cases with different ship facings and target positions
	testCases := []struct {
		name          string
		shipDir       float64 // Ship facing direction
		targetX       float64 // Target position
		targetY       float64
		expectedAngle float64 // Expected weapon firing direction
	}{
		{
			name:          "Ship facing North, target East",
			shipDir:       math.Pi / 2, // 90° (North)
			targetX:       55000,       // East of ship
			targetY:       50000,       // Same Y as ship
			expectedAngle: 0,           // Should fire East (0°)
		},
		{
			name:          "Ship facing East, target North",
			shipDir:       0,           // 0° (East)
			targetX:       50000,       // Same X as ship
			targetY:       55000,       // North of ship
			expectedAngle: math.Pi / 2, // Should fire North (90°)
		},
		{
			name:          "Ship facing South, target West",
			shipDir:       -math.Pi / 2, // -90° (South)
			targetX:       45000,        // West of ship
			targetY:       50000,        // Same Y as ship
			expectedAngle: math.Pi,      // Should fire West (180°)
		},
		{
			name:          "Ship facing West, target South",
			shipDir:       math.Pi,      // 180° (West)
			targetX:       50000,        // Same X as ship
			targetY:       45000,        // South of ship
			expectedAngle: -math.Pi / 2, // Should fire South (-90°)
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset game state for each test
			gs.Torps = make([]*game.Torpedo, 0)
			gs.Plasmas = make([]*game.Plasma, 0)

			// Create shooter bot
			shooter := gs.Players[0]
			shooter.Status = game.StatusAlive
			shooter.Team = game.TeamFed
			shooter.Ship = game.ShipDestroyer
			shooter.X = 50000
			shooter.Y = 50000
			shooter.Dir = tc.shipDir // Set ship facing direction
			shooter.Fuel = 10000
			shooter.WTemp = 0
			shooter.NumTorps = 0
			shooter.NumPlasma = 0
			shooter.Cloaked = false
			shooter.Repairing = false

			// Create stationary target
			target := gs.Players[1]
			target.Status = game.StatusAlive
			target.Team = game.TeamKli
			target.Ship = game.ShipCruiser
			target.X = tc.targetX
			target.Y = tc.targetY
			target.Speed = 0 // Stationary target
			target.Dir = 0

			t.Run("fireBotTorpedo", func(t *testing.T) {
				initialTorps := len(gs.Torps)
				server.fireBotTorpedo(shooter, target)

				if len(gs.Torps) <= initialTorps {
					t.Fatal("No torpedo was fired")
				}

				torp := gs.Torps[len(gs.Torps)-1]

				// Verify torpedo direction is toward target, not ship direction
				if math.Abs(torp.Dir-tc.expectedAngle) > 0.1 { // Allow small tolerance for jitter
					t.Errorf("Torpedo fired in wrong direction: got %.3f, expected %.3f (ship facing %.3f)",
						torp.Dir, tc.expectedAngle, tc.shipDir)
				}

				// Verify torpedo direction is NOT the same as ship direction (unless coincidentally)
				if math.Abs(torp.Dir-tc.shipDir) < 0.1 && math.Abs(tc.expectedAngle-tc.shipDir) > 0.1 {
					t.Errorf("Torpedo direction incorrectly matches ship direction: %.3f", torp.Dir)
				}
			})

			t.Run("fireBotTorpedoWithLead", func(t *testing.T) {
				initialTorps := len(gs.Torps)
				server.fireBotTorpedoWithLead(shooter, target)

				if len(gs.Torps) <= initialTorps {
					t.Fatal("No torpedo was fired")
				}

				torp := gs.Torps[len(gs.Torps)-1]

				// For stationary target, should fire directly at target
				if math.Abs(torp.Dir-tc.expectedAngle) > 0.1 {
					t.Errorf("Torpedo with lead fired in wrong direction: got %.3f, expected %.3f",
						torp.Dir, tc.expectedAngle)
				}
			})

			t.Run("fireEnhancedTorpedo", func(t *testing.T) {
				initialTorps := len(gs.Torps)
				server.fireEnhancedTorpedo(shooter, target)

				if len(gs.Torps) <= initialTorps {
					t.Fatal("No torpedo was fired")
				}

				torp := gs.Torps[len(gs.Torps)-1]

				// Should fire toward target regardless of ship direction
				if math.Abs(torp.Dir-tc.expectedAngle) > 0.1 {
					t.Errorf("Enhanced torpedo fired in wrong direction: got %.3f, expected %.3f",
						torp.Dir, tc.expectedAngle)
				}
			})

			// Test plasma firing for ships that have plasma
			if game.ShipData[shooter.Ship].HasPlasma {
				t.Run("fireBotPlasma", func(t *testing.T) {
					initialPlasmas := len(gs.Plasmas)
					server.fireBotPlasma(shooter, target)

					if len(gs.Plasmas) <= initialPlasmas {
						t.Fatal("No plasma was fired")
					}

					plasma := gs.Plasmas[len(gs.Plasmas)-1]

					// Should fire toward target regardless of ship direction
					if math.Abs(plasma.Dir-tc.expectedAngle) > 0.1 {
						t.Errorf("Plasma fired in wrong direction: got %.3f, expected %.3f",
							plasma.Dir, tc.expectedAngle)
					}
				})
			}

			// Test phaser firing (visual confirmation - phasers hit instantly)
			t.Run("fireBotPhaser", func(t *testing.T) {
				initialDamage := target.Damage
				server.fireBotPhaser(shooter, target)

				// Phaser should hit regardless of ship direction if target is in range
				dist := game.Distance(shooter.X, shooter.Y, target.X, target.Y)
				shipStats := game.ShipData[shooter.Ship]
				phaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0

				if dist <= phaserRange {
					if target.Damage <= initialDamage {
						t.Errorf("Phaser should have hit target (distance: %.0f, range: %.0f)", dist, phaserRange)
					}
				}
			})
		})
	}
}

// TestTorpedoSpreadPattern verifies that torpedo spread patterns
// are centered on the target direction, not ship direction
func TestTorpedoSpreadPattern(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Create shooter facing North
	shooter := gs.Players[0]
	shooter.Status = game.StatusAlive
	shooter.Team = game.TeamFed
	shooter.Ship = game.ShipCruiser
	shooter.X = 50000
	shooter.Y = 50000
	shooter.Dir = math.Pi / 2 // Facing North
	shooter.Fuel = 10000
	shooter.WTemp = 0
	shooter.NumTorps = 0

	// Create target to the East (perpendicular to ship facing)
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.X = 55000
	target.Y = 50000
	target.Speed = 0

	// Fire 3-torpedo spread
	initialTorps := len(gs.Torps)
	server.fireTorpedoSpread(shooter, target, 3)

	if len(gs.Torps) < initialTorps+3 {
		t.Fatalf("Expected 3 torpedoes, got %d", len(gs.Torps)-initialTorps)
	}

	// Check that all torpedoes are aimed generally toward the target (East)
	// and not toward ship facing direction (North)
	targetDirection := math.Atan2(target.Y-shooter.Y, target.X-shooter.X) // Should be 0 (East)

	for i := initialTorps; i < len(gs.Torps); i++ {
		torp := gs.Torps[i]

		// All torpedoes should be aimed roughly East, not North
		angleDiffFromTarget := math.Abs(torp.Dir - targetDirection)
		if angleDiffFromTarget > math.Pi {
			angleDiffFromTarget = 2*math.Pi - angleDiffFromTarget
		}

		angleDiffFromShip := math.Abs(torp.Dir - shooter.Dir)
		if angleDiffFromShip > math.Pi {
			angleDiffFromShip = 2*math.Pi - angleDiffFromShip
		}

		// Torpedo should be closer to target direction than ship direction
		if angleDiffFromTarget > math.Pi/4 { // Within 45° of target
			t.Errorf("Torpedo %d not aimed toward target: dir=%.3f, target_dir=%.3f, ship_dir=%.3f",
				i, torp.Dir, targetDirection, shooter.Dir)
		}

		// Should not be aligned with ship direction (unless coincidentally same as target)
		if angleDiffFromShip < 0.1 && angleDiffFromTarget > 0.1 {
			t.Errorf("Torpedo %d incorrectly aligned with ship direction instead of target", i)
		}
	}
}

// TestPlanetDefenseWeaponsIgnoreHeading verifies that planet defense weapons
// fire toward enemies regardless of ship heading
func TestPlanetDefenseWeaponsIgnoreHeading(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Create defender facing South
	defender := gs.Players[0]
	defender.Status = game.StatusAlive
	defender.Team = game.TeamFed
	defender.Ship = game.ShipDestroyer
	defender.X = 50000
	defender.Y = 50000
	defender.Dir = -math.Pi / 2 // Facing South
	defender.Fuel = 10000
	defender.WTemp = 0
	defender.NumTorps = 0

	// Create enemy to the North (opposite of ship facing)
	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamKli
	enemy.X = 50000
	enemy.Y = 55000 // North of defender
	enemy.Speed = 0

	enemyDist := game.Distance(defender.X, defender.Y, enemy.X, enemy.Y)

	// Test planet defense weapon logic
	initialTorps := len(gs.Torps)
	server.planetDefenseWeaponLogic(defender, enemy, enemyDist)

	if len(gs.Torps) > initialTorps {
		// A torpedo was fired - verify it's aimed at enemy, not ship direction
		torp := gs.Torps[len(gs.Torps)-1]
		expectedDir := math.Atan2(enemy.Y-defender.Y, enemy.X-defender.X) // North = π/2

		if math.Abs(torp.Dir-expectedDir) > 0.1 {
			t.Errorf("Planet defense torpedo not aimed at enemy: got %.3f, expected %.3f (ship facing %.3f)",
				torp.Dir, expectedDir, defender.Dir)
		}

		// Should definitely NOT be firing South (ship direction)
		if math.Abs(torp.Dir-defender.Dir) < 0.1 {
			t.Error("Planet defense torpedo incorrectly firing in ship direction instead of toward enemy")
		}
	}
}
