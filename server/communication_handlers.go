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

	// Limit message length; preserve plain text for the client.
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
	p := c.getPlayer() // ownership-checked: nil if the slot was reassigned
	if p == nil {
		c.server.gameState.Mu.RUnlock()
		return
	}
	senderName := formatPlayerName(p)
	c.server.gameState.Mu.RUnlock()

	// Broadcast to all players (non-blocking)
	c.server.tryBroadcast(ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[ALL] %s: %s", senderName, msgData.Text),
			"type": "all",
			"from": playerID,
		},
	})
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

	// Limit message length; preserve plain text for the client.
	msgData.Text = sanitizeText(msgData.Text)

	// Keep client assignments and slot ownership stable through delivery.
	// This is the same server -> game-state lock order used by reset and snapshots.
	c.server.mu.RLock()
	defer c.server.mu.RUnlock()
	c.server.gameState.Mu.RLock()
	defer c.server.gameState.Mu.RUnlock()
	p := c.getPlayer()
	if p == nil || p.OwnerClientID != c.ID || !p.Connected || p.Status == game.StatusFree {
		return
	}
	playerID, senderName, team := p.ID, formatPlayerName(p), p.Team

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

	for _, client := range c.server.clients {
		recipient := client.getPlayer()
		if recipient != nil && recipient.OwnerClientID == client.ID && recipient.Connected &&
			recipient.Status != game.StatusFree && recipient.Team == team {
			client.sendMsg(teamMsg)
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

	// Limit message length; preserve plain text for the client.
	msgData.Text = sanitizeText(msgData.Text)

	c.server.mu.RLock()
	defer c.server.mu.RUnlock()
	c.server.gameState.Mu.RLock()
	defer c.server.gameState.Mu.RUnlock()
	p := c.getPlayer()
	targetPlayer := c.server.gameState.Players[msgData.Target]
	if p == nil || p.OwnerClientID != c.ID || !p.Connected || p.Status == game.StatusFree ||
		targetPlayer == nil || !targetPlayer.Connected || targetPlayer.Status == game.StatusFree {
		return
	}
	playerID := p.ID
	senderName := formatPlayerName(p)
	targetName := formatPlayerName(targetPlayer)

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

	for _, client := range c.server.clients {
		if client.ID == targetPlayer.OwnerClientID || client.ID == p.OwnerClientID {
			client.sendMsg(privMsg)
		}
	}
}
