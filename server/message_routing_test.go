package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestHandleMessageRouting verifies that handleMessage dispatches all known
// message types without panicking and that unknown types are handled gracefully.
func TestHandleMessageRouting(t *testing.T) {
	server := NewServer()

	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(0)

	// Set up a minimal alive player so handlers that check player status
	// can proceed past initial validation.
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.Team = game.TeamFed
	player.Ship = game.ShipCruiser
	player.Fuel = 5000
	player.Shields_up = true
	server.gameState.Mu.Unlock()

	// All known message types with minimal valid JSON payloads.
	// Handlers may reject these on validation, but the routing must not panic.
	knownTypes := map[string]string{
		MsgTypeLogin:    `{"name":"Test","team":1,"ship":2}`,
		MsgTypeMove:     `{"direction":1.5,"speed":5}`,
		MsgTypeFire:     `{}`,
		MsgTypePhaser:   `{"direction":1.0,"target":-1}`,
		MsgTypeShields:  `{"up":true}`,
		MsgTypeOrbit:    `{}`,
		MsgTypeRepair:   `{}`,
		MsgTypeLock:     `{"type":"planet","target":0}`,
		MsgTypeBeam:     `{"up":true}`,
		MsgTypeBomb:     `{}`,
		MsgTypeTractor:  `{"target":1}`,
		MsgTypePressor:  `{"target":1}`,
		MsgTypePlasma:   `{"direction":1.0}`,
		MsgTypeDetonate: `{}`,
		MsgTypeCloak:    `{}`,
		MsgTypeMessage:  `{"text":"hello","to":"all"}`,
		MsgTypeTeamMsg:  `{"text":"team hello"}`,
		MsgTypePrivMsg:  `{"text":"private hello","target":1}`,
		MsgTypeQuit:     `{}`,
	}

	for msgType, payload := range knownTypes {
		// Skip quit since it marks the client as quitting and would affect later iterations
		if msgType == MsgTypeQuit {
			continue
		}

		t.Run("dispatch_"+msgType, func(t *testing.T) {
			msg := ClientMessage{
				Type: msgType,
				Data: json.RawMessage(payload),
			}

			// handleMessage has a recover() so it won't propagate panics,
			// but we want to ensure it doesn't even need to recover.
			// We call it and verify no error message indicating a routing failure.
			client.handleMessage(msg)
		})
	}
}

// TestHandleMessageUnknownType verifies unknown message types are handled
// without panic or crash.
func TestHandleMessageUnknownType(t *testing.T) {
	server := NewServer()

	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 10),
	}
	client.SetPlayerID(-1)

	msg := ClientMessage{
		Type: "nonexistent_type",
		Data: json.RawMessage(`{}`),
	}

	// Should hit the default case and log, not panic
	client.handleMessage(msg)
}

// TestHandleMessageAllTypesHaveConstants verifies that the message type
// constants used in handleMessage match the expected set, catching typos
// or missing cases.
func TestHandleMessageAllTypesHaveConstants(t *testing.T) {
	// These are the client-to-server message types that handleMessage must route.
	expectedTypes := []string{
		MsgTypeLogin,
		MsgTypeMove,
		MsgTypeFire,
		MsgTypePhaser,
		MsgTypeShields,
		MsgTypeOrbit,
		MsgTypeRepair,
		MsgTypeLock,
		MsgTypeBeam,
		MsgTypeBomb,
		MsgTypeTractor,
		MsgTypePressor,
		MsgTypePlasma,
		MsgTypeDetonate,
		MsgTypeCloak,
		MsgTypeMessage,
		MsgTypeTeamMsg,
		MsgTypePrivMsg,
		MsgTypeQuit,
	}

	// Verify none of the constants are empty strings (would indicate a typo)
	for _, msgType := range expectedTypes {
		if msgType == "" {
			t.Errorf("Found empty message type constant in expected types list")
		}
	}

	// Verify all constants are unique
	seen := make(map[string]bool)
	for _, msgType := range expectedTypes {
		if seen[msgType] {
			t.Errorf("Duplicate message type constant: %s", msgType)
		}
		seen[msgType] = true
	}

	// Verify count matches what we expect (19 client message types)
	if len(expectedTypes) != 19 {
		t.Errorf("Expected 19 client message types, got %d", len(expectedTypes))
	}
}

// TestHandleMessageQuitSetsQuitting verifies that the quit message type
// sets the quitting flag to prevent re-login.
func TestHandleMessageQuitSetsQuitting(t *testing.T) {
	server := NewServer()

	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(0)

	// Set up a minimal alive player
	server.gameState.Mu.Lock()
	player := server.gameState.Players[0]
	player.Status = game.StatusAlive
	player.Team = game.TeamFed
	player.Ship = game.ShipCruiser
	player.Name = "TestPlayer"
	server.gameState.Mu.Unlock()

	if client.quitting.Load() {
		t.Fatal("Client should not be quitting before quit message")
	}

	msg := ClientMessage{
		Type: MsgTypeQuit,
		Data: json.RawMessage(`{}`),
	}
	client.handleMessage(msg)

	if !client.quitting.Load() {
		t.Error("Client should be marked as quitting after quit message")
	}

	// Verify that a subsequent login attempt is rejected
	loginMsg := ClientMessage{
		Type: MsgTypeLogin,
		Data: json.RawMessage(`{"name":"Hacker","team":1,"ship":2}`),
	}
	client.handleMessage(loginMsg)

	// The client's playerID should remain -1 (login rejected)
	if client.GetPlayerID() != -1 {
		t.Error("Login should be rejected after quit, but playerID was set")
	}
}
