package server

import (
	"testing"
	"time"

	"github.com/lab1702/netrek-web/game"
)

func TestVictoryConditions(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Helper function to set up a basic game state
	setupGameState := func() {
		server.gameState.Frame = 200      // Ensure frame count is high enough
		server.gameState.GameOver = false // Reset game over flag
		server.gameState.Winner = 0
		server.gameState.WinType = ""
		// Clear all players first
		for i := range server.gameState.Players {
			server.gameState.Players[i] = &game.Player{
				ID:     i,
				Status: game.StatusFree,
				Team:   0,
			}
		}
		// Reset all planets to neutral
		for i := range server.gameState.Planets {
			server.gameState.Planets[i].Owner = game.TeamNone
		}
	}

	t.Run("NoVictoryWithOnlyOneTeam", func(t *testing.T) {
		setupGameState()

		// Add only Federation players
		server.gameState.Players[0] = &game.Player{
			ID:     0,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
		}
		server.gameState.Players[1] = &game.Player{
			ID:     1,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
		}

		server.checkVictoryConditions()

		if server.gameState.GameOver {
			t.Error("Game should not be over with only one team playing")
		}
	})

	t.Run("GenocideVictory", func(t *testing.T) {
		setupGameState()

		// Set up scenario where Federation eliminates Romulans
		// First mark that both teams played by having dead Romans
		server.gameState.Players[0] = &game.Player{
			ID:     0,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
		}
		server.gameState.Players[1] = &game.Player{
			ID:     1,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
		}
		// Key: Former Romulan player who died (status != StatusFree shows they played)
		server.gameState.Players[2] = &game.Player{
			ID:     2,
			Status: game.StatusDead, // Dead Romulan (shows they played)
			Team:   game.TeamRom,
		}

		server.checkVictoryConditions()

		if !server.gameState.GameOver {
			t.Error("Expected genocide victory")
		}
		if server.gameState.WinType != "genocide" {
			t.Errorf("Expected genocide victory, got %s", server.gameState.WinType)
		}
		if server.gameState.Winner != game.TeamFed {
			t.Errorf("Expected Federation to win, got %d", server.gameState.Winner)
		}
	})

	t.Run("ConquestVictory", func(t *testing.T) {
		setupGameState()

		// Set up scenario with multiple teams alive so genocide doesn't trigger
		server.gameState.Players[0] = &game.Player{
			ID:     0,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
		}
		server.gameState.Players[1] = &game.Player{
			ID:     1,
			Status: game.StatusAlive,
			Team:   game.TeamRom, // Both teams have alive players
		}

		// Federation owns all planets (conquest condition)
		for i := range server.gameState.Planets {
			server.gameState.Planets[i].Owner = game.TeamFed
		}

		server.checkVictoryConditions()

		if !server.gameState.GameOver {
			t.Error("Expected conquest victory")
		}
		if server.gameState.WinType != "conquest" {
			t.Errorf("Expected conquest victory, got %s", server.gameState.WinType)
		}
		if server.gameState.Winner != game.TeamFed {
			t.Errorf("Expected Federation to win, got %d", server.gameState.Winner)
		}
	})

	t.Run("DominationVictory", func(t *testing.T) {
		setupGameState()

		// Set up scenario where Federation owns some planets, others are independent
		// and enemies have no armies (both teams alive to prevent genocide)
		server.gameState.Players[0] = &game.Player{
			ID:     0,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
			Armies: 0,
		}
		server.gameState.Players[1] = &game.Player{
			ID:     1,
			Status: game.StatusAlive, // Both teams alive (prevents genocide)
			Team:   game.TeamRom,
			Armies: 0, // No armies to retake planets
		}

		// Critical: Federation owns SOME planets, rest are independent
		// This prevents conquest victory (which requires ALL planets)
		server.gameState.Planets[0].Owner = game.TeamFed
		server.gameState.Planets[1].Owner = game.TeamFed
		// Make sure not all planets are owned by Fed (to avoid conquest)
		for i := 2; i < len(server.gameState.Planets); i++ {
			server.gameState.Planets[i].Owner = game.TeamNone // Independent
		}

		server.checkVictoryConditions()

		if !server.gameState.GameOver {
			t.Error("Expected domination victory")
		}
		if server.gameState.WinType != "domination" {
			t.Errorf("Expected domination victory, got %s", server.gameState.WinType)
		}
		if server.gameState.Winner != game.TeamFed {
			t.Errorf("Expected Federation to win, got %d", server.gameState.Winner)
		}
	})

	t.Run("NoDominationVictoryWhenEnemyHasArmies", func(t *testing.T) {
		setupGameState()

		// Similar to domination test but enemy has armies (both alive to prevent genocide)
		server.gameState.Players[0] = &game.Player{
			ID:     0,
			Status: game.StatusAlive,
			Team:   game.TeamFed,
			Armies: 0,
		}
		server.gameState.Players[1] = &game.Player{
			ID:     1,
			Status: game.StatusAlive, // Both teams alive (prevents genocide)
			Team:   game.TeamRom,
			Armies: 5, // Has armies to potentially retake planets
		}

		// Federation owns some planets, rest are independent
		// Same setup as domination test but enemy has armies
		server.gameState.Planets[0].Owner = game.TeamFed
		server.gameState.Planets[1].Owner = game.TeamFed
		for i := 2; i < len(server.gameState.Planets); i++ {
			server.gameState.Planets[i].Owner = game.TeamNone
		}

		server.checkVictoryConditions()

		if server.gameState.GameOver {
			t.Error("Game should not be over when enemy has armies")
		}
	})
}

