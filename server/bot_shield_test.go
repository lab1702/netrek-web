package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestBotShieldAssessment(t *testing.T) {
	tests := []struct {
		name           string
		fuel           int
		setupTorpedoes func(gs *game.GameState, botID int)
		setupEnemies   func(gs *game.GameState, botID int)
		setupPlasmas   func(gs *game.GameState, botID int)
		expectedShield bool
		description    string
	}{
		{
			name: "Torpedo 3000u inbound - shields up",
			fuel: 1000,
			setupTorpedoes: func(gs *game.GameState, botID int) {
				// Create very close threatening torpedo
				bot := gs.Players[botID]
				torp := &game.Torpedo{
					ID:     0,
					Owner:  1,            // Different from bot
					X:      bot.X + 1900, // Within TorpedoVeryClose range
					Y:      bot.Y,
					Dir:    math.Pi,       // Heading west (toward bot)
					Speed:  600,           // Fast torpedo
					Status: game.TorpMove, // Moving
					Team:   game.TeamKli,  // Enemy team
				}
				gs.Torps = append(gs.Torps, torp)
			},
			expectedShield: true,
			description:    "Bot should shield against incoming torpedo at 3000u",
		},
		{
			name: "Enemy ship 2000u away - shields up even with low fuel",
			fuel: 700, // Low fuel, but should still shield
			setupEnemies: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				enemy := gs.Players[1]
				enemy.Status = game.StatusAlive
				enemy.Team = game.TeamKli // Enemy team
				enemy.Ship = game.ShipDestroyer
				enemy.X = bot.X + 2000 // 2000 units away
				enemy.Y = bot.Y
				enemy.Speed = 5
			},
			expectedShield: true,
			description:    "Bot should shield against close enemy even with lower fuel",
		},
		{
			name: "No threats with low fuel - shields down",
			fuel: 500, // Above FuelCritical but below FuelLow
			setupTorpedoes: func(gs *game.GameState, botID int) {
				// No torpedoes
			},
			setupEnemies: func(gs *game.GameState, botID int) {
				// No close enemies
			},
			expectedShield: false,
			description:    "Bot should not shield with no threats and low fuel",
		},
		{
			name: "Critical fuel - no shields even with threats",
			fuel: 300, // Below FuelCritical
			setupTorpedoes: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				torp := &game.Torpedo{
					ID:     0,
					Owner:  1,
					X:      bot.X + 1500,
					Y:      bot.Y,
					Dir:    math.Pi,
					Speed:  600,
					Status: game.TorpMove,
					Team:   game.TeamKli,
				}
				gs.Torps = append(gs.Torps, torp)
			},
			expectedShield: false,
			description:    "Bot should not shield with critical fuel even with threats",
		},
		{
			name: "Very close torpedo - shields up with minimal fuel",
			fuel: 950, // Just above FuelLow
			setupTorpedoes: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				torp := &game.Torpedo{
					ID:     0,
					Owner:  1,
					X:      bot.X + 1800, // Within TorpedoVeryClose range
					Y:      bot.Y,
					Dir:    math.Pi,
					Speed:  600,
					Status: game.TorpMove,
					Team:   game.TeamKli,
				}
				gs.Torps = append(gs.Torps, torp)
			},
			expectedShield: true,
			description:    "Bot should shield against very close torpedo",
		},
		{
			name: "Carrying armies with enemy nearby - shields up",
			fuel: 1000,
			setupEnemies: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				bot.Armies = 3 // Carrying armies

				enemy := gs.Players[1]
				enemy.Status = game.StatusAlive
				enemy.Team = game.TeamKli
				enemy.Ship = game.ShipCruiser
				enemy.X = bot.X + 3200 // Within ArmyCarryingRange
				enemy.Y = bot.Y
			},
			expectedShield: true,
			description:    "Bot carrying armies should shield when enemy is nearby",
		},
		{
			name: "Planet defense with close enemy - shields up",
			fuel: 1000,
			setupEnemies: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				bot.BotDefenseTarget = 0 // Defending planet

				enemy := gs.Players[1]
				enemy.Status = game.StatusAlive
				enemy.Team = game.TeamKli
				enemy.Ship = game.ShipScout
				enemy.X = bot.X + 2800 // Within DefenseShieldRange
				enemy.Y = bot.Y
			},
			expectedShield: true,
			description:    "Bot defending planet should shield when enemy is close",
		},
		{
			name: "Plasma threat nearby - shields up",
			fuel: 1500,
			setupPlasmas: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				plasma := &game.Plasma{
					ID:     0,
					Owner:  1,
					X:      bot.X + 1800, // Within PlasmaClose range
					Y:      bot.Y,
					Dir:    math.Pi,
					Speed:  300,
					Status: game.TorpMove,
					Team:   game.TeamKli,
				}
				gs.Plasmas = append(gs.Plasmas, plasma)
			},
			expectedShield: true,
			description:    "Bot should shield against close plasma",
		},
		{
			name: "Enemy within phaser range - shields up",
			fuel: 1600,
			setupEnemies: func(gs *game.GameState, botID int) {
				bot := gs.Players[botID]
				enemy := gs.Players[1]
				enemy.Status = game.StatusAlive
				enemy.Team = game.TeamKli
				enemy.Ship = game.ShipDestroyer

				// Position enemy within 80% of phaser range
				enemyStats := game.ShipData[enemy.Ship]
				phaserRange := float64(game.PhaserDist) * float64(enemyStats.PhaserDamage) / 100.0
				distance := phaserRange * PhaserRangeFactor * 0.9 // Within 80% range

				enemy.X = bot.X + distance
				enemy.Y = bot.Y
			},
			expectedShield: true,
			description:    "Bot should shield when enemy is within phaser range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal game state
			gs := game.NewGameState()
			server := &Server{
				gameState: gs,
				broadcast: make(chan ServerMessage, 100),
			}

			// Setup bot player
			bot := gs.Players[0]
			bot.Status = game.StatusAlive
			bot.Team = game.TeamFed
			bot.Ship = game.ShipDestroyer
			bot.X = 50000
			bot.Y = 50000
			bot.Fuel = tt.fuel
			bot.Shields_up = false    // Start with shields down
			bot.Armies = 0            // Default no armies
			bot.BotDefenseTarget = -1 // Default not defending
			// Set movement data for threat assessment
			bot.Speed = 0
			bot.DesSpeed = 0
			bot.Dir = 0
			bot.DesDir = 0

			// Initialize other players as not alive (except enemy setup)
			for i := 1; i < game.MaxPlayers; i++ {
				gs.Players[i].Status = game.StatusFree
			}

			// Setup test scenario
			if tt.setupTorpedoes != nil {
				tt.setupTorpedoes(gs, 0)
			}
			if tt.setupEnemies != nil {
				tt.setupEnemies(gs, 0)
			}
			if tt.setupPlasmas != nil {
				tt.setupPlasmas(gs, 0)
			}

			// Run shield assessment
			server.assessAndActivateShields(bot, nil)

			// Check result
			if bot.Shields_up != tt.expectedShield {
				t.Errorf("%s: expected shields_up=%v, got %v",
					tt.description, tt.expectedShield, bot.Shields_up)

				// Additional debug info
				t.Logf("Bot fuel: %d", bot.Fuel)
				t.Logf("Number of torpedoes: %d", len(gs.Torps))
				t.Logf("Number of plasmas: %d", len(gs.Plasmas))
				enemyCount := 0
				for _, enemy := range gs.Players {
					if enemy.Status == game.StatusAlive && enemy.Team != bot.Team {
						enemyCount++
						dist := game.Distance(bot.X, bot.Y, enemy.X, enemy.Y)
						t.Logf("Enemy at distance: %.0f", dist)
					}
				}
				t.Logf("Number of alive enemies: %d", enemyCount)
			}
		})
	}
}

