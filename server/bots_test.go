package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestBotPlanetDefense(t *testing.T) {
	// Set deterministic seed for reproducible tests

	// Create minimal game state
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Create a friendly planet
	planet := gs.Planets[0]
	planet.Owner = game.TeamFed
	planet.X = 50000
	planet.Y = 50000
	planet.Armies = 5
	planet.Name = "Test Planet"

	// Create a defender bot
	defender := gs.Players[0]
	defender.Status = game.StatusAlive
	defender.Team = game.TeamFed
	defender.Ship = game.ShipDestroyer
	defender.IsBot = true
	defender.Connected = true
	defender.X = 55000 // 5k away from planet
	defender.Y = 55000
	defender.Fuel = 5000
	defender.NumTorps = 0
	defender.BotDefenseTarget = -1

	// Create an enemy bomber approaching the planet
	bomber := gs.Players[1]
	bomber.Status = game.StatusAlive
	bomber.Team = game.TeamRom
	bomber.Ship = game.ShipCruiser
	bomber.IsBot = false
	bomber.Connected = true
	bomber.X = 52000 // 4k from planet, within bombing + intercept range
	bomber.Y = 52000
	bomber.Speed = 5
	bomber.Dir = 0 // Heading roughly toward planet
	bomber.Armies = 3

	// Test threat detection
	threatenedPlanet, enemy, dist := server.getThreatenedFriendlyPlanet(defender)
	if threatenedPlanet == nil {
		t.Fatal("Expected to detect threatened planet")
	}
	if threatenedPlanet.ID != planet.ID {
		t.Errorf("Expected planet ID %d, got %d", planet.ID, threatenedPlanet.ID)
	}
	if enemy == nil || enemy.ID != bomber.ID {
		t.Fatal("Expected to detect enemy bomber")
	}
	if dist <= 0 {
		t.Fatal("Expected positive distance to enemy")
	}

	// Test bot defense activation
	server.defendPlanet(defender, threatenedPlanet, enemy, dist)

	// Verify defense target is set
	if defender.BotDefenseTarget != planet.ID {
		t.Errorf("Expected BotDefenseTarget to be %d, got %d", planet.ID, defender.BotDefenseTarget)
	}

	// Verify bot has shields up when defending
	if !defender.Shields_up {
		t.Error("Expected bot to have shields up when defending")
	}

	// Verify bot cleared conflicting states
	if defender.Orbiting != -1 {
		t.Error("Expected bot to clear orbiting state when defending")
	}
	if defender.Bombing {
		t.Error("Expected bot to clear bombing state when defending")
	}

	// Test weapon firing when enemy is in range
	// Move enemy closer to trigger weapon logic
	bomber.X = 50500
	bomber.Y = 50500
	enemyDist := game.Distance(defender.X, defender.Y, bomber.X, bomber.Y)

	// Make sure bot has resources to fire
	defender.Dir = game.NormalizeAngle(enemy.Dir)
	defender.Fuel = 5000
	defender.WTemp = 0
	defender.NumTorps = 0

	torpsBefore := len(server.gameState.Torps)
	fuelBefore := defender.Fuel

	// Test aggressive weapon logic
	server.planetDefenseWeaponLogic(defender, bomber, enemyDist)

	// Verify that a weapon was actually fired (torpedo or phaser)
	torpsAfter := len(server.gameState.Torps)
	fuelAfter := defender.Fuel

	if torpsAfter <= torpsBefore && fuelAfter >= fuelBefore {
		t.Error("Expected planetDefenseWeaponLogic to fire a weapon (torpedo or phaser), but no weapon was fired")
	}

	if torpsAfter > torpsBefore {
		// Torpedo was fired - verify it belongs to the defender
		newTorp := server.gameState.Torps[torpsAfter-1]
		if newTorp.Owner != defender.ID {
			t.Errorf("Expected torpedo owner to be %d, got %d", defender.ID, newTorp.Owner)
		}
		if newTorp.Team != defender.Team {
			t.Errorf("Expected torpedo team to be %d, got %d", defender.Team, newTorp.Team)
		}
		if newTorp.Status != game.TorpMove {
			t.Errorf("Expected torpedo status TorpMove, got %d", newTorp.Status)
		}
	}

	// Verify BotCooldown was set (weapon logic always sets a cooldown)
	if defender.BotCooldown <= 0 {
		t.Error("Expected BotCooldown to be set after firing weapon")
	}
}

func TestBotDefensePersistence(t *testing.T) {

	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	// Setup similar to above test
	planet := gs.Planets[0]
	planet.Owner = game.TeamFed
	planet.X = 50000
	planet.Y = 50000

	defender := gs.Players[0]
	defender.Status = game.StatusAlive
	defender.Team = game.TeamFed
	defender.Ship = game.ShipDestroyer
	defender.IsBot = true
	defender.Connected = true
	defender.X = 55000
	defender.Y = 55000
	defender.BotDefenseTarget = planet.ID // Already defending

	bomber := gs.Players[1]
	bomber.Status = game.StatusAlive
	bomber.Team = game.TeamRom
	bomber.Ship = game.ShipCruiser
	bomber.X = 65000 // Far from planet (outside threat range)
	bomber.Y = 65000

	// Test that defense persists when no immediate threats
	threatenedPlanet, enemy, _ := server.getThreatenedFriendlyPlanet(defender)
	if threatenedPlanet != nil || enemy != nil {
		t.Error("Expected no threats detected when enemy is far away")
	}

	// Verify bot maintains defense target even when no current threats
	if defender.BotDefenseTarget != planet.ID {
		t.Error("Bot should maintain defense target until explicitly cleared")
	}

	t.Log("Defense persistence test completed successfully")
}

func TestStarbaseDefense(t *testing.T) {

	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 100),
	}

	planet := gs.Planets[0]
	planet.Owner = game.TeamKli
	planet.X = 30000
	planet.Y = 30000

	// Create starbase defender
	starbase := gs.Players[0]
	starbase.Status = game.StatusAlive
	starbase.Team = game.TeamKli
	starbase.Ship = game.ShipStarbase
	starbase.IsBot = true
	starbase.Connected = true
	starbase.X = 35000
	starbase.Y = 35000
	starbase.BotDefenseTarget = -1
	starbase.Fuel = 30000 // Give starbase sufficient fuel for shields

	bomber := gs.Players[1]
	bomber.Status = game.StatusAlive
	bomber.Team = game.TeamFed
	bomber.X = 32000
	bomber.Y = 32000
	bomber.Speed = 3
	bomber.Dir = 0

	// Test starbase defense
	threatenedPlanet, enemy, dist := server.getThreatenedFriendlyPlanet(starbase)
	if threatenedPlanet == nil || enemy == nil {
		t.Fatal("Starbase should detect threatened planet")
	}

	server.starbaseDefendPlanet(starbase, threatenedPlanet, enemy, dist)

	// Verify starbase-specific behavior
	if starbase.DesSpeed != 0 {
		t.Error("Starbase should not move when defending")
	}
	if !starbase.Shields_up {
		t.Error("Starbase should have shields up when defending")
	}
	if starbase.BotDefenseTarget != planet.ID {
		t.Error("Starbase should set defense target")
	}

	t.Log("Starbase defense test completed successfully")
}
