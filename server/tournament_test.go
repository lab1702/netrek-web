package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestTournamentMode(t *testing.T) {
	// Create a test server
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10) // Buffered to prevent blocking

	// Helper function to set up a basic game state
	setupGameState := func() {
		server.gameState.Frame = 200 // Ensure frame count is high enough
		server.gameState.T_mode = false
		server.gameState.T_start = 0
		server.gameState.T_remain = 0
		server.gameState.GameOver = false
		server.gameState.Winner = 0
		server.gameState.WinType = ""
		// Clear all players first
		for i := range server.gameState.Players {
			server.gameState.Players[i] = &game.Player{
				ID:        i,
				Status:    game.StatusFree,
				Team:      0,
				Connected: false,
			}
		}
		// Initialize tournament stats if it's nil
		if server.gameState.TournamentStats == nil {
			server.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)
		}
	}

	t.Run("NoTournamentWithInsufficientPlayers", func(t *testing.T) {
		setupGameState()

		// Add only 3 Federation players (not enough for tournament)
		for i := 0; i < 3; i++ {
			server.gameState.Players[i] = &game.Player{
				ID:        i,
				Status:    game.StatusAlive,
				Team:      game.TeamFed,
				Connected: true,
			}
		}

		server.checkTournamentMode()

		if server.gameState.T_mode {
			t.Error("Tournament mode should not activate with insufficient players")
		}
	})

	t.Run("NoTournamentWithOnlyOneTeam", func(t *testing.T) {
		setupGameState()

		// Add 8 Federation players (enough players but only one team)
		for i := 0; i < 8; i++ {
			server.gameState.Players[i] = &game.Player{
				ID:        i,
				Status:    game.StatusAlive,
				Team:      game.TeamFed,
				Connected: true,
			}
		}

		server.checkTournamentMode()

		if server.gameState.T_mode {
			t.Error("Tournament mode should not activate with only one team")
		}
	})

	t.Run("TournamentActivatesWithSufficientPlayers", func(t *testing.T) {
		setupGameState()

		// Add 4 Federation players and 4 Romulan players
		for i := 0; i < 4; i++ {
			server.gameState.Players[i] = &game.Player{
				ID:        i,
				Status:    game.StatusAlive,
				Team:      game.TeamFed,
				Ship:      game.ShipDestroyer,
				Connected: true,
				Kills:     5.0, // Give some initial stats to test reset
				Deaths:    2,
				Damage:    50,
				X:         12345,
				Y:         67890,
			}
		}
		for i := 4; i < 8; i++ {
			server.gameState.Players[i] = &game.Player{
				ID:        i,
				Status:    game.StatusAlive,
				Team:      game.TeamRom,
				Ship:      game.ShipCruiser,
				Connected: true,
				Kills:     3.0,
				Deaths:    1,
				Damage:    25,
				X:         98765,
				Y:         43210,
			}
		}

		server.checkTournamentMode()

		if !server.gameState.T_mode {
			t.Error("Tournament mode should activate with 4v4 players")
		}

		if server.gameState.T_remain != 1800 {
			t.Errorf("Expected 1800 seconds remaining, got %d", server.gameState.T_remain)
		}

		if server.gameState.T_start != server.gameState.Frame {
			t.Errorf("Expected T_start to be current frame %d, got %d", server.gameState.Frame, server.gameState.T_start)
		}

		// Check that all players were reset
		for i := 0; i < 8; i++ {
			p := server.gameState.Players[i]
			if p.Kills != 0 {
				t.Errorf("Player %d kills should be reset to 0, got %f", i, p.Kills)
			}
			if p.Deaths != 0 {
				t.Errorf("Player %d deaths should be reset to 0, got %d", i, p.Deaths)
			}
			if p.Damage != 0 {
				t.Errorf("Player %d damage should be reset to 0, got %d", i, p.Damage)
			}
			// Check that position was reset near home world
			expectedHomeX := float64(game.TeamHomeX[p.Team])
			expectedHomeY := float64(game.TeamHomeY[p.Team])
			if p.X < expectedHomeX-5000 || p.X > expectedHomeX+5000 {
				t.Errorf("Player %d X position %f not near home %f", i, p.X, expectedHomeX)
			}
			if p.Y < expectedHomeY-5000 || p.Y > expectedHomeY+5000 {
				t.Errorf("Player %d Y position %f not near home %f", i, p.Y, expectedHomeY)
			}
		}

		// Check that activation message was broadcast
		select {
		case msg := <-server.broadcast:
			if msg.Type != MsgTypeMessage {
				t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
			}
			data, ok := msg.Data.(map[string]interface{})
			if !ok {
				t.Error("Expected message data to be a map")
			}
			if data["type"] != "info" {
				t.Errorf("Expected info message, got %s", data["type"])
			}
		default:
			t.Error("Expected tournament activation broadcast message")
		}
	})

	t.Run("TournamentDeactivatesWithInsufficientPlayers", func(t *testing.T) {
		setupGameState()

		// Start with tournament mode active
		server.gameState.T_mode = true

		// Add insufficient players (only 2 total)
		server.gameState.Players[0] = &game.Player{
			ID:        0,
			Status:    game.StatusAlive,
			Team:      game.TeamFed,
			Connected: true,
		}
		server.gameState.Players[1] = &game.Player{
			ID:        1,
			Status:    game.StatusAlive,
			Team:      game.TeamRom,
			Connected: true,
		}

		server.checkTournamentMode()

		if server.gameState.T_mode {
			t.Error("Tournament mode should deactivate with insufficient players")
		}

		// Check that deactivation message was broadcast
		select {
		case msg := <-server.broadcast:
			if msg.Type != MsgTypeMessage {
				t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
			}
			data, ok := msg.Data.(map[string]interface{})
			if !ok {
				t.Error("Expected message data to be a map")
			}
			if data["type"] != "info" {
				t.Errorf("Expected info message, got %s", data["type"])
			}
		default:
			t.Error("Expected tournament deactivation broadcast message")
		}
	})

	t.Run("TournamentNoTimeoutIfGameAlreadyOver", func(t *testing.T) {
		setupGameState()

		// Set up tournament that's timed out but game is already over
		server.gameState.T_mode = true
		server.gameState.T_start = 0
		server.gameState.Frame = 18010 // More than 30 minutes elapsed
		server.gameState.GameOver = true
		server.gameState.Winner = game.TeamRom
		server.gameState.WinType = "conquest"

		originalWinner := server.gameState.Winner
		originalWinType := server.gameState.WinType

		server.checkTournamentMode()

		// Should not change existing victory
		if server.gameState.Winner != originalWinner {
			t.Errorf("Winner should not change when game already over")
		}
		if server.gameState.WinType != originalWinType {
			t.Errorf("Win type should not change when game already over")
		}
	})
}

