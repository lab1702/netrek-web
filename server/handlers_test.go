package server

import (
	"strings"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// Test the /refit command functionality
func TestRefitCommand(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client
	client := &Client{
		ID:       1,
		PlayerID: 0,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Initialize a test player
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.Ship = game.ShipScout // Start with Scout
	player.NextShipType = -1     // No pending refit initially
	server.gameState.Mu.Unlock()

	// Test valid refit command
	client.handleBotCommand("/refit DD")

	// Check that NextShipType was set correctly
	server.gameState.Mu.RLock()
	if player.NextShipType != 1 { // 1 = Destroyer
		t.Errorf("Expected NextShipType to be 1 (Destroyer), got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()

	// Test that a confirmation message was sent
	select {
	case msg := <-client.send:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Expected message data to be map[string]interface{}")
		}
		text, ok := data["text"].(string)
		if !ok || text != "Refit to Destroyer when you next respawn." {
			t.Errorf("Expected confirmation message, got: %v", text)
		}
	default:
		t.Errorf("Expected a confirmation message to be sent")
	}

	// Test respawn with pending refit
	server.respawnPlayer(player)

	// Check that ship type changed and NextShipType was reset
	server.gameState.Mu.RLock()
	if player.Ship != game.ShipDestroyer {
		t.Errorf("Expected ship type to be Destroyer after respawn, got %v", player.Ship)
	}
	if player.NextShipType != -1 {
		t.Errorf("Expected NextShipType to be reset to -1 after respawn, got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()
}

// Test invalid refit command
func TestRefitCommandInvalid(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client
	client := &Client{
		ID:       1,
		PlayerID: 0,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Initialize a test player
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.NextShipType = -1 // No pending refit initially
	server.gameState.Mu.Unlock()

	// Test invalid ship type
	client.handleBotCommand("/refit INVALID")

	// Check that NextShipType was not changed
	server.gameState.Mu.RLock()
	if player.NextShipType != -1 {
		t.Errorf("Expected NextShipType to remain -1 for invalid command, got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()

	// Test that an error message was sent
	select {
	case msg := <-client.send:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Expected message data to be map[string]interface{}")
		}
		msgType, ok := data["type"].(string)
		if !ok || msgType != "warning" {
			t.Errorf("Expected warning message type, got: %v", msgType)
		}
	default:
		t.Errorf("Expected an error message to be sent")
	}
}

// Test refit command with no arguments
func TestRefitCommandNoArgs(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client
	client := &Client{
		ID:       1,
		PlayerID: 0,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Test refit with no arguments
	client.handleBotCommand("/refit")

	// Test that a usage message was sent
	select {
	case msg := <-client.send:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Expected message data to be map[string]interface{}")
		}
		text, ok := data["text"].(string)
		if !ok || text != "Usage: /refit SC|DD|CA|BB|AS|SB|GA" {
			t.Errorf("Expected usage message, got: %v", text)
		}
	default:
		t.Errorf("Expected a usage message to be sent")
	}
}

// Test ship alias case insensitivity
func TestRefitCommandCaseInsensitive(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client
	client := &Client{
		ID:       1,
		PlayerID: 0,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Initialize a test player
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.NextShipType = -1
	server.gameState.Mu.Unlock()

	// Test lowercase command
	client.handleBotCommand("/refit dd")

	// Check that NextShipType was set correctly
	server.gameState.Mu.RLock()
	if player.NextShipType != 1 { // 1 = Destroyer
		t.Errorf("Expected NextShipType to be 1 (Destroyer) for lowercase 'dd', got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()

	// Drain the confirmation message
	<-client.send

	// Reset for mixed case test
	server.gameState.Mu.Lock()
	player.NextShipType = -1
	server.gameState.Mu.Unlock()

	// Test mixed case command
	client.handleBotCommand("/refit Ca")

	// Check that NextShipType was set correctly
	server.gameState.Mu.RLock()
	if player.NextShipType != 2 { // 2 = Cruiser
		t.Errorf("Expected NextShipType to be 2 (Cruiser) for mixed case 'Ca', got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()
}

// Test starbase refit limitation
func TestRefitStarbaseLimitation(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client for player 2
	client2 := &Client{
		ID:       2,
		PlayerID: 1,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Initialize test players on the same team
	server.gameState.Mu.Lock()
	player1 := server.gameState.Players[0]
	player1.Status = game.StatusAlive
	player1.Ship = game.ShipStarbase // Player 1 is already a starbase
	player1.Team = game.TeamFed
	player1.Connected = true
	player1.NextShipType = -1

	player2 := server.gameState.Players[1]
	player2.Status = game.StatusAlive
	player2.Ship = game.ShipDestroyer // Player 2 is a destroyer
	player2.Team = game.TeamFed       // Same team as player 1
	player2.Connected = true
	player2.NextShipType = -1
	server.gameState.Mu.Unlock()

	// Try to refit player 2 to starbase (should fail)
	client2.handleBotCommand("/refit SB")

	// Check that NextShipType was not set for player 2
	server.gameState.Mu.RLock()
	if player2.NextShipType != -1 {
		t.Errorf("Expected NextShipType to remain -1 when team already has starbase, got %d", player2.NextShipType)
	}
	server.gameState.Mu.RUnlock()

	// Check that a warning message was sent
	select {
	case msg := <-client2.send:
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Expected message data to be map[string]interface{}")
		}
		msgType, ok := data["type"].(string)
		if !ok || msgType != "warning" {
			t.Errorf("Expected warning message type, got: %v", msgType)
		}
		text, ok := data["text"].(string)
		if !ok || !strings.Contains(text, "starbase") {
			t.Errorf("Expected starbase limitation message, got: %v", text)
		}
	default:
		t.Errorf("Expected a warning message to be sent")
	}
}

// Test starbase refit when current player is the starbase (should be allowed)
func TestRefitStarbaseCurrentPlayerIsStarbase(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Create a mock client
	client := &Client{
		ID:       1,
		PlayerID: 0,
		server:   server,
		send:     make(chan ServerMessage, 10),
	}

	// Initialize test player as a starbase
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.Ship = game.ShipStarbase // Player is currently a starbase
	player.Team = game.TeamFed
	player.Connected = true
	player.NextShipType = -1
	server.gameState.Mu.Unlock()

	// Try to refit to starbase again (should be allowed since they're already the starbase)
	client.handleBotCommand("/refit SB")

	// Check that NextShipType was set correctly
	server.gameState.Mu.RLock()
	if player.NextShipType != int(game.ShipStarbase) {
		t.Errorf("Expected NextShipType to be set to starbase when current player is the starbase, got %d", player.NextShipType)
	}
	server.gameState.Mu.RUnlock()

	// Check that a confirmation message was sent
	select {
	case msg := <-client.send:
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Errorf("Expected message data to be map[string]interface{}")
		}
		msgType, ok := data["type"].(string)
		if !ok || msgType != "info" {
			t.Errorf("Expected info message type, got: %v", msgType)
		}
	default:
		t.Errorf("Expected a confirmation message to be sent")
	}
}

// Test respawn with starbase refit when team already has one
func TestRespawnStarbaseRefitLimitation(t *testing.T) {
	// Create a test server
	server := NewServer()

	// Initialize two players on the same team
	server.gameState.Mu.Lock()
	player1 := server.gameState.Players[0]
	player1.Status = game.StatusAlive
	player1.Ship = game.ShipStarbase // Player 1 is a starbase
	player1.Team = game.TeamFed
	player1.Connected = true

	player2 := server.gameState.Players[1]
	player2.Status = game.StatusDead
	player2.Ship = game.ShipDestroyer
	player2.Team = game.TeamFed // Same team
	player2.Connected = true
	player2.NextShipType = int(game.ShipStarbase) // Wants to refit to starbase
	server.gameState.Mu.Unlock()

	// Respawn player 2
	server.respawnPlayer(player2)

	// Check that player 2 did NOT become a starbase and refit was cancelled
	server.gameState.Mu.RLock()
	if player2.Ship == game.ShipStarbase {
		t.Errorf("Expected player 2 to NOT become starbase when team already has one")
	}
	if player2.NextShipType != -1 {
		t.Errorf("Expected NextShipType to be reset to -1 after failed refit, got %d", player2.NextShipType)
	}
	server.gameState.Mu.RUnlock()
}
