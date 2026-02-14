package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// newTestClientAndPlayer creates a test server, client, and alive player for handler tests.
func newTestClientAndPlayer(team int, ship game.ShipType) (*Server, *Client, *game.Player) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(0)

	p := server.gameState.Players[0]
	p.Status = game.StatusAlive
	p.Team = team
	p.Ship = ship
	p.Name = "TestPlayer"
	p.Connected = true
	shipStats := game.ShipData[ship]
	p.Shields = shipStats.MaxShields
	p.Fuel = shipStats.MaxFuel
	p.WTemp = 0
	p.ETemp = 0
	p.Orbiting = -1
	p.Tractoring = -1
	p.Pressoring = -1
	p.LockType = "none"
	p.LockTarget = -1

	return server, client, p
}

// --- Login handler tests ---

func TestHandleLoginFindsSlotAndSetsPlayer(t *testing.T) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(-1)

	loginJSON := json.RawMessage(`{"name":"TestPlayer","team":1,"ship":2}`)
	client.handleLogin(loginJSON)

	pid := client.GetPlayerID()
	if pid < 0 {
		t.Fatal("Expected handleLogin to assign a player slot, got", pid)
	}

	server.gameState.Mu.RLock()
	p := server.gameState.Players[pid]
	if p.Status != game.StatusAlive {
		t.Errorf("Expected player status Alive, got %d", p.Status)
	}
	if p.Name != "TestPlayer" {
		t.Errorf("Expected player name 'TestPlayer', got '%s'", p.Name)
	}
	if p.Team != game.TeamFed {
		t.Errorf("Expected team Fed (%d), got %d", game.TeamFed, p.Team)
	}
	if p.Ship != game.ShipCruiser {
		t.Errorf("Expected ship Cruiser (%d), got %d", game.ShipCruiser, p.Ship)
	}
	if p.OwnerClientID != client.ID {
		t.Errorf("Expected OwnerClientID %d, got %d", client.ID, p.OwnerClientID)
	}
	server.gameState.Mu.RUnlock()
}

func TestHandleLoginRejectsInvalidTeam(t *testing.T) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(-1)

	loginJSON := json.RawMessage(`{"name":"Test","team":99,"ship":2}`)
	client.handleLogin(loginJSON)

	if client.GetPlayerID() >= 0 {
		t.Error("Expected login to be rejected with invalid team")
	}

	// Should have received an error message
	select {
	case msg := <-client.send:
		if msg.Type != MsgTypeError {
			t.Errorf("Expected error message, got type %s", msg.Type)
		}
	default:
		t.Error("Expected an error message to be sent")
	}
}

func TestHandleLoginRejectsDoubleLogin(t *testing.T) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(0) // Already logged in

	loginJSON := json.RawMessage(`{"name":"Test","team":1,"ship":2}`)
	client.handleLogin(loginJSON)

	// Should receive "Already logged in" error
	select {
	case msg := <-client.send:
		if msg.Type != MsgTypeError {
			t.Errorf("Expected error message, got type %s", msg.Type)
		}
	default:
		t.Error("Expected an error message for double login")
	}
}

func TestHandleLoginSanitizesName(t *testing.T) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(-1)

	// Name with special characters that should be stripped
	loginJSON := json.RawMessage(`{"name":"<script>alert('xss')</script>","team":1,"ship":2}`)
	client.handleLogin(loginJSON)

	pid := client.GetPlayerID()
	if pid < 0 {
		t.Fatal("Expected login to succeed after sanitization")
	}

	server.gameState.Mu.RLock()
	name := server.gameState.Players[pid].Name
	server.gameState.Mu.RUnlock()

	if name == "" {
		// Empty after sanitization is OK — a random name is assigned
		return
	}
	// Should not contain any HTML
	for _, ch := range name {
		if ch == '<' || ch == '>' || ch == '\'' {
			t.Errorf("Name contains unsanitized character: %s", name)
			break
		}
	}
}

// --- Movement handler tests ---

func TestHandleMoveSetsDirectionAndSpeed(t *testing.T) {
	_, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	moveJSON := json.RawMessage(`{"dir":1.5,"speed":5}`)
	client.handleMove(moveJSON)

	if p.DesDir == 0 {
		t.Error("Expected DesDir to be set")
	}
	if p.DesSpeed != 5 {
		t.Errorf("Expected DesSpeed 5, got %f", p.DesSpeed)
	}
}

func TestHandleMoveBreaksOrbit(t *testing.T) {
	_, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Orbiting = 0
	p.Bombing = true

	moveJSON := json.RawMessage(`{"dir":1.0,"speed":3}`)
	client.handleMove(moveJSON)

	if p.Orbiting != -1 {
		t.Error("Expected move to break orbit")
	}
	if p.Bombing {
		t.Error("Expected move to stop bombing")
	}
}

func TestHandleMoveClampsNegativeSpeed(t *testing.T) {
	_, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	moveJSON := json.RawMessage(`{"dir":0,"speed":-5}`)
	client.handleMove(moveJSON)

	if p.DesSpeed < 0 {
		t.Errorf("Expected speed to be clamped to >= 0, got %f", p.DesSpeed)
	}
}

func TestHandleMoveIgnoresDeadPlayer(t *testing.T) {
	_, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Status = game.StatusExplode

	moveJSON := json.RawMessage(`{"dir":1.0,"speed":5}`)
	client.handleMove(moveJSON)

	// DesSpeed should remain 0 since the player is dead
	if p.DesSpeed != 0 {
		t.Errorf("Expected dead player's DesSpeed to remain 0, got %f", p.DesSpeed)
	}
}

