package server

import (
	"testing"
	"time"
)

// TestBotCmdThrottled verifies the shared bot-command rate limiter: it rejects
// (and notifies) when called within the cooldown window, and otherwise records
// the time and allows the command. This guard is applied to every bot command,
// including /removebot and /balance which previously had none.
func TestBotCmdThrottled(t *testing.T) {
	newClient := func() *Client {
		return &Client{
			ID:             1,
			server:         NewServer(),
			send:           make(chan ServerMessage, 10),
			botCmdCooldown: 10 * time.Second,
		}
	}

	t.Run("AllowsWhenCooldownElapsed", func(t *testing.T) {
		c := newClient()
		c.lastBotCmd = time.Now().Add(-time.Hour) // well past cooldown (also the zero-value case)
		if c.botCmdThrottled() {
			t.Error("command should be allowed when the cooldown has elapsed")
		}
	})

	t.Run("RejectsWithinCooldown", func(t *testing.T) {
		c := newClient()
		// First call records the timestamp and is allowed.
		if c.botCmdThrottled() {
			t.Fatal("first command should be allowed")
		}
		// Immediate second call is within the cooldown window and must be rejected.
		if !c.botCmdThrottled() {
			t.Error("a second command within the cooldown window must be rejected")
		}
		// A warning message should have been queued to the client.
		select {
		case <-c.send:
		default:
			t.Error("expected a rate-limit warning message to be sent to the client")
		}
	})
}
