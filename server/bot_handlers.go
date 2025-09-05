package server

import (
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"math/rand"
	"strings"
	"time"
)

// handleBotCommand processes bot-related slash commands
func (c *Client) handleBotCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/addbot":
		// /addbot [team] [ship_type]
		team := game.TeamFed
		ship := 1 // Destroyer

		if len(parts) > 1 {
			switch parts[1] {
			case "fed":
				team = game.TeamFed
			case "rom":
				team = game.TeamRom
			case "kli":
				team = game.TeamKli
			case "ori":
				team = game.TeamOri
			}
		}

		if len(parts) > 2 {
			// Parse ship alias (SC, DD, CA, etc.) - consistent with /refit
			shipTypeStr := strings.ToUpper(parts[2])
			if shipTypeInt, ok := shipAlias[shipTypeStr]; ok {
				ship = shipTypeInt
			} else {
				// Invalid ship type - send error message and return
				c.send <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": "Invalid ship type. Usage: /addbot [fed/rom/kli/ori] [SC|DD|CA|BB|AS|SB|GA]",
						"type": "warning",
					},
				}
				return
			}
		}

		// Level parameter is ignored - all bots are hard mode now
		if len(parts) > 3 {
			// Silently ignore the level parameter for backward compatibility
		}

		// Check starbase limit before adding bot
		if ship == int(game.ShipStarbase) {
			c.server.gameState.Mu.Lock()
			starbaseCounts := c.server.countStarbasesByTeam()
			if starbaseCounts[team] >= 1 {
				c.server.gameState.Mu.Unlock()
				c.send <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": "Cannot add starbase bot - team already has a starbase. Only one starbase per team is allowed.",
						"type": "warning",
					},
				}
				return
			}
			c.server.gameState.Mu.Unlock()
		}

		c.server.AddBot(team, ship)

	case "/removebot":
		// Remove a random bot
		for i, p := range c.server.gameState.Players {
			if p.IsBot && p.Connected {
				c.server.RemoveBot(i)
				break
			}
		}

	case "/balance":
		// Auto-balance teams with bots
		c.server.AutoBalanceBots()

	case "/clearbots":
		// Remove all bots
		for i, p := range c.server.gameState.Players {
			if p.IsBot && p.Connected {
				c.server.RemoveBot(i)
			}
		}

	case "/fillbots":
		// Fill available slots with bots of every ship type
		// All bots use hard difficulty mode
		// Only allow one starbase per team

		c.server.gameState.Mu.Lock()

		// All ship types including starbase (but starbase is limited)
		allShipTypes := []int{
			int(game.ShipScout),      // 0
			int(game.ShipDestroyer),  // 1
			int(game.ShipCruiser),    // 2
			int(game.ShipBattleship), // 3
			int(game.ShipAssault),    // 4
			int(game.ShipStarbase),   // 5
			int(game.ShipGalaxy),     // 6
		}

		teams := []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri}

		botsAdded := 0
		maxBots := game.MaxPlayers - 4 // Leave room for humans

		// Keep track of how many of each ship type we've added per team
		// This helps ensure we get variety
		teamShipCounts := make(map[int]map[int]int)
		for _, team := range teams {
			teamShipCounts[team] = make(map[int]int)
		}

		// Count existing bots
		currentBots := 0
		for _, p := range c.server.gameState.Players {
			if p.IsBot && p.Status != game.StatusFree {
				currentBots++
			}
		}

		c.server.gameState.Mu.Unlock()

		// Add bots using balanced selection to ensure ship type diversity
		// Use a round-robin approach that interleaves teams and ship types
		rand.Seed(time.Now().UnixNano())
		consecutiveFailures := 0
		maxConsecutiveFailures := len(teams) * len(allShipTypes) * 3 // Safety limit

		for currentBots+botsAdded < maxBots && consecutiveFailures < maxConsecutiveFailures {
			// Select team and ship type to promote diversity
			// Find the team with the fewest bots
			var selectedTeam int
			minTeamBots := 999
			for _, team := range teams {
				totalForTeam := 0
				for _, count := range teamShipCounts[team] {
					totalForTeam += count
				}
				if totalForTeam < minTeamBots {
					minTeamBots = totalForTeam
					selectedTeam = team
				}
			}

			// For the selected team, find ship type with lowest count
			// But add some randomness to avoid pure determinism
			var candidateShips []int
			minShipCount := 999

			// First pass: find minimum count for this team
			for _, shipType := range allShipTypes {
				// Skip starbase if team already has one
				if shipType == int(game.ShipStarbase) {
					c.server.gameState.Mu.RLock()
					currentStarbaseCount := c.server.countStarbasesByTeam()
					c.server.gameState.Mu.RUnlock()
					if currentStarbaseCount[selectedTeam] >= 1 {
						continue
					}
				}

				count := teamShipCounts[selectedTeam][shipType]
				if count < minShipCount {
					minShipCount = count
				}
			}

			// Second pass: collect all ship types with minimum count
			for _, shipType := range allShipTypes {
				// Skip starbase if team already has one
				if shipType == int(game.ShipStarbase) {
					c.server.gameState.Mu.RLock()
					currentStarbaseCount := c.server.countStarbasesByTeam()
					c.server.gameState.Mu.RUnlock()
					if currentStarbaseCount[selectedTeam] >= 1 {
						continue
					}
				}

				if teamShipCounts[selectedTeam][shipType] == minShipCount {
					candidateShips = append(candidateShips, shipType)
				}
			}

			// If no candidates (shouldn't happen), break
			if len(candidateShips) == 0 {
				consecutiveFailures++
				continue
			}

			// Randomly select from candidates to add variety
			selectedShipType := candidateShips[rand.Intn(len(candidateShips))]

			// Try to add the bot
			c.server.AddBot(selectedTeam, selectedShipType)

			// Check if bot was actually added
			c.server.gameState.Mu.RLock()
			newBotCount := 0
			for _, p := range c.server.gameState.Players {
				if p.IsBot && p.Status != game.StatusFree {
					newBotCount++
				}
			}
			c.server.gameState.Mu.RUnlock()

			if newBotCount > currentBots+botsAdded {
				// Bot was successfully added
				botsAdded++
				teamShipCounts[selectedTeam][selectedShipType]++
				consecutiveFailures = 0
			} else {
				// Bot was not added (probably no free slots)
				consecutiveFailures++
			}
		}

		// Send confirmation message with details
		if botsAdded > 0 {
			c.send <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("Added %d bots with diverse ship types (1 starbase max per team)", botsAdded),
					"type": "info",
				},
			}
		} else {
			c.send <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "No bots were added - server may be full or all ship types already present",
					"type": "warning",
				},
			}
		}

	case "/refit":
		// /refit [ship_type]
		if len(parts) < 2 {
			c.send <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "Usage: /refit SC|DD|CA|BB|AS|SB|GA",
					"type": "warning",
				},
			}
			return
		}

		// Get the ship type from alias (case insensitive)
		shipTypeStr := strings.ToUpper(parts[1])
		shipTypeInt, ok := shipAlias[shipTypeStr]
		if !ok {
			c.send <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "Invalid ship type. Usage: /refit SC|DD|CA|BB|AS|SB|GA",
					"type": "warning",
				},
			}
			return
		}

		// Check starbase limit before allowing refit (each team can have at most 1 starbase)
		c.server.gameState.Mu.Lock()
		p := c.server.gameState.Players[c.PlayerID]
		if shipTypeInt == int(game.ShipStarbase) {
			// Count existing starbases, but exclude this player in case they're already a starbase
			starbaseCounts := c.server.countStarbasesByTeam()
			if p.Ship == game.ShipStarbase {
				// If this player is currently a starbase, subtract 1 from the count
				starbaseCounts[p.Team]--
			}
			if starbaseCounts[p.Team] >= 1 {
				c.server.gameState.Mu.Unlock()
				c.send <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": "Your team already has a starbase. Only one starbase per team is allowed.",
						"type": "warning",
					},
				}
				return
			}
		}

		// Set the next ship type for this player
		p.NextShipType = shipTypeInt
		c.server.gameState.Mu.Unlock()

		// Get the ship name for the confirmation message
		shipName := game.ShipData[game.ShipType(shipTypeInt)].Name

		// Send confirmation message
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("Refit to %s when you next respawn.", shipName),
				"type": "info",
			},
		}

	case "/help":
		// Send help message
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Bot commands: /addbot [fed/rom/kli/ori] [SC|DD|CA|BB|AS|SB|GA] | /removebot | /balance | /clearbots | /fillbots | /refit SC|DD|CA|BB|AS|SB|GA",
				"type": "info",
			},
		}

	default:
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Unknown command. Type /help for bot commands.",
				"type": "warning",
			},
		}
	}
}
