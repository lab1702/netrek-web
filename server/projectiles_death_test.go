package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestTorpedoSurvivesOwnerExplosion(t *testing.T) {
	// Create a test server
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Set up player A (shooter)
	playerA := server.gameState.Players[0]
	playerA.Status = game.StatusAlive
	playerA.Ship = game.ShipCruiser
	playerA.X = 10000
	playerA.Y = 10000
	playerA.Fuel = 5000
	playerA.NumTorps = 0

	// Set up player B (target)
	playerB := server.gameState.Players[1]
	playerB.Status = game.StatusAlive
	playerB.Ship = game.ShipCruiser
	playerB.X = 15000 // 5000 units away
	playerB.Y = 10000
	playerB.Damage = 0
	playerB.Team = game.TeamKli // Different team to allow hits

	// Player A fires a torpedo
	shipStats := game.ShipData[playerA.Ship]
	torp := &game.Torpedo{
		ID:     0,
		Owner:  0, // Player A
		X:      playerA.X,
		Y:      playerA.Y,
		Dir:    0, // Firing east toward player B
		Speed:  float64(shipStats.TorpSpeed * 20),
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: 1, // Moving
		Team:   playerA.Team,
	}
	server.gameState.Torps = append(server.gameState.Torps, torp)
	playerA.NumTorps = 1

	// Verify initial state
	if len(server.gameState.Torps) != 1 {
		t.Fatalf("Expected 1 torpedo, got %d", len(server.gameState.Torps))
	}
	if playerA.NumTorps != 1 {
		t.Fatalf("Expected player A to have 1 torpedo, got %d", playerA.NumTorps)
	}

	// Player A explodes (killed by something else)
	playerA.Status = game.StatusExplode
	playerA.ExplodeTimer = 10
	playerA.WhyDead = game.KillTorp // Killed by enemy torpedo

	// Simulate one game update tick to handle explosion
	server.updateGame()

	// Verify torpedo still exists after owner explosion
	if len(server.gameState.Torps) != 1 {
		t.Errorf("Expected torpedo to survive owner explosion, but torpedo count is %d", len(server.gameState.Torps))
	}

	// Verify torpedo is still moving
	originalX := torp.X
	server.updateProjectiles()
	if torp.X <= originalX {
		t.Errorf("Expected torpedo to keep moving after owner explosion")
	}

	// Continue simulating until player A transitions to dead state
	for playerA.Status == game.StatusExplode {
		server.updateGame()
	}

	// Player A should now be dead, and counters should be reset
	if playerA.Status != game.StatusDead {
		t.Errorf("Expected player A to be dead, got status %d", playerA.Status)
	}
	if playerA.NumTorps != 0 {
		t.Errorf("Expected player A torpedo count to be reset to 0, got %d", playerA.NumTorps)
	}

	// But torpedo should still exist in the game world
	if len(server.gameState.Torps) != 1 {
		t.Errorf("Expected torpedo to still exist after owner death, but count is %d", len(server.gameState.Torps))
	}

	// Simulate more ticks to see if torpedo can still hit
	initialTargetDamage := playerB.Damage
	for i := 0; i < 50 && len(server.gameState.Torps) > 0; i++ {
		server.updateProjectiles()
		if playerB.Damage > initialTargetDamage {
			t.Logf("Torpedo from dead player successfully hit target after %d ticks", i+1)
			break
		}
	}

	// Verify the torpedo can still cause damage even after owner is dead
	if playerB.Damage <= initialTargetDamage {
		t.Errorf("Expected torpedo from dead player to be able to hit targets")
	}
}