func TestTournamentPlayerReset(t *testing.T) {
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10)

	// Initialize tournament stats
	server.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)

	// Set up players to trigger tournament - need 4 per team
	for i := 0; i < 4; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamFed,
			Ship:      game.ShipDestroyer,
			Connected: true,
			Kills:     10.0,
			Deaths:    5,
			Damage:    50,
			X:         50000,
			Y:         60000,
		}
	}
	for i := 4; i < 8; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamRom,
			Ship:      game.ShipCruiser,
			Connected: true,
			Kills:     5.0,
			Deaths:    2,
			Damage:    25,
		}
	}

	server.gameState.Frame = 100
	server.checkTournamentMode()

	// Tournament should be activated
	if !server.gameState.T_mode {
		t.Error("Tournament mode should be activated with 4v4")
	}

	// Check that first few players were reset
	for i := 0; i < 2; i++ {
		p := server.gameState.Players[i]
		if p.Kills != 0 {
			t.Errorf("Player %d kills should be reset to 0, got %f", i, p.Kills)
		}
		if p.Deaths != 0 {
			t.Errorf("Player %d deaths should be reset to 0, got %d", i, p.Deaths)
		}
		if p.Damage != 0 {
			t.Errorf("Player %d damage should be reset to 0, got %d", i, p.Damage)
		}
		if p.Speed != 0 {
			t.Errorf("Player %d speed should be reset to 0, got %f", i, p.Speed)
		}
		if p.DesSpeed != 0 {
			t.Errorf("Player %d desSpeed should be reset to 0, got %f", i, p.DesSpeed)
		}
		// Check position was reset to near home
		expectedHomeX := float64(game.TeamHomeX[p.Team])
		expectedHomeY := float64(game.TeamHomeY[p.Team])
		if p.X < expectedHomeX-5000 || p.X > expectedHomeX+5000 {
			t.Errorf("Player %d X position %f not near home %f", i, p.X, expectedHomeX)
		}
		if p.Y < expectedHomeY-5000 || p.Y > expectedHomeY+5000 {
			t.Errorf("Player %d Y position %f not near home %f", i, p.Y, expectedHomeY)
		}
		// Check ship stats were reset
		shipStats := game.ShipData[p.Ship]
		if p.Shields != shipStats.MaxShields {
			t.Errorf("Player %d shields should be %d, got %d", i, shipStats.MaxShields, p.Shields)
		}
		if p.Fuel != shipStats.MaxFuel {
			t.Errorf("Player %d fuel should be %d, got %d", i, shipStats.MaxFuel, p.Fuel)
		}
	}
}
