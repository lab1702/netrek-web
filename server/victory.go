package server

import (
	"fmt"
	"time"

	"github.com/lab1702/netrek-web/game"
)

// checkVictoryConditions checks for genocide or conquest victory
func (s *Server) checkVictoryConditions() {
	if s.gameState.GameOver {
		return // Game already over
	}

	// Count active players and planets per team
	for i := range s.gameState.TeamPlayers {
		s.gameState.TeamPlayers[i] = 0
		s.gameState.TeamPlanets[i] = 0
	}

	// Count active players per team
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive {
			switch p.Team {
			case game.TeamFed:
				s.gameState.TeamPlayers[0]++
			case game.TeamRom:
				s.gameState.TeamPlayers[1]++
			case game.TeamKli:
				s.gameState.TeamPlayers[2]++
			case game.TeamOri:
				s.gameState.TeamPlayers[3]++
			}
		}
	}

	// Count planets per team
	for _, planet := range s.gameState.Planets {
		switch planet.Owner {
		case game.TeamFed:
			s.gameState.TeamPlanets[0]++
		case game.TeamRom:
			s.gameState.TeamPlanets[1]++
		case game.TeamKli:
			s.gameState.TeamPlanets[2]++
		case game.TeamOri:
			s.gameState.TeamPlanets[3]++
		}
	}

	// Check for genocide (all players of other teams eliminated)
	// But require that multiple teams were playing (had players at some point)
	totalPlayers := 0
	teamsAlive := 0
	lastTeamAlive := -1
	teamsEverPlayed := 0 // Track how many teams have ever had players

	// Check current players
	for i, count := range s.gameState.TeamPlayers {
		totalPlayers += count
		if count > 0 {
			teamsAlive++
			lastTeamAlive = 1 << i // Convert to team flag (1, 2, 4, 8)
		}
	}

	// Count how many teams have ever had players in this game
	for _, p := range s.gameState.Players {
		if p.Status != game.StatusFree && p.Team > 0 {
			// This player slot was used by a team
			switch p.Team {
			case game.TeamFed:
				teamsEverPlayed |= 1
			case game.TeamRom:
				teamsEverPlayed |= 2
			case game.TeamKli:
				teamsEverPlayed |= 4
			case game.TeamOri:
				teamsEverPlayed |= 8
			}
		}
	}

	// Count bits set in teamsEverPlayed to get number of teams that played
	numTeamsPlayed := 0
	for i := 0; i < 4; i++ {
		if (teamsEverPlayed>>i)&1 == 1 {
			numTeamsPlayed++
		}
	}

	// Only check for genocide if:
	// - At least 2 different teams have played
	// - Game has been running for a bit
	// - Only one team remains alive
	// - At least 2 total players currently
	if numTeamsPlayed >= 2 && totalPlayers >= 2 && s.gameState.Frame > 100 && teamsAlive == 1 && lastTeamAlive > 0 {
		// Genocide victory
		s.gameState.GameOver = true
		s.gameState.Winner = lastTeamAlive
		s.gameState.WinType = "genocide"
		s.announceVictory()
		return
	}

	// Check for conquest (one team owns all planets)
	// Also require multiple players for conquest victory
	if totalPlayers >= 2 && s.gameState.Frame > 100 {
		for i, count := range s.gameState.TeamPlanets {
			if count == game.MaxPlanets {
				// Conquest victory
				s.gameState.GameOver = true
				s.gameState.Winner = 1 << i // Convert to team flag
				s.gameState.WinType = "conquest"
				s.announceVictory()
				return
			}
		}
	}

	// Check for domination victory (one team owns all planets that are owned,
	// and no enemy players are carrying armies to retake independent planets)
	if totalPlayers >= 2 && s.gameState.Frame > 100 {
		// First check if only one team owns planets
		teamsOwningPlanets := 0
		teamWithPlanets := -1
		independentPlanets := 0

		for _, planet := range s.gameState.Planets {
			if planet.Owner == game.TeamNone {
				independentPlanets++
			}
		}

		for i, count := range s.gameState.TeamPlanets {
			if count > 0 {
				teamsOwningPlanets++
				teamWithPlanets = i
			}
		}

		// If only one team owns planets and there are independent planets
		if teamsOwningPlanets == 1 && teamWithPlanets >= 0 && independentPlanets > 0 {
			// Check if any enemy players are carrying armies
			enemyHasArmies := false
			dominantTeam := 1 << teamWithPlanets

			for _, p := range s.gameState.Players {
				// Check if player is alive, on a different team, and carrying armies
				if p.Status == game.StatusAlive && p.Team != dominantTeam && p.Armies > 0 {
					enemyHasArmies = true
					break
				}
			}

			// If no enemies have armies, the dominant team wins
			if !enemyHasArmies {
				s.gameState.GameOver = true
				s.gameState.Winner = dominantTeam
				s.gameState.WinType = "domination"
				s.announceVictory()
				return
			}
		}
	}
}

