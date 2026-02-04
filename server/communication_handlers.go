package server

import (
	"encoding/json"
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"strings"
)

// handleChatMessage handles all-players messages
func (c *Client) handleChatMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	// Check for bot commands (before sanitization)
	if strings.HasPrefix(msgData.Text, "/") {
		c.handleBotCommand(msgData.Text)
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	senderName := formatPlayerName(p)
	c.server.gameState.Mu.RUnlock()

	// Broadcast to all players
	c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[ALL] %s: %s", senderName, msgData.Text),
			"type": "all",
			"from": c.PlayerID,
		},
	}
}

// handleTeamMessage handles team-only messages
func (c *Client) handleTeamMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	// Read sender info under game state lock
	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	senderName := formatPlayerName(p)
	team := p.Team
	c.server.gameState.Mu.RUnlock()

	// Send to team members only
	teamMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[TEAM] %s: %s", senderName, msgData.Text),
			"type": "team",
			"from": c.PlayerID,
			"team": team,
		},
	}

	// Lock ordering: s.mu first, then s.gameState.Mu
	c.server.mu.RLock()
	c.server.gameState.Mu.RLock()
	for _, client := range c.server.clients {
		if client.PlayerID >= 0 && client.PlayerID < game.MaxPlayers {
			clientPlayer := c.server.gameState.Players[client.PlayerID]
			if clientPlayer.Team == team {
				select {
				case client.send <- teamMsg:
				default:
					// Client's send channel is full, skip
				}
			}
		}
	}
	c.server.gameState.Mu.RUnlock()
	c.server.mu.RUnlock()
}

// handlePrivateMessage handles private messages
func (c *Client) handlePrivateMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	if msgData.Target < 0 || msgData.Target >= game.MaxPlayers {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	// Read player info under game state lock
	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	targetPlayer := c.server.gameState.Players[msgData.Target]
	senderName := formatPlayerName(p)
	targetName := formatPlayerName(targetPlayer)
	c.server.gameState.Mu.RUnlock()

	// Send to target and sender only
	privMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[PRIV->%s] %s: %s", targetName, senderName, msgData.Text),
			"type": "private",
			"from": c.PlayerID,
			"to":   msgData.Target,
		},
	}

	c.server.mu.RLock()
	for _, client := range c.server.clients {
		if client.PlayerID == msgData.Target || client.PlayerID == c.PlayerID {
			select {
			case client.send <- privMsg:
			default:
				// Client's send channel is full, skip
			}
		}
	}
	c.server.mu.RUnlock()
}