func TestPlasmaSurvivesOwnerExplosion(t *testing.T) {
	// Create a test server
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Set up player A (shooter) - use destroyer which has plasma
	playerA := server.gameState.Players[0]
	playerA.Status = game.StatusAlive
	playerA.Ship = game.ShipDestroyer
	playerA.X = 10000
	playerA.Y = 10000
	playerA.Fuel = 5000
	playerA.NumPlasma = 0
	playerA.Connected = true // Mark as connected
	playerA.IsBot = false    // Human player

	// Set up player B (target)
	playerB := server.gameState.Players[1]
	playerB.Status = game.StatusAlive
	playerB.Ship = game.ShipCruiser
	playerB.X = 20000 // 10000 units away (outside immediate plasma hit range)
	playerB.Y = 10000
	playerB.Damage = 0
	playerB.Team = game.TeamKli // Different team to allow hits
	playerB.Connected = true    // Mark as connected so server doesn't clear projectiles
	playerB.IsBot = false       // Human player

	// Player A fires a plasma
	shipStats := game.ShipData[playerA.Ship]
	plasma := &game.Plasma{
		ID:     0,
		Owner:  0, // Player A
		X:      playerA.X,
		Y:      playerA.Y,
		Dir:    0, // Firing east toward player B
		Speed:  float64(shipStats.PlasmaSpeed * 20),
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse,
		Status: 1, // Moving
		Team:   playerA.Team,
	}
	server.gameState.Plasmas = append(server.gameState.Plasmas, plasma)
	playerA.NumPlasma = 1

	// Verify initial state
	if len(server.gameState.Plasmas) != 1 {
		t.Fatalf("Expected 1 plasma, got %d", len(server.gameState.Plasmas))
	}
	if playerA.NumPlasma != 1 {
		t.Fatalf("Expected player A to have 1 plasma, got %d", playerA.NumPlasma)
	}

	// Player A explodes
	playerA.Status = game.StatusExplode
	playerA.ExplodeTimer = 10
	playerA.WhyDead = game.KillTorp

	// Simulate one game update tick to handle explosion
	server.updateGame()

	// Verify plasma still exists after owner explosion
	if len(server.gameState.Plasmas) != 1 {
		t.Errorf("Expected plasma to survive owner explosion, but plasma count is %d", len(server.gameState.Plasmas))
	}

	// Continue simulating until player A is dead
	for playerA.Status == game.StatusExplode {
		server.updateGame()
	}

	// Player A should be dead with reset counters
	if playerA.Status != game.StatusDead {
		t.Errorf("Expected player A to be dead, got status %d", playerA.Status)
	}
	if playerA.NumPlasma != 0 {
		t.Errorf("Expected player A plasma count to be reset to 0, got %d", playerA.NumPlasma)
	}

	// But plasma should still exist
	if len(server.gameState.Plasmas) != 1 {
		t.Errorf("Expected plasma to still exist after owner death, but count is %d", len(server.gameState.Plasmas))
	}

	// Simulate more ticks to see if plasma can still hit
	initialTargetDamage := playerB.Damage
	for i := 0; i < 20 && len(server.gameState.Plasmas) > 0; i++ {
		server.updateProjectiles()
		if playerB.Damage > initialTargetDamage {
			t.Logf("Plasma from dead player successfully hit target after %d ticks", i+1)
			break
		}
	}

	// Verify the plasma still exists OR has hit the target
	if playerB.Damage <= initialTargetDamage && len(server.gameState.Plasmas) == 0 {
		// If plasma is gone but didn't hit target, that's an error
		t.Errorf("Expected plasma from dead player to either still exist or have hit target")
	} else if playerB.Damage > initialTargetDamage {
		t.Logf("Plasma from dead player successfully hit target")
	} else if len(server.gameState.Plasmas) > 0 {
		t.Logf("Plasma still exists and is traveling towards target")
	}
}

