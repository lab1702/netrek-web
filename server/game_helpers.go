package server

import (
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"math/rand"
)

// formatPlayerName returns the player name with team and slot info
// Format: "Name [Team Slot]" e.g., "Player1 [Rom 2]"
func formatPlayerName(p *game.Player) string {
	teamName := ""
	switch p.Team {
	case game.TeamFed:
		teamName = "F"
	case game.TeamRom:
		teamName = "R"
	case game.TeamKli:
		teamName = "K"
	case game.TeamOri:
		teamName = "O"
	default:
		teamName = "I"
	}

	// Player ID is the slot number (0-based in internal, but display as 1-based)
	slot := p.ID
	return fmt.Sprintf("%s [%s%d]", p.Name, teamName, slot)
}

// respawnPlayer respawns a dead player at their home planet
func (s *Server) respawnPlayer(p *game.Player) {
	// IMPORTANT: Preserve the ship type for bots unless they have a pending refit
	// Bots should respawn with the same ship type, just like human players
	currentShipType := p.Ship

	// Reset player state
	p.Status = game.StatusAlive
	p.ExplodeTimer = 0
	p.WhyDead = 0
	p.KilledBy = -1
	p.KillsStreak = 0 // Reset kill streak on death

	// Check for pending refit before resetting ship stats
	if p.NextShipType >= 0 && p.NextShipType < len(game.ShipData) {
		// Special check for starbase refit - ensure team doesn't already have one
		if game.ShipType(p.NextShipType) == game.ShipStarbase {
			starbaseCounts := s.countStarbasesByTeam()
			// Exclude current player if they're already a starbase
			if p.Ship == game.ShipStarbase {
				starbaseCounts[p.Team]--
			}
			// Check if team would exceed starbase limit
			if starbaseCounts[p.Team] >= 1 {
				// Cancel the starbase refit, keep current ship type
				p.NextShipType = -1
				// Note: We could send a message here, but respawn doesn't have access to client
				// The player will be notified via game update that their refit was cancelled
			} else {
				// Safe to refit to starbase
				p.Ship = game.ShipType(p.NextShipType)
				p.NextShipType = -1
			}
		} else {
			// Non-starbase refit, always allowed
			p.Ship = game.ShipType(p.NextShipType)
			p.NextShipType = -1
		}
	} else {
		// No pending refit - preserve existing ship type
		// This is especially important for bots to maintain ship diversity
		p.Ship = currentShipType
	}

	// Reset ship stats
	shipStats := game.ShipData[p.Ship]
	p.Shields = shipStats.MaxShields
	p.Damage = 0
	p.Fuel = shipStats.MaxFuel
	p.WTemp = 0
	p.ETemp = 0
	p.Speed = 0
	p.DesSpeed = 0
	p.Shields_up = false // Shields DOWN by default when respawning
	p.Cloaked = false
	p.Tractoring = -1
	p.Pressoring = -1

	// Reset all action flags
	p.Repairing = false
	p.RepairRequest = false
	p.RepairCounter = 0
	p.Bombing = false
	p.Beaming = false
	p.BeamingUp = false
	p.Orbiting = -1
	p.Armies = 0 // Clear any armies being carried
	p.NumTorps = 0
	p.NumPlasma = 0

	// Reset engine overheat state
	p.EngineOverheat = false
	p.OverheatTimer = 0

	// Reset lock-on
	p.LockType = "none"
	p.LockTarget = -1

	// Reset fractional accumulators
	p.SubDir = 0  // Reset fractional turn accumulator
	p.AccFrac = 0 // Reset fractional acceleration accumulator

	// Set position near home planet with random offset (like original Netrek)
	// Original uses: pl->pl_x + (random() % 10000) - 5000
	var homeX, homeY float64
	switch p.Team {
	case game.TeamFed:
		homeX, homeY = 20000, 80000 // Earth
	case game.TeamRom:
		homeX, homeY = 20000, 20000 // Romulus
	case game.TeamKli:
		homeX, homeY = 80000, 20000 // Klingus
	case game.TeamOri:
		homeX, homeY = 80000, 80000 // Orion
	default:
		homeX, homeY = 50000, 50000 // Center if no team
	}

	// Add random offset between -5000 and +5000 for both X and Y
	offsetX := float64(rand.Intn(10000) - 5000)
	offsetY := float64(rand.Intn(10000) - 5000)
	p.X = homeX + offsetX
	p.Y = homeY + offsetY

	// Random starting direction
	p.Dir = float64(s.gameState.Frame%360) * 0.0174533 // Convert to radians
	p.DesDir = p.Dir

	// Start with green alert
	p.AlertLevel = "green"

}