func TestShieldAssessmentConstants(t *testing.T) {
	// Test that our constants are reasonable
	tests := []struct {
		name     string
		constant interface{}
		check    func(interface{}) bool
		desc     string
	}{
		{"FuelCritical", FuelCritical, func(v interface{}) bool { return v.(int) > 0 && v.(int) < 1000 }, "Should be positive and reasonable"},
		{"FuelLow", FuelLow, func(v interface{}) bool { return v.(int) > FuelCritical && v.(int) < 2000 }, "Should be above critical but reasonable"},
		{"FuelModerate", FuelModerate, func(v interface{}) bool { return v.(int) > FuelLow && v.(int) < 3000 }, "Should be above low but reasonable"},
		{"TorpedoVeryClose", TorpedoVeryClose, func(v interface{}) bool { return v.(float64) > 1000 && v.(float64) < 3000 }, "Should be reasonable torpedo range"},
		{"EnemyClose", EnemyClose, func(v interface{}) bool { return v.(float64) > 2000 && v.(float64) < 4000 }, "Should be reasonable enemy detection range"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.check(tt.constant) {
				t.Errorf("Constant %s failed check: %s (value: %v)", tt.name, tt.desc, tt.constant)
			}
		})
	}
}

func TestTorpedoThreatDetection(t *testing.T) {
	// Test the isTorpedoThreatening function specifically
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	bot := gs.Players[0]
	bot.X = 50000
	bot.Y = 50000
	bot.DesDir = 0   // Heading east
	bot.DesSpeed = 5 // Moving
	bot.Speed = 5
	bot.Dir = 0 // Current direction also east

	tests := []struct {
		name      string
		torpX     float64
		torpY     float64
		torpDir   float64
		torpSpeed float64
		expected  bool
	}{
		{
			name:      "Torpedo heading directly at bot",
			torpX:     47000, // 3000 units west of bot
			torpY:     50000, // Same Y
			torpDir:   0,     // Heading east (toward bot)
			torpSpeed: 600,
			expected:  true,
		},
		{
			name:      "Torpedo heading away from bot",
			torpX:     47000,   // 3000 units west of bot
			torpY:     50000,   // Same Y
			torpDir:   math.Pi, // Heading west (away from bot)
			torpSpeed: 600,
			expected:  false,
		},
		{
			name:      "Very close torpedo regardless of direction",
			torpX:     50900, // Very close
			torpY:     50000,
			torpDir:   math.Pi / 2, // Heading north (perpendicular)
			torpSpeed: 600,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			torp := &game.Torpedo{
				X:     tt.torpX,
				Y:     tt.torpY,
				Dir:   tt.torpDir,
				Speed: tt.torpSpeed,
			}

			result := server.isTorpedoThreatening(bot, torp)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for torpedo threat detection", tt.expected, result)
			}
		})
	}
}
