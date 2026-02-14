package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestStarbaseTorpedoFiringRegardlessOfFacing(t *testing.T) {
	// Set deterministic seed for reproducible tests

	// Create minimal game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Create a starbase at position (50000, 50000) facing north (0 radians)
	starbase := gs.Players[0]
	starbase.Status = game.StatusAlive
	starbase.Team = game.TeamFed
	starbase.Ship = game.ShipStarbase
	starbase.IsBot = true
	starbase.Connected = true
	starbase.X = 50000
	starbase.Y = 50000
	starbase.Dir = 0      // Facing north
	starbase.DesDir = 0   // Desired direction north
	starbase.Fuel = 30000 // Give starbase sufficient fuel
	starbase.NumTorps = 0 // No torpedoes initially
	starbase.WTemp = 0    // Cool weapon temperature
	starbase.Shields_up = true
	starbase.BotDefenseTarget = -1

	// Create an enemy ship behind the starbase (south of it)
	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipDestroyer
	enemy.IsBot = false
	enemy.Connected = true
	enemy.X = 50000 // Same X coordinate
	enemy.Y = 46000 // 4000 units south of starbase (behind it)
	enemy.Speed = 3
	enemy.Dir = math.Pi // Moving north toward starbase
	enemy.Armies = 0

	// Calculate distance
	enemyDist := game.Distance(starbase.X, starbase.Y, enemy.X, enemy.Y)
	t.Logf("Initial setup: starbase at (%.0f, %.0f) facing %.2f radians, enemy at (%.0f, %.0f) distance %.0f",
		starbase.X, starbase.Y, starbase.Dir, enemy.X, enemy.Y, enemyDist)

	// Test starbaseDefensiveCombat function directly
	initialTorpCount := len(gs.Torps)
	server.starbaseDefensiveCombat(starbase, enemy, enemyDist)

	// Verify that torpedo was fired
	finalTorpCount := len(gs.Torps)
	if finalTorpCount <= initialTorpCount {
		t.Errorf("Expected starbase to fire torpedo. Initial torps: %d, Final torps: %d",
			initialTorpCount, finalTorpCount)
		t.Logf("Starbase stats: Dir=%.2f, NumTorps=%d, Fuel=%d, WTemp=%d",
			starbase.Dir, starbase.NumTorps, starbase.Fuel, starbase.WTemp)
	} else {
		t.Logf("SUCCESS: Starbase fired torpedo despite facing away from enemy. Torps: %d -> %d",
			initialTorpCount, finalTorpCount)
	}

	// Verify torpedo properties
	if len(gs.Torps) > 0 {
		torp := gs.Torps[len(gs.Torps)-1] // Get the last torpedo
		if torp.Owner != starbase.ID {
			t.Errorf("Expected torpedo owner to be starbase ID %d, got %d", starbase.ID, torp.Owner)
		}
		if torp.Team != starbase.Team {
			t.Errorf("Expected torpedo team to be %d, got %d", starbase.Team, torp.Team)
		}
		t.Logf("Torpedo fired: Owner=%d, Team=%d, Dir=%.2f, Speed=%.0f",
			torp.Owner, torp.Team, torp.Dir, torp.Speed)
	}
}