func TestCounterConsistencyAfterOwnerDeath(t *testing.T) {
	// This test checks for the counter consistency issue
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
		clients:   make(map[int]*Client),
	}

	// Set up ONLY ONE player to avoid tournament mode activation
	// (tournament mode needs 4+ players per team on 2+ teams)
	playerA := server.gameState.Players[0]
	playerA.Status = game.StatusAlive
	playerA.Ship = game.ShipCruiser
	playerA.NumTorps = 0
	playerA.Connected = true // Mark as human player
	playerA.IsBot = false    // Explicitly not a bot
	playerA.Team = game.TeamFed

	// Don't set up any other players to avoid tournament mode

	// Create a torpedo that will expire (low fuse)
	torp := &game.Torpedo{
		ID:     0,
		Owner:  0,
		X:      10000,
		Y:      10000,
		Dir:    0,
		Speed:  240, // 12 * 20
		Damage: 40,
		Fuse:   5, // Will expire in 5 ticks (longer to debug)
		Status: 1,
		Team:   playerA.Team,
	}
	server.gameState.Torps = append(server.gameState.Torps, torp)
	playerA.NumTorps = 1

	t.Logf("Initial state: NumTorps=%d, GameTorps=%d", playerA.NumTorps, len(server.gameState.Torps))

	// Player dies and transitions through explosion to dead
	playerA.Status = game.StatusExplode
	playerA.ExplodeTimer = 10

	// Simulate explosion phase
	for playerA.Status == game.StatusExplode {
		t.Logf("Explosion tick: Status=%d, Timer=%d, NumTorps=%d, GameTorps=%d",
			playerA.Status, playerA.ExplodeTimer, playerA.NumTorps, len(server.gameState.Torps))
		server.updateGame()
	}

	t.Logf("After explosion: Status=%d, NumTorps=%d, GameTorps=%d",
		playerA.Status, playerA.NumTorps, len(server.gameState.Torps))

	// Now player is dead and counters are reset
	if playerA.NumTorps != 0 {
		t.Errorf("Expected NumTorps to be 0 after death, got %d", playerA.NumTorps)
	}

	// Check if torpedo still exists
	t.Logf("Torpedo still exists? %d torpedoes in game", len(server.gameState.Torps))
	if len(server.gameState.Torps) > 0 {
		t.Logf("Remaining torpedo: Fuse=%d, Owner=%d, Status=%d",
			server.gameState.Torps[0].Fuse, server.gameState.Torps[0].Owner, server.gameState.Torps[0].Status)
	}

	// Let torpedo expire naturally (if it still exists)
	for len(server.gameState.Torps) > 0 && server.gameState.Torps[0].Fuse > 0 {
		t.Logf("Torpedo tick: Fuse=%d, NumTorps=%d", server.gameState.Torps[0].Fuse, playerA.NumTorps)
		server.updateProjectiles()
	}

	// Final state
	t.Logf("Final state: NumTorps=%d, GameTorps=%d", playerA.NumTorps, len(server.gameState.Torps))

	// Check if counter went negative (this would be the bug)
	if playerA.NumTorps < 0 {
		t.Errorf("NumTorps went negative after torpedo expiry: %d", playerA.NumTorps)
	}
}

func TestProjectileUpdateAfterOwnerDeath(t *testing.T) {
	// This test directly tests the projectile update system without updateGame()
	server := &Server{
		gameState: game.NewGameState(),
		broadcast: make(chan ServerMessage, 10),
	}

	// Set up player A
	playerA := server.gameState.Players[0]
	playerA.Status = game.StatusAlive
	playerA.Ship = game.ShipCruiser
	playerA.NumTorps = 0

	// Create a torpedo
	torp := &game.Torpedo{
		ID:     0,
		Owner:  0,
		X:      10000,
		Y:      10000,
		Dir:    0,
		Speed:  240,
		Damage: 40,
		Fuse:   40, // Normal fuse
		Status: 1,
		Team:   playerA.Team,
	}
	server.gameState.Torps = append(server.gameState.Torps, torp)
	playerA.NumTorps = 1

	t.Logf("Initial: Player NumTorps=%d, Game Torps=%d, Torpedo Fuse=%d", playerA.NumTorps, len(server.gameState.Torps), torp.Fuse)

	// Player dies (set status directly)
	playerA.Status = game.StatusExplode
	playerA.ExplodeTimer = 10

	// Clear the player's torpedo counter (like the websocket.go code does)
	playerA.NumTorps = 0

	t.Logf("After death: Player NumTorps=%d, Game Torps=%d, Torpedo Fuse=%d", playerA.NumTorps, len(server.gameState.Torps), torp.Fuse)

	// Now call updateProjectiles directly (NOT updateGame)
	server.updateProjectiles()

	t.Logf("After updateProjectiles: Player NumTorps=%d, Game Torps=%d", playerA.NumTorps, len(server.gameState.Torps))
	if len(server.gameState.Torps) > 0 {
		t.Logf("Remaining torpedo: Fuse=%d, Status=%d", server.gameState.Torps[0].Fuse, server.gameState.Torps[0].Status)
	}

	// Verify torpedo still exists and is updating
	if len(server.gameState.Torps) != 1 {
		t.Errorf("Expected torpedo to survive owner death when updateProjectiles called directly, got %d torps", len(server.gameState.Torps))
	}

	// Verify torpedo moved
	if len(server.gameState.Torps) > 0 {
		if server.gameState.Torps[0].X <= 10000 {
			t.Errorf("Expected torpedo to move, but X is still %f", server.gameState.Torps[0].X)
		}
		if server.gameState.Torps[0].Fuse != 39 {
			t.Errorf("Expected torpedo fuse to decrement to 39, got %d", server.gameState.Torps[0].Fuse)
		}
	}

	// Check counter - should NOT go negative since we cleared it before projectile removal
	if playerA.NumTorps < 0 {
		t.Errorf("Player torpedo counter went negative: %d", playerA.NumTorps)
	}
}
