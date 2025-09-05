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

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	c.server.mu.RUnlock()

	// Broadcast to all players
	c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[ALL] %s: %s", formatPlayerName(p), msgData.Text),
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

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	team := p.Team
	c.server.mu.RUnlock()

	// Send to team members only
	teamMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[TEAM] %s: %s", formatPlayerName(p), msgData.Text),
			"type": "team",
			"from": c.PlayerID,
			"team": team,
		},
	}

	c.server.mu.RLock()
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

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	targetPlayer := c.server.gameState.Players[msgData.Target]
	c.server.mu.RUnlock()

	// Send to target and sender only
	privMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[PRIV->%s] %s: %s", formatPlayerName(targetPlayer), formatPlayerName(p), msgData.Text),
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
