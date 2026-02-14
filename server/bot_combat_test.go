package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestBotCombatImprovements(t *testing.T) {
	// Create minimal game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Set up players
	bot1 := gs.Players[0]
	bot2 := gs.Players[1]
	enemy := gs.Players[2]

	// Configure bot1
	bot1.Status = game.StatusAlive
	bot1.Team = game.TeamFed
	bot1.Ship = game.ShipCruiser
	bot1.IsBot = true
	bot1.Connected = true
	bot1.Fuel = 10000
	bot1.NumTorps = 0

	// Configure bot2
	bot2.Status = game.StatusAlive
	bot2.Team = game.TeamFed
	bot2.Ship = game.ShipDestroyer
	bot2.IsBot = true
	bot2.Connected = true
	bot2.Fuel = 10000
	bot2.NumTorps = 0

	// Configure enemy
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipCruiser
	enemy.IsBot = false
	enemy.Connected = true

	// Position bots near each other and enemy in range
	bot1.X, bot1.Y = 50000, 50000
	bot2.X, bot2.Y = 51000, 50000   // Near bot1 for coordination
	enemy.X, enemy.Y = 53000, 50000 // In combat range

	// Damage enemy to test burst fire mode
	enemy.Damage = game.ShipData[enemy.Ship].MaxDamage * 3 / 4 // 75% damaged

	// Test 1: Target selection - test selectBestCombatTarget directly
	// This avoids dependence on the full bot pipeline (planet defense, repair, etc.)
	target := server.SelectBestCombatTarget(bot1)
	if target == nil {
		t.Errorf("Bot1 selectBestCombatTarget returned nil, expected damaged enemy")
	} else if target.ID != enemy.ID {
		t.Errorf("Bot1 selectBestCombatTarget chose player %d, expected %d (damaged enemy)", target.ID, enemy.ID)
	}

	// Test 2: Target persistence - bot1 should have a lock after selecting
	if bot1.BotTargetLockTime <= 0 {
		t.Errorf("Bot1 has no target lock time after selecting target")
	}

	// Test 3: Team coordination - bot2 should also select the same target
	target2 := server.SelectBestCombatTarget(bot2)
	if target2 == nil {
		t.Errorf("Bot2 selectBestCombatTarget returned nil, expected damaged enemy")
	} else if target2.ID != enemy.ID {
		t.Errorf("Bot2 selectBestCombatTarget chose player %d, expected %d", target2.ID, enemy.ID)
	}

	// Test 4: Target scoring prioritizes damaged enemies
	score := server.CalculateTargetScore(bot1, enemy, 3000)
	if score <= 0 {
		t.Errorf("Target score for damaged enemy should be positive, got %f", score)
	}

	// Verify damage bonus is applied
	undamagedEnemy := gs.Players[3]
	undamagedEnemy.Status = game.StatusAlive
	undamagedEnemy.Team = game.TeamRom
	undamagedEnemy.Ship = game.ShipCruiser
	undamagedEnemy.Connected = true
	undamagedEnemy.X, undamagedEnemy.Y = 53000, 50000 // Same distance
	undamagedEnemy.Damage = 0

	damagedScore := server.CalculateTargetScore(bot1, enemy, 3000)
	undamagedScore := server.CalculateTargetScore(bot1, undamagedEnemy, 3000)
	if damagedScore <= undamagedScore {
		t.Errorf("Damaged enemy score (%f) should exceed undamaged score (%f)", damagedScore, undamagedScore)
	}
}

func TestBotTargetPersistence(t *testing.T) {
	// Create minimal game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	bot := gs.Players[0]
	enemy1 := gs.Players[1]
	enemy2 := gs.Players[2]

	// Configure bot
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.Ship = game.ShipCruiser
	bot.IsBot = true
	bot.Connected = true
	bot.Fuel = 10000

	// Configure enemies
	enemy1.Status = game.StatusAlive
	enemy1.Team = game.TeamRom
	enemy1.Ship = game.ShipDestroyer
	enemy1.Connected = true

	enemy2.Status = game.StatusAlive
	enemy2.Team = game.TeamRom
	enemy2.Ship = game.ShipScout
	enemy2.Connected = true

	// Position enemies at different distances
	bot.X, bot.Y = 50000, 50000
	enemy1.X, enemy1.Y = 55000, 50000 // Closer
	enemy2.X, enemy2.Y = 58000, 50000 // Farther

	// Let bot acquire initial target via direct call
	target := server.SelectBestCombatTarget(bot)
	if target == nil {
		t.Fatal("Bot failed to acquire initial target")
	}
	initialTarget := bot.BotTarget
	initialLockTime := bot.BotTargetLockTime

	if initialLockTime <= 0 {
		t.Errorf("Bot should have positive lock time after target acquisition, got %d", initialLockTime)
	}

	// Move the other enemy slightly closer (but not dramatically better)
	if initialTarget == enemy1.ID {
		enemy2.X = 54000 // Make enemy2 slightly closer
	} else {
		enemy1.X = 54000 // Make enemy1 slightly closer
	}

	// Re-select target — persistence should prevent switching for marginal improvement
	target2 := server.SelectBestCombatTarget(bot)
	if target2 == nil {
		t.Fatal("Bot lost target on re-selection")
	}

	if bot.BotTarget != initialTarget {
		t.Errorf("Bot switched targets from %d to %d despite having lock (lock time: %d); persistence should prevent switching for marginal improvement",
			initialTarget, bot.BotTarget, bot.BotTargetLockTime)
	}
}
