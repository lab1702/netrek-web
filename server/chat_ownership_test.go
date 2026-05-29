package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestChatMessageRequiresSlotOwnership verifies an all-chat message is only
// broadcast on behalf of a client that actually owns its player slot. A client
// whose slot was reassigned to another player must not be able to chat as the
// new occupant.
func TestChatMessageRequiresSlotOwnership(t *testing.T) {
	newClientOwningSlot0 := func() (*Server, *Client) {
		s := NewServer()
		for i := range s.gameState.Players {
			s.gameState.Players[i] = &game.Player{ID: i, Status: game.StatusFree, OwnerClientID: -1}
		}
		c := &Client{ID: 7, server: s, send: make(chan ServerMessage, 10)}
		c.SetPlayerID(0)
		p := s.gameState.Players[0]
		p.Status = game.StatusAlive
		p.Team = game.TeamFed
		p.Name = "Alice"
		p.OwnerClientID = 7
		return s, c
	}

	msg, _ := json.Marshal(MessageData{Text: "hello"})

	t.Run("OwnerCanChat", func(t *testing.T) {
		s, c := newClientOwningSlot0()
		c.handleChatMessage(msg)
		select {
		case <-s.broadcast:
		default:
			t.Error("the slot owner's chat message should be broadcast")
		}
	})

	t.Run("NonOwnerCannotChat", func(t *testing.T) {
		s, c := newClientOwningSlot0()
		// Slot 0 has been reassigned to a different client.
		s.gameState.Players[0].OwnerClientID = 99
		c.handleChatMessage(msg)
		select {
		case <-s.broadcast:
			t.Error("a client that no longer owns its slot must not be able to chat")
		default:
		}
	})
}
