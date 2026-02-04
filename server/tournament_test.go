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

		// Check that activation messages were broadcast (warning before reset, info after)
		// Drain all messages and verify we got both warning and info
		gotWarning := false
		gotInfo := false
		for i := 0; i < 2; i++ {
			select {
			case msg := <-server.broadcast:
				if msg.Type != MsgTypeMessage {
					t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
					continue
				}
				data, ok := msg.Data.(map[string]interface{})
				if !ok {
					t.Error("Expected message data to be a map")
					continue
				}
				switch data["type"] {
				case "warning":
					gotWarning = true
				case "info":
					gotInfo = true
				}
			default:
				// No more messages
			}
		}
		if !gotWarning {
			t.Error("Expected tournament activation warning message before reset")
		}
		if !gotInfo {
			t.Error("Expected tournament activation info message after reset")
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

func TestTournamentTimeoutCoVictory(t *testing.T) {
	// Create a test server
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10)

	// Set up tournament mode with timed out condition
	server.gameState.T_mode = true
	server.gameState.T_start = 0
	server.gameState.Frame = 18010 // More than 30 minutes (1800 * 10 frames/sec = 18000)
	server.gameState.GameOver = false
	server.gameState.Winner = 0
	server.gameState.WinType = ""

	// Add enough active players to maintain tournament mode
	for i := 0; i < 4; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamFed,
			Connected: true,
		}
	}
	for i := 4; i < 8; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamRom,
			Connected: true,
		}
	}

	// Set up team planet counts where Fed and Rom tie with 15 each, Kli has 10
	server.gameState.TeamPlanets[0] = 15 // Federation
	server.gameState.TeamPlanets[1] = 15 // Romulan
	server.gameState.TeamPlanets[2] = 10 // Klingon
	server.gameState.TeamPlanets[3] = 0  // Orion

	server.checkTournamentMode()

	// Game should be over with both Fed and Rom as co-victors
	if !server.gameState.GameOver {
		t.Error("Game should be over after tournament timeout")
	}

	expectedWinner := game.TeamFed | game.TeamRom // Both teams combined
	if server.gameState.Winner != expectedWinner {
		t.Errorf("Expected winner to be Fed|Rom (%d), got %d", expectedWinner, server.gameState.Winner)
	}

	if server.gameState.WinType != "timeout" {
		t.Errorf("Expected win type 'timeout', got '%s'", server.gameState.WinType)
	}

	// Check that victory message was broadcast
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
		// Check that message contains both team names
		messageText, ok := data["text"].(string)
		if !ok {
			t.Error("Expected message text to be a string")
		} else {
			if !contains(messageText, "Federation") {
				t.Errorf("Message should contain 'Federation', got: %s", messageText)
			}
			if !contains(messageText, "Romulan") {
				t.Errorf("Message should contain 'Romulan', got: %s", messageText)
			}
			if !contains(messageText, "share victory") {
				t.Errorf("Message should indicate shared victory, got: %s", messageText)
			}
		}
	default:
		t.Error("Expected victory broadcast message")
	}
}

func TestTournamentTimeoutSingleWinnerUnchanged(t *testing.T) {
	// Create a test server
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10)

	// Set up tournament mode with timed out condition
	server.gameState.T_mode = true
	server.gameState.T_start = 0
	server.gameState.Frame = 18010 // More than 30 minutes
	server.gameState.GameOver = false
	server.gameState.Winner = 0
	server.gameState.WinType = ""

	// Add enough active players to maintain tournament mode
	for i := 0; i < 4; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamFed,
			Connected: true,
		}
	}
	for i := 4; i < 8; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamRom,
			Connected: true,
		}
	}

	// Set up team planet counts where Fed wins with 20, others have less
	server.gameState.TeamPlanets[0] = 20 // Federation (clear winner)
	server.gameState.TeamPlanets[1] = 15 // Romulan
	server.gameState.TeamPlanets[2] = 5  // Klingon
	server.gameState.TeamPlanets[3] = 0  // Orion

	server.checkTournamentMode()

	// Game should be over with only Fed as victor
	if !server.gameState.GameOver {
		t.Error("Game should be over after tournament timeout")
	}

	if server.gameState.Winner != game.TeamFed {
		t.Errorf("Expected winner to be Fed only (%d), got %d", game.TeamFed, server.gameState.Winner)
	}

	if server.gameState.WinType != "timeout" {
		t.Errorf("Expected win type 'timeout', got '%s'", server.gameState.WinType)
	}

	// Check that victory message was broadcast
	select {
	case msg := <-server.broadcast:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Error("Expected message data to be a map")
		}
		// Check that message contains single team name
		messageText, ok := data["text"].(string)
		if !ok {
			t.Error("Expected message text to be a string")
		} else {
			if !contains(messageText, "Federation") {
				t.Errorf("Message should contain 'Federation', got: %s", messageText)
			}
			if contains(messageText, "share") {
				t.Errorf("Message should not indicate shared victory for single winner, got: %s", messageText)
			}
		}
	default:
		t.Error("Expected victory broadcast message")
	}
}

func TestTournamentTimeoutThreeWayTie(t *testing.T) {
	// Create a test server
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 10)

	// Set up tournament mode with timed out condition
	server.gameState.T_mode = true
	server.gameState.T_start = 0
	server.gameState.Frame = 18010
	server.gameState.GameOver = false
	server.gameState.Winner = 0
	server.gameState.WinType = ""

	// Add enough active players to maintain tournament mode
	// Need at least 4 players per team for 3 teams to maintain tournament
	for i := 0; i < 4; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamFed,
			Connected: true,
		}
	}
	for i := 4; i < 8; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamRom,
			Connected: true,
		}
	}
	for i := 8; i < 12; i++ {
		server.gameState.Players[i] = &game.Player{
			ID:        i,
			Status:    game.StatusAlive,
			Team:      game.TeamKli,
			Connected: true,
		}
	}

	// Set up team planet counts where Fed, Rom, and Kli tie with 13 each
	server.gameState.TeamPlanets[0] = 13 // Federation
	server.gameState.TeamPlanets[1] = 13 // Romulan
	server.gameState.TeamPlanets[2] = 13 // Klingon
	server.gameState.TeamPlanets[3] = 1  // Orion

	server.checkTournamentMode()

	// Game should be over with Fed, Rom, and Kli as co-victors
	if !server.gameState.GameOver {
		t.Error("Game should be over after tournament timeout")
	}

	expectedWinner := game.TeamFed | game.TeamRom | game.TeamKli
	if server.gameState.Winner != expectedWinner {
		t.Errorf("Expected winner to be Fed|Rom|Kli (%d), got %d", expectedWinner, server.gameState.Winner)
	}

	// Check that victory message contains all three team names
	select {
	case msg := <-server.broadcast:
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Error("Expected message data to be a map")
		}
		messageText, ok := data["text"].(string)
		if !ok {
			t.Error("Expected message text to be a string")
		} else {
			if !contains(messageText, "Federation") {
				t.Errorf("Message should contain 'Federation', got: %s", messageText)
			}
			if !contains(messageText, "Romulan") {
				t.Errorf("Message should contain 'Romulan', got: %s", messageText)
			}
			if !contains(messageText, "Klingon") {
				t.Errorf("Message should contain 'Klingon', got: %s", messageText)
			}
			if !contains(messageText, "share victory") {
				t.Errorf("Message should indicate shared victory, got: %s", messageText)
			}
		}
	default:
		t.Error("Expected victory broadcast message")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
