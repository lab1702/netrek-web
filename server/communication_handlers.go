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

	if !c.validPlayerID() {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	// Check for bot commands (after sanitization)
	// Bot commands require an active (alive) player — validPlayerID is checked
	// above but we also verify the player exists before routing to bot handler.
	if strings.HasPrefix(msgData.Text, "/") {
		c.handleBotCommand(msgData.Text)
		return
	}

	playerID := c.GetPlayerID()
	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[playerID]
	if p == nil {
		c.server.gameState.Mu.RUnlock()
		return
	}
	senderName := formatPlayerName(p)
	c.server.gameState.Mu.RUnlock()

	// Broadcast to all players (non-blocking)
	select {
	case c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[ALL] %s: %s", senderName, msgData.Text),
			"type": "all",
			"from": playerID,
		},
	}:
	default:
	}
}

// handleTeamMessage handles team-only messages
func (c *Client) handleTeamMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if !c.validPlayerID() {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	// Read sender info and cache player teams in a single lock acquisition
	// to avoid stale routing between two separate RLock calls.
	playerID := c.GetPlayerID()
	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[playerID]
	if p == nil {
		c.server.gameState.Mu.RUnlock()
		return
	}
	senderName := formatPlayerName(p)
	team := p.Team
	playerTeams := make(map[int]int)
	for i, pl := range c.server.gameState.Players {
		if pl.Status != game.StatusFree {
			playerTeams[i] = pl.Team
		}
	}
	c.server.gameState.Mu.RUnlock()

	// Send to team members only
	teamMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[TEAM] %s: %s", senderName, msgData.Text),
			"type": "team",
			"from": playerID,
			"team": team,
		},
	}

	// Iterate clients under s.mu only (no nested gameState lock)
	c.server.mu.RLock()
	defer c.server.mu.RUnlock()
	for _, client := range c.server.clients {
		pid := client.GetPlayerID()
		if pid >= 0 && pid < game.MaxPlayers {
			if playerTeams[pid] == team {
				select {
				case client.send <- teamMsg:
				default:
					// Client's send channel is full, skip
				}
			}
		}
	}
}

// handlePrivateMessage handles private messages
func (c *Client) handlePrivateMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if !c.validPlayerID() {
		return
	}

	if msgData.Target < 0 || msgData.Target >= game.MaxPlayers {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	// Read player info under game state lock
	playerID := c.GetPlayerID()
	c.server.gameState.Mu.RLock()
	p := c.server.gameState.Players[playerID]
	targetPlayer := c.server.gameState.Players[msgData.Target]
	if p == nil || targetPlayer == nil {
		c.server.gameState.Mu.RUnlock()
		return
	}
	senderName := formatPlayerName(p)
	targetName := formatPlayerName(targetPlayer)
	c.server.gameState.Mu.RUnlock()

	// Send to target and sender only
	privMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[PRIV->%s] %s: %s", targetName, senderName, msgData.Text),
			"type": "private",
			"from": playerID,
			"to":   msgData.Target,
		},
	}

	c.server.mu.RLock()
	defer c.server.mu.RUnlock()
	for _, client := range c.server.clients {
		cid := client.GetPlayerID()
		if cid == msgData.Target || cid == playerID {
			select {
			case client.send <- privMsg:
			default:
				// Client's send channel is full, skip
			}
		}
	}
}