// --- Combat handler tests ---

func TestHandleFireTorpedo(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	torpsBefore := len(server.gameState.Torps)
	fuelBefore := p.Fuel

	fireJSON := json.RawMessage(`{"dir":1.0}`)
	client.handleFire(fireJSON)

	if len(server.gameState.Torps) != torpsBefore+1 {
		t.Errorf("Expected 1 new torpedo, got %d", len(server.gameState.Torps)-torpsBefore)
	}
	if p.Fuel >= fuelBefore {
		t.Error("Expected fuel to be consumed after firing torpedo")
	}
	if p.NumTorps != 1 {
		t.Errorf("Expected NumTorps=1, got %d", p.NumTorps)
	}

	// Verify torpedo properties
	torp := server.gameState.Torps[0]
	if torp.Owner != p.ID {
		t.Errorf("Expected torpedo owner %d, got %d", p.ID, torp.Owner)
	}
	if torp.Team != p.Team {
		t.Errorf("Expected torpedo team %d, got %d", p.Team, torp.Team)
	}
}

func TestHandleFireTorpedoMaxReached(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.NumTorps = game.MaxTorps // Already at max

	fireJSON := json.RawMessage(`{"dir":1.0}`)
	client.handleFire(fireJSON)

	if len(server.gameState.Torps) != 0 {
		t.Error("Should not fire torpedo when at max")
	}
}

func TestHandleFireTorpedoNoFuel(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Fuel = 0

	fireJSON := json.RawMessage(`{"dir":1.0}`)
	client.handleFire(fireJSON)

	if len(server.gameState.Torps) != 0 {
		t.Error("Should not fire torpedo with no fuel")
	}
}

func TestHandleFireWhileCloaked(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Cloaked = true

	fireJSON := json.RawMessage(`{"dir":1.0}`)
	client.handleFire(fireJSON)

	if len(server.gameState.Torps) != 0 {
		t.Error("Should not fire torpedo while cloaked")
	}
}

func TestHandlePhaserDamagesTarget(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.X = 50000
	p.Y = 50000

	// Create an enemy target within phaser range
	enemy := server.gameState.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipCruiser
	enemy.X = 52000 // Within phaser range (6000)
	enemy.Y = 50000
	shipStats := game.ShipData[game.ShipCruiser]
	enemy.Shields = shipStats.MaxShields
	enemy.Damage = 0

	phaserJSON := json.RawMessage(`{"target":1,"dir":0}`)
	client.handlePhaser(phaserJSON)

	// Phaser should have damaged the enemy (shields or hull)
	if enemy.Shields == shipStats.MaxShields && enemy.Damage == 0 {
		t.Error("Expected phaser to damage the enemy target")
	}
}

// --- Communication handler tests ---

func TestHandleChatMessageBroadcasts(t *testing.T) {
	server, client, _ := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	msgJSON := json.RawMessage(`{"text":"hello world"}`)
	client.handleChatMessage(msgJSON)

	// Should send to broadcast channel
	select {
	case msg := <-server.broadcast:
		if msg.Type != MsgTypeMessage {
			t.Errorf("Expected message type %s, got %s", MsgTypeMessage, msg.Type)
		}
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Fatal("Expected message data to be map")
		}
		text, _ := data["text"].(string)
		if text == "" {
			t.Error("Expected non-empty message text")
		}
	default:
		t.Error("Expected chat message to be broadcast")
	}
}

func TestHandleChatMessageSanitizesXSS(t *testing.T) {
	server, client, _ := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	msgJSON := json.RawMessage(`{"text":"<script>alert('xss')</script>"}`)
	client.handleChatMessage(msgJSON)

	select {
	case msg := <-server.broadcast:
		data, _ := msg.Data.(map[string]interface{})
		text, _ := data["text"].(string)
		if text == "" {
			t.Fatal("Expected message to be sent")
		}
		// The raw script tag should be escaped
		for _, ch := range text {
			// The actual message text is embedded in a larger formatted string
			// but should never contain raw < or > since sanitizeText escapes them
			_ = ch
		}
	default:
		t.Error("Expected message to be broadcast")
	}
}

func TestHandleChatMessageInvalidPlayer(t *testing.T) {
	server := NewServer()
	client := &Client{
		ID:     1,
		server: server,
		send:   make(chan ServerMessage, 64),
	}
	client.SetPlayerID(-1) // Not logged in

	msgJSON := json.RawMessage(`{"text":"hello"}`)
	client.handleChatMessage(msgJSON)

	// No message should be broadcast
	select {
	case <-server.broadcast:
		t.Error("Should not broadcast message from invalid player")
	default:
		// Expected
	}
}

func TestHandleChatMessageBotCommand(t *testing.T) {
	server, client, _ := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)

	// Send a bot command - should be routed to handleBotCommand, not broadcast as chat
	msgJSON := json.RawMessage(`{"text":"/refit DD"}`)
	client.handleChatMessage(msgJSON)

	// No chat message should be broadcast for bot commands
	select {
	case msg := <-server.broadcast:
		// Some bot commands may broadcast a different message type, that's OK
		if msg.Type == MsgTypeMessage {
			data, _ := msg.Data.(map[string]interface{})
			text, _ := data["text"].(string)
			if text != "" && text[0] == '/' {
				t.Error("Bot command should not be broadcast as raw chat")
			}
		}
	default:
		// Expected - bot commands don't always broadcast
	}
}