func TestAnnounceVictory(t *testing.T) {
	server := NewServer()

	// Create a buffered channel to prevent blocking
	server.broadcast = make(chan ServerMessage, 10)

	// Set up a Federation victory
	server.gameState.GameOver = true
	server.gameState.Winner = game.TeamFed
	server.gameState.WinType = "conquest"

	// Call announceVictory but don't wait for the goroutine
	server.announceVictory()

	// Check that a message was broadcast
	select {
	case msg := <-server.broadcast:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}

		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Error("Expected message data to be a map")
		}

		if data["type"] != "victory" {
			t.Errorf("Expected victory message, got %s", data["type"])
		}

		if data["winner"] != game.TeamFed {
			t.Errorf("Expected Federation winner, got %v", data["winner"])
		}

	case <-time.After(100 * time.Millisecond):
		t.Error("Expected a broadcast message but didn't receive one")
	}
}

func TestResetGame(t *testing.T) {
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10)

	// Set up some players
	server.gameState.Players[0] = &game.Player{
		ID:        0,
		Name:      "TestPlayer",
		Team:      game.TeamFed,
		Ship:      game.ShipDestroyer,
		Status:    game.StatusAlive,
		Connected: true,
		IsBot:     false,
	}
	server.gameState.Players[1] = &game.Player{
		ID:        1,
		Name:      "BotPlayer",
		Team:      game.TeamRom,
		Ship:      game.ShipCruiser,
		Status:    game.StatusAlive,
		Connected: true,
		IsBot:     true,
	}

	// Reset the game
	server.resetGame()

	// Check that human player is preserved but moved to outfit status
	if server.gameState.Players[0].Status != game.StatusOutfit {
		t.Errorf("Expected human player to be in outfit status, got %d", server.gameState.Players[0].Status)
	}
	if !server.gameState.Players[0].Connected {
		t.Error("Expected human player to remain connected")
	}

	// Check that bot is disconnected
	if server.gameState.Players[1].Connected {
		t.Error("Expected bot to be disconnected")
	}
	if server.gameState.Players[1].Status != game.StatusFree {
		t.Errorf("Expected bot to be free, got %d", server.gameState.Players[1].Status)
	}

	// Check that a reset message was broadcast
	select {
	case msg := <-server.broadcast:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected a broadcast message but didn't receive one")
	}
}
