package server

import (
	"encoding/json"
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"log"
	"math/rand"
	"time"
)

// handleLogin processes login requests
func (c *Client) handleLogin(data json.RawMessage) {
	var loginData LoginData
	if err := json.Unmarshal(data, &loginData); err != nil {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid login data",
		}
		return
	}

	// Validate team and ship type
	if !validateTeam(loginData.Team) {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid team selection",
		}
		return
	}

	if !validateShipType(loginData.Ship) {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid ship type",
		}
		return
	}

	// Sanitize the player name to prevent XSS
	loginData.Name = sanitizeName(loginData.Name)

	// Ensure name is not empty after sanitization
	if loginData.Name == "" {
		loginData.Name = fmt.Sprintf("Player%d", rand.Intn(1000))
	}

	// Find a player slot
	c.server.gameState.Mu.Lock()

	playerID := -1

	// Check team balance
	{
		// Count players per team (only count connected, alive players)
		teamCounts := make(map[int]int)
		// Initialize all teams to 0
		teamCounts[game.TeamFed] = 0
		teamCounts[game.TeamRom] = 0
		teamCounts[game.TeamKli] = 0
		teamCounts[game.TeamOri] = 0

		for _, p := range c.server.gameState.Players {
			if p.Status == game.StatusAlive && p.Connected {
				teamCounts[p.Team]++
			}
		}

		// Find the maximum team size
		maxCount := 0
		for _, count := range teamCounts {
			if count > maxCount {
				maxCount = count
			}
		}

		// Check if the requested team would have more players than others after joining
		requestedTeamCount := teamCounts[loginData.Team]

		// If this team already has the max number of players and at least one other team has fewer
		if requestedTeamCount >= maxCount && maxCount > 0 {
			// Check if at least one other team has fewer players
			hasFewerTeam := false
			for team, count := range teamCounts {
				if team != loginData.Team && count < requestedTeamCount {
					hasFewerTeam = true
					break
				}
			}

			if hasFewerTeam {
				// Reject - this team already has the most players
				log.Printf("Team balance enforced: Player %s denied joining team %d (would have %d players, other teams have fewer)",
					loginData.Name, loginData.Team, requestedTeamCount+1)
				c.send <- ServerMessage{
					Type: MsgTypeError,
					Data: "Team is full. Please join a team with fewer players for balance.",
				}
				c.server.gameState.Mu.Unlock()
				return
			}
		}

		// Log successful team join (show counts BEFORE this player joins)
		log.Printf("Player %s joining team %d (current counts before join: Fed=%d, Rom=%d, Kli=%d, Ori=%d)",
			loginData.Name, loginData.Team,
			teamCounts[game.TeamFed], teamCounts[game.TeamRom],
			teamCounts[game.TeamKli], teamCounts[game.TeamOri])
	}

	// Check starbase limit (each team can have at most 1 starbase)
	if loginData.Ship == game.ShipStarbase {
		starbaseCounts := c.server.countStarbasesByTeam()
		if starbaseCounts[loginData.Team] >= 1 {
			c.send <- ServerMessage{
				Type: MsgTypeError,
				Data: "Your team already has a starbase. Only one starbase per team is allowed.",
			}
			c.server.gameState.Mu.Unlock()
			return
		}
	}

	// Find a free slot
	for i := 0; i < game.MaxPlayers; i++ {
		if c.server.gameState.Players[i].Status == game.StatusFree {
			playerID = i
			break
		}
	}

	if playerID == -1 {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Server full",
		}
		c.server.gameState.Mu.Unlock()
		return
	}

	// Set up the player (use pointer to modify in place)
	p := c.server.gameState.Players[playerID]

	// Initialize new player
	p.Name = loginData.Name
	p.Team = loginData.Team
	p.Ship = loginData.Ship
	p.Status = game.StatusAlive

	// Set starting position near home planet with random offset (like original Netrek)
	var homeX, homeY float64
	switch loginData.Team {
	case game.TeamFed:
		homeX, homeY = 20000, 80000 // Earth
	case game.TeamRom:
		homeX, homeY = 20000, 20000 // Romulus
	case game.TeamKli:
		homeX, homeY = 80000, 20000 // Klingus
	case game.TeamOri:
		homeX, homeY = 80000, 80000 // Orion
	default:
		homeX, homeY = 50000, 50000 // Center
	}

	// Add random offset between -5000 and +5000 (from original: random() % 10000 - 5000)
	offsetX := float64(rand.Intn(10000) - 5000)
	offsetY := float64(rand.Intn(10000) - 5000)
	p.X = homeX + offsetX
	p.Y = homeY + offsetY

	// Initialize ship stats
	shipStats := game.ShipData[loginData.Ship]
	p.Shields = shipStats.MaxShields
	p.Damage = 0
	p.Fuel = shipStats.MaxFuel
	p.Armies = 0
	p.WTemp = 0
	p.ETemp = 0
	p.Dir = 0
	p.Speed = 0
	p.DesDir = 0
	p.DesSpeed = 0
	p.Connected = true
	p.Shields_up = false // Shields DOWN by default
	p.NextShipType = -1  // No pending refit on join

	c.PlayerID = playerID

	// Send success response
	c.send <- ServerMessage{
		Type: "login_success",
		Data: map[string]interface{}{
			"player_id": playerID,
			"team":      loginData.Team,
			"ship":      loginData.Ship,
		},
	}

	shipData := game.ShipData[p.Ship]
	log.Printf("Player %s joined as %s on team %d", loginData.Name, shipData.Name, loginData.Team)

	// Unlock before broadcasting to avoid deadlock
	c.server.gameState.Mu.Unlock()

	// Broadcast updated team counts to all clients
	c.server.broadcastTeamCounts()
}

// handleQuit handles player quit/self-destruct request
func (c *Client) handleQuit(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		// If already dead, just disconnect
		c.server.gameState.Mu.Unlock()
		c.conn.Close()
		return
	}

	// Self-destruct the ship
	p.Status = game.StatusExplode
	p.ExplodeTimer = 10       // Explosion animation frames
	p.KilledBy = c.PlayerID   // Killed by self
	p.WhyDead = game.KillQuit // Quit reason

	// Stop all movement
	p.Speed = 0
	p.DesSpeed = 0

	// Clear all states
	// Clear lock-on when destroyed
	p.LockType = "none"
	p.LockTarget = -1
	p.Shields_up = false
	p.Cloaked = false
	p.Repairing = false
	p.RepairRequest = false
	p.Bombing = false
	p.Beaming = false
	p.BeamingUp = false
	p.Tractoring = -1
	p.Pressoring = -1
	p.Orbiting = -1

	selfDestructMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("%s self-destructed", p.Name),
			"type": "warning",
		},
	}

	// Unlock before broadcasting to avoid deadlock
	c.server.gameState.Mu.Unlock()

	// Broadcast self-destruct message after releasing lock
	c.server.broadcast <- selfDestructMsg

	// Broadcast updated team counts to all clients
	c.server.broadcastTeamCounts()

	// Close the connection after a short delay to allow the explosion to be seen
	go func() {
		select {
		case <-time.After(1 * time.Second):
			c.conn.Close()
		case <-c.server.done:
			c.conn.Close()
		}
	}()
}
