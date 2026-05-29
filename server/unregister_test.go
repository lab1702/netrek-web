package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestFreeDisconnectedSlotOwnership verifies a disconnecting client only frees
// the player slot it actually owns. A stale/delayed unregister must not wipe a
// slot that has since been reassigned to a different client.
func TestFreeDisconnectedSlotOwnership(t *testing.T) {
	newServer := func() *Server {
		s := NewServer()
		for i := range s.gameState.Players {
			s.gameState.Players[i] = &game.Player{ID: i, Status: game.StatusFree, OwnerClientID: -1}
		}
		return s
	}

	t.Run("FreesSlotOwnedByClient", func(t *testing.T) {
		s := newServer()
		p := s.gameState.Players[5]
		p.Status = game.StatusAlive
		p.OwnerClientID = 42
		p.Name = "Alice"

		if !s.freeDisconnectedSlot(42, 5) {
			t.Fatal("should free a slot owned by the disconnecting client")
		}
		if p.Status != game.StatusFree || p.OwnerClientID != -1 || p.Connected {
			t.Error("slot should be reset to free/unowned")
		}
	})

	t.Run("DoesNotFreeSlotReassignedToAnotherClient", func(t *testing.T) {
		s := newServer()
		p := s.gameState.Players[5]
		p.Status = game.StatusAlive
		p.OwnerClientID = 99 // slot now owned by a different client
		p.Name = "Bob"

		if s.freeDisconnectedSlot(42, 5) {
			t.Error("a stale unregister from client 42 must not free a slot owned by client 99")
		}
		if p.Status != game.StatusAlive || p.OwnerClientID != 99 {
			t.Error("the new owner's live session must be untouched")
		}
	})

	t.Run("DoesNotFreeBot", func(t *testing.T) {
		s := newServer()
		p := s.gameState.Players[5]
		p.Status = game.StatusAlive
		p.IsBot = true
		p.OwnerClientID = -1

		if s.freeDisconnectedSlot(42, 5) {
			t.Error("bots must not be freed by a client disconnect")
		}
		if p.Status != game.StatusAlive {
			t.Error("bot slot must be untouched")
		}
	})

	t.Run("OutOfRangeIsNoop", func(t *testing.T) {
		s := newServer()
		if s.freeDisconnectedSlot(42, -1) || s.freeDisconnectedSlot(42, game.MaxPlayers) {
			t.Error("out-of-range player IDs must be a no-op")
		}
	})
}