// getTeamNamesFromFlag converts a combined team flag to a slice of team names
func getTeamNamesFromFlag(teamFlag int) []string {
	var names []string
	if teamFlag&game.TeamFed != 0 {
		names = append(names, "Federation")
	}
	if teamFlag&game.TeamRom != 0 {
		names = append(names, "Romulan")
	}
	if teamFlag&game.TeamKli != 0 {
		names = append(names, "Klingon")
	}
	if teamFlag&game.TeamOri != 0 {
		names = append(names, "Orion")
	}
	return names
}

// formatTeamNames formats a list of team names for display
func formatTeamNames(names []string) string {
	if len(names) == 0 {
		return "No Teams" // More descriptive than "Unknown"
	}
	if len(names) == 1 {
		return names[0]
	}
	if len(names) == 2 {
		return names[0] + " & " + names[1]
	}
	// For 3+ teams, use commas with final "&"
	result := ""
	for i, name := range names {
		if i == len(names)-1 {
			result += " & " + name
		} else if i > 0 {
			result += ", " + name
		} else {
			result = name
		}
	}
	return result
}

// announceVictory sends victory message to all clients
func (s *Server) announceVictory() {
	teamNames := getTeamNamesFromFlag(s.gameState.Winner)
	teamNameStr := formatTeamNames(teamNames)

	var message string
	if s.gameState.WinType == "genocide" {
		if len(teamNames) > 1 {
			message = fmt.Sprintf("🎉 GENOCIDE! %s teams have eliminated all enemies! Shared victory!", teamNameStr)
		} else {
			message = fmt.Sprintf("🎉 GENOCIDE! %s team has eliminated all enemies! Victory!", teamNameStr)
		}
	} else if s.gameState.WinType == "conquest" {
		if len(teamNames) > 1 {
			message = fmt.Sprintf("🎉 CONQUEST! %s teams have captured all planets! Shared victory!", teamNameStr)
		} else {
			message = fmt.Sprintf("🎉 CONQUEST! %s team has captured all planets! Victory!", teamNameStr)
		}
	} else if s.gameState.WinType == "domination" {
		if len(teamNames) > 1 {
			message = fmt.Sprintf("🏆 DOMINATION! %s teams control all owned planets and enemies have no armies! Shared victory!", teamNameStr)
		} else {
			message = fmt.Sprintf("🏆 DOMINATION! %s team controls all owned planets and enemies have no armies! Victory!", teamNameStr)
		}
	} else if s.gameState.WinType == "timeout" {
		if len(teamNames) > 1 {
			message = fmt.Sprintf("⏱️ TIME LIMIT! %s teams share victory by controlling the most planets!", teamNameStr)
		} else {
			message = fmt.Sprintf("⏱️ TIME LIMIT! %s team wins by controlling the most planets!", teamNameStr)
		}
	}

	// Broadcast victory message
	select {
	case s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text":     message,
			"type":     "victory",
			"winner":   s.gameState.Winner,
			"win_type": s.gameState.WinType,
		},
	}:
	default:
	}

	// Schedule game reset after 10 seconds, respecting server shutdown
	go func() {
		select {
		case <-time.After(10 * time.Second):
			s.resetGame()
		case <-s.done:
			// Server shutting down, skip reset
		}
	}()
}

// resetGame resets the game state for a new round
func (s *Server) resetGame() {
	// Lock ordering: s.mu first, then s.gameState.Mu
	s.mu.Lock()
	// Reset all connected clients back to lobby (no player slot assigned)
	for _, client := range s.clients {
		client.PlayerID = -1 // Back to lobby - no slot assigned
	}
	s.mu.Unlock()

	// Reset game state in-place (do not replace the pointer)
	s.gameState.Mu.Lock()

	// Reset all player slots
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		*p = game.Player{
			ID:                  i,
			Status:              game.StatusFree,
			Tractoring:          -1,
			Pressoring:          -1,
			Orbiting:            -1,
			LockType:            "none",
			LockTarget:          -1,
			BotDefenseTarget:    -1,
			BotPlanetApproachID: -1,
			BotTarget:           -1,
			NextShipType:        -1,
		}
	}

	// Re-initialize planets
	game.InitPlanets(s.gameState)
	game.InitINLPlanetFlags(s.gameState)

	// Reset game-level state
	s.gameState.Frame = 0
	s.gameState.TickCount = 0
	s.gameState.T_mode = false
	s.gameState.T_start = 0
	s.gameState.T_remain = 0
	s.gameState.GameOver = false
	s.gameState.Winner = 0
	s.gameState.WinType = ""
	s.gameState.Torps = make([]*game.Torpedo, 0)
	s.gameState.Plasmas = make([]*game.Plasma, 0)
	s.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)
	for i := range s.gameState.TeamPlayers {
		s.gameState.TeamPlayers[i] = 0
		s.gameState.TeamPlanets[i] = 0
	}

	s.gameState.Mu.Unlock()

	// Announce game reset
	select {
	case s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": "🔄 Game reset! All players returned to lobby. Choose team & ship again.",
			"type": "info",
		},
	}:
	default:
	}
}