// broadcastDeathMessage sends a death message to all players
func (s *Server) broadcastDeathMessage(victim *game.Player, killer *game.Player) {
	var msg string
	shipType := game.ShipData[victim.Ship].Name

	switch victim.WhyDead {
	case game.KillTorp:
		if killer != nil {
			msg = fmt.Sprintf("%s (%s) was destroyed by %s's torpedo",
				formatPlayerName(victim), shipType, formatPlayerName(killer))
		} else {
			msg = fmt.Sprintf("%s (%s) was destroyed by a torpedo",
				formatPlayerName(victim), shipType)
		}
	case game.KillPhaser:
		if killer != nil {
			msg = fmt.Sprintf("%s (%s) was killed by %s's phaser",
				formatPlayerName(victim), shipType, formatPlayerName(killer))
		} else {
			msg = fmt.Sprintf("%s (%s) was killed by a phaser",
				formatPlayerName(victim), shipType)
		}
	case game.KillExplosion:
		if killer != nil {
			msg = fmt.Sprintf("%s (%s) was killed by %s's explosion",
				formatPlayerName(victim), shipType, formatPlayerName(killer))
		} else {
			msg = fmt.Sprintf("%s (%s) was killed by explosion",
				formatPlayerName(victim), shipType)
		}
	default:
		msg = fmt.Sprintf("%s (%s) was destroyed", formatPlayerName(victim), shipType)
	}

	// Send death message to all clients
	messageData := map[string]interface{}{
		"text": msg,
		"type": "kill",
	}

	// Add killer's player ID if available (for team color)
	if killer != nil {
		messageData["from"] = killer.ID
	}

	s.broadcast <- ServerMessage{
		Type: "message",
		Data: messageData,
	}
}

// getTeamName returns the team name for display
func getTeamName(team int) string {
	switch team {
	case game.TeamFed:
		return "Federation"
	case game.TeamRom:
		return "Romulan"
	case game.TeamKli:
		return "Klingon"
	case game.TeamOri:
		return "Orion"
	default:
		return "Independent"
	}
}

// countStarbasesByTeam returns the number of starbases each team currently has
// Note: caller must hold the gameState mutex
func (s *Server) countStarbasesByTeam() map[int]int {
	starbaseCounts := map[int]int{
		game.TeamFed: 0,
		game.TeamRom: 0,
		game.TeamKli: 0,
		game.TeamOri: 0,
	}

	for _, p := range s.gameState.Players {
		// Count connected players (alive or dead) with starbase ship type
		// Include dead players because they might respawn as starbase
		if p.Connected && p.Ship == game.ShipStarbase && p.Status != game.StatusFree {
			starbaseCounts[p.Team]++
		}
	}

	return starbaseCounts
}

// broadcastTeamCounts sends current team counts to all connected clients
func (s *Server) broadcastTeamCounts() {
	s.gameState.Mu.RLock()

	// Count players per team (only count connected, alive players)
	teamCounts := map[string]int{
		"fed": 0,
		"rom": 0,
		"kli": 0,
		"ori": 0,
	}

	totalPlayers := 0
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Connected {
			totalPlayers++
			switch p.Team {
			case game.TeamFed:
				teamCounts["fed"]++
			case game.TeamRom:
				teamCounts["rom"]++
			case game.TeamKli:
				teamCounts["kli"]++
			case game.TeamOri:
				teamCounts["ori"]++
			}
		}
	}

	s.gameState.Mu.RUnlock()

	// Broadcast the update to all clients
	s.broadcast <- ServerMessage{
		Type: MsgTypeTeamUpdate,
		Data: map[string]interface{}{
			"total": totalPlayers,
			"teams": teamCounts,
		},
	}
}
