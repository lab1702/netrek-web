package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestRepairStartMessageIsPrivate verifies that the "is repairing damage"
// notice sent when a ship begins repairing is addressed only to the repairing
// player (a "to" field), matching the repair-completion message. Without it the
// message is broadcast to every connected client, spamming and leaking each
// ship's repair status.
func TestRepairStartMessageIsPrivate(t *testing.T) {
	gs := game.NewGameState()
	s := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	const idx = 3
	p := gs.Players[idx]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.RepairRequest = true
	p.Speed = 0
	p.Orbiting = -1

	s.updatePlayerSystems(p, idx)

	select {
	case msg := <-s.broadcast:
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected message data type %T", msg.Data)
		}
		to, ok := data["to"]
		if !ok {
			t.Fatal("repair-start message has no 'to' field; it is broadcast to all clients")
		}
		if to != idx {
			t.Errorf("repair-start message addressed to %v, want %d", to, idx)
		}
	default:
		t.Fatal("expected a repair-start broadcast message")
	}
}
