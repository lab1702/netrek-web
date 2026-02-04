package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestApplyDamageWithShields(t *testing.T) {
	tests := []struct {
		name            string
		initialShields  int
		initialDamage   int
		shieldsUp       bool
		damageAmount    int
		expectedShields int
		expectedDamage  int
		expectedDealt   int
	}{
		{
			name:            "shields fully absorb damage",
			initialShields:  100,
			initialDamage:   10,
			shieldsUp:       true,
			damageAmount:    50,
			expectedShields: 50,
			expectedDamage:  10,
			expectedDealt:   50,
		},
		{
			name:            "shields partially absorb, remainder to hull",
			initialShields:  30,
			initialDamage:   10,
			shieldsUp:       true,
			damageAmount:    50,
			expectedShields: 0,
			expectedDamage:  30, // 10 + (50-30)
			expectedDealt:   50,
		},
		{
			name:            "shields down, all damage to hull",
			initialShields:  100,
			initialDamage:   10,
			shieldsUp:       false,
			damageAmount:    50,
			expectedShields: 100, // Unchanged
			expectedDamage:  60,  // 10 + 50
			expectedDealt:   50,
		},
		{
			name:            "zero damage",
			initialShields:  100,
			initialDamage:   10,
			shieldsUp:       true,
			damageAmount:    0,
			expectedShields: 100,
			expectedDamage:  10,
			expectedDealt:   0,
		},
		{
			name:            "negative damage",
			initialShields:  100,
			initialDamage:   10,
			shieldsUp:       true,
			damageAmount:    -10,
			expectedShields: 100,
			expectedDamage:  10,
			expectedDealt:   0,
		},
		{
			name:            "shields up but at zero",
			initialShields:  0,
			initialDamage:   10,
			shieldsUp:       true,
			damageAmount:    50,
			expectedShields: 0,
			expectedDamage:  60, // 10 + 50
			expectedDealt:   50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &game.Player{
				Shields:    tt.initialShields,
				Damage:     tt.initialDamage,
				Shields_up: tt.shieldsUp,
			}

			dealt := game.ApplyDamageWithShields(player, tt.damageAmount)

			if player.Shields != tt.expectedShields {
				t.Errorf("Expected shields %d, got %d", tt.expectedShields, player.Shields)
			}
			if player.Damage != tt.expectedDamage {
				t.Errorf("Expected damage %d, got %d", tt.expectedDamage, player.Damage)
			}
			if dealt != tt.expectedDealt {
				t.Errorf("Expected damage dealt %d, got %d", tt.expectedDealt, dealt)
			}
		})
	}
}

func TestTorpedoShieldHandling(t *testing.T) {
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Create target player with shields
	target := server.gameState.Players[0]
	target.Status = game.StatusAlive
	target.Ship = game.ShipCruiser
	target.Shields = 100
	target.Shields_up = true
	target.Damage = 0

	// Create torpedo that does 40 damage
	torp := &game.Torpedo{
		Owner:  1,
		Damage: 40,
	}

	server.handleTorpedoHit(torp, target, 0)

	// Shields should absorb all damage
	if target.Shields != 60 {
		t.Errorf("Expected shields 60, got %d", target.Shields)
	}
	if target.Damage != 0 {
		t.Errorf("Expected hull damage 0, got %d", target.Damage)
	}
}

func TestPlasmaShieldHandling(t *testing.T) {
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Create target player with partial shields
	target := server.gameState.Players[0]
	target.Status = game.StatusAlive
	target.Ship = game.ShipCruiser
	target.Shields = 20
	target.Shields_up = true
	target.Damage = 0

	// Create plasma that does 50 damage
	plasma := &game.Plasma{
		Owner:  1,
		Damage: 50,
	}

	server.handlePlasmaHit(plasma, target, 0)

	// Shields should absorb 20, hull takes remaining 30
	if target.Shields != 0 {
		t.Errorf("Expected shields 0, got %d", target.Shields)
	}
	if target.Damage != 30 {
		t.Errorf("Expected hull damage 30, got %d", target.Damage)
	}
}

func TestShipExplosionShieldHandling(t *testing.T) {
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Setup exploding ship
	explodingShip := server.gameState.Players[0]
	explodingShip.Status = game.StatusExplode
	explodingShip.ExplodeTimer = game.ExplodeTimerFrames
	explodingShip.Ship = game.ShipCruiser
	explodingShip.X = 50000
	explodingShip.Y = 50000
	explodingShip.WhyDead = game.KillTorp

	// Setup target with shields down
	target := server.gameState.Players[1]
	target.Status = game.StatusAlive
	target.Ship = game.ShipScout
	target.X = 50000
	target.Y = 50000 // Same position for full damage
	target.Shields = 75
	target.Shields_up = false // Shields down
	target.Damage = 0

	// Simulate one game update tick to trigger explosion damage
	server.updateGame()

	// All damage should go to hull since shields are down
	explosionDamage := game.GetShipExplosionDamage(explodingShip.Ship) // Should be 100 for cruiser
	if target.Shields != 75 {
		t.Errorf("Expected shields unchanged at 75, got %d", target.Shields)
	}
	if target.Damage != explosionDamage {
		t.Errorf("Expected hull damage %d, got %d", explosionDamage, target.Damage)
	}
}

func TestPhaserShieldHandling(t *testing.T) {
	// Test that our refactored phaser still works correctly
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}
	server.clients = make(map[int]*Client)

	// Create shooter
	shooter := server.gameState.Players[0]
	shooter.Status = game.StatusAlive
	shooter.Ship = game.ShipDestroyer
	shooter.X = 0
	shooter.Y = 0
	shooter.Fuel = 1000

	// Create target with shields up
	target := server.gameState.Players[1]
	target.Status = game.StatusAlive
	target.Ship = game.ShipScout
	target.Team = game.TeamKli // Different team
	target.X = 1000            // Within phaser range
	target.Y = 0
	target.Shields = 50
	target.Shields_up = true
	target.Damage = 0

	// Create a mock client
	client := &Client{
		server: server,
		send:   make(chan ServerMessage, 10),
	}
	client.SetPlayerID(0)

	// Calculate expected damage: 85 * (1.0 - 1000/5100) ≈ 68.3 → 68
	shipStats := game.ShipData[shooter.Ship]
	phaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)
	dist := float64(1000)
	expectedDamage := int(float64(shipStats.PhaserDamage) * (1.0 - dist/phaserRange))

	// Fire phaser through client handler (tests our refactored code)
	phaserData := PhaserData{
		Target: 1,
	}
	data, _ := json.Marshal(phaserData)
	client.handlePhaser(data)

	// Check damage was applied correctly through shields
	if expectedDamage <= 50 {
		// All absorbed by shields
		if target.Shields != 50-expectedDamage {
			t.Errorf("Expected shields %d, got %d", 50-expectedDamage, target.Shields)
		}
		if target.Damage != 0 {
			t.Errorf("Expected hull damage 0, got %d", target.Damage)
		}
	} else {
		// Partial absorption
		if target.Shields != 0 {
			t.Errorf("Expected shields 0, got %d", target.Shields)
		}
		if target.Damage != expectedDamage-50 {
			t.Errorf("Expected hull damage %d, got %d", expectedDamage-50, target.Damage)
		}
	}
}