func TestStarbaseDefenseWeaponLogicWithCloseEnemy(t *testing.T) {
	// Set deterministic seed for reproducible tests

	// Create minimal game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Create a starbase
	starbase := gs.Players[0]
	starbase.Status = game.StatusAlive
	starbase.Team = game.TeamFed
	starbase.Ship = game.ShipStarbase
	starbase.IsBot = true
	starbase.Connected = true
	starbase.X = 50000
	starbase.Y = 50000
	starbase.Fuel = 30000
	starbase.NumTorps = 0
	starbase.WTemp = 0

	// Create an enemy ship very close to the starbase (within StarbaseTorpRange)
	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipScout
	enemy.X = 52000 // 2000 units away (well within StarbaseTorpRange of 8400)
	enemy.Y = 52000
	enemy.Speed = 5
	enemy.Armies = 2 // Carrying armies to make it a high-priority target

	enemyDist := game.Distance(starbase.X, starbase.Y, enemy.X, enemy.Y)
	t.Logf("Close enemy test: distance=%.0f, StarbaseTorpRange=%d", enemyDist, game.StarbaseTorpRange)

	// Test starbaseDefenseWeaponLogic function directly
	initialTorpCount := len(gs.Torps)
	server.starbaseDefenseWeaponLogic(starbase, enemy, enemyDist)

	// Verify that torpedo was fired by the close-range fallback
	finalTorpCount := len(gs.Torps)
	if finalTorpCount <= initialTorpCount {
		t.Errorf("Expected starbase to fire torpedo with close-range fallback. Initial torps: %d, Final torps: %d",
			initialTorpCount, finalTorpCount)
	} else {
		t.Logf("SUCCESS: Starbase used close-range fallback to fire torpedo. Torps: %d -> %d",
			initialTorpCount, finalTorpCount)
	}
}

func TestStarbaseFiresAtMovingEnemyBehind(t *testing.T) {
	// This test simulates a more realistic scenario where an enemy is orbiting behind the starbase

	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Starbase facing east (π/2)
	starbase := gs.Players[0]
	starbase.Status = game.StatusAlive
	starbase.Team = game.TeamFed
	starbase.Ship = game.ShipStarbase
	starbase.IsBot = true
	starbase.Connected = true
	starbase.X = 50000
	starbase.Y = 50000
	starbase.Dir = math.Pi / 2 // Facing east
	starbase.DesDir = math.Pi / 2
	starbase.Fuel = 30000
	starbase.NumTorps = 0
	starbase.WTemp = 0

	// Enemy circling behind starbase (west side)
	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamKli
	enemy.Ship = game.ShipCruiser
	enemy.X = 46000 // 4000 units west of starbase (behind from its perspective)
	enemy.Y = 50000 // Same Y
	enemy.Speed = 8 // Fast moving
	enemy.Dir = 0   // Moving east toward starbase (cos(0)=1 → positive X)
	enemy.Armies = 5 // High-value target

	enemyDist := game.Distance(starbase.X, starbase.Y, enemy.X, enemy.Y)

	// The angle from starbase to enemy
	angleToEnemy := math.Atan2(enemy.Y-starbase.Y, enemy.X-starbase.X)
	angleDiff := math.Abs(starbase.Dir - angleToEnemy)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	t.Logf("Starbase facing %.2f rad, enemy at angle %.2f rad, angle difference %.2f rad (%.1f degrees)",
		starbase.Dir, angleToEnemy, angleDiff, angleDiff*180/math.Pi)
	t.Logf("Enemy distance: %.0f, StarbaseTorpRange: %d", enemyDist, game.StarbaseTorpRange)

	// Run multiple update cycles to simulate the game loop
	torpedoFired := false
	for i := 0; i < 10; i++ {
		initialTorpCount := len(gs.Torps)

		// Simulate the starbase bot AI detecting this as a threat
		server.starbaseDefensiveCombat(starbase, enemy, enemyDist)

		finalTorpCount := len(gs.Torps)
		if finalTorpCount > initialTorpCount {
			torpedoFired = true
			t.Logf("Torpedo fired on iteration %d", i+1)
			break
		}

		// Reduce cooldown
		if starbase.BotCooldown > 0 {
			starbase.BotCooldown--
		}
	}

	if !torpedoFired {
		t.Errorf("Starbase should have fired torpedo at enemy behind it within 10 iterations")
		t.Logf("Final starbase state: Dir=%.2f, NumTorps=%d, Fuel=%d, WTemp=%d, BotCooldown=%d",
			starbase.Dir, starbase.NumTorps, starbase.Fuel, starbase.WTemp, starbase.BotCooldown)
	} else {
		t.Logf("SUCCESS: Starbase successfully fired at enemy behind it")
	}
}
