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

// announceVictory sends victory message to all clients
func (s *Server) announceVictory() {
	teamName := ""
	switch s.gameState.Winner {
	case game.TeamFed:
		teamName = "Federation"
	case game.TeamRom:
		teamName = "Romulan"
	case game.TeamKli:
		teamName = "Klingon"
	case game.TeamOri:
		teamName = "Orion"
	}

	var message string
	if s.gameState.WinType == "genocide" {
		message = fmt.Sprintf("🎉 GENOCIDE! %s team has eliminated all enemies! Victory!", teamName)
	} else if s.gameState.WinType == "conquest" {
		message = fmt.Sprintf("🎉 CONQUEST! %s team has captured all planets! Victory!", teamName)
	} else if s.gameState.WinType == "domination" {
		message = fmt.Sprintf("🏆 DOMINATION! %s team controls all owned planets and enemies have no armies! Victory!", teamName)
	} else if s.gameState.WinType == "timeout" {
		message = fmt.Sprintf("⏱️ TIME LIMIT! %s team wins by controlling the most planets!", teamName)
	}

	// Broadcast victory message
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text":     message,
			"type":     "victory",
			"winner":   s.gameState.Winner,
			"win_type": s.gameState.WinType,
		},
	}

	// Schedule game reset after 10 seconds
	go func() {
		time.Sleep(10 * time.Second)
		s.resetGame()
	}()
}

// resetGame resets the game state for a new round
func (s *Server) resetGame() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create new game state
	newState := game.NewGameState()

	// Preserve connected players but reset their status
	for i, p := range s.gameState.Players {
		if p.Connected {
			// For bots, disconnect them when game resets
			if p.IsBot {
				// Mark bot as disconnected so slot becomes free
				newState.Players[i].Status = game.StatusFree
				newState.Players[i].Connected = false
				newState.Players[i].IsBot = false
			} else {
				// For human players, preserve connection
				newState.Players[i] = &game.Player{
					ID:         i,
					Name:       p.Name,
					Team:       p.Team,
					Ship:       p.Ship,
					Status:     game.StatusOutfit,
					Connected:  true,
					Tractoring: -1,
					Pressoring: -1,
					Orbiting:   -1,
				}
				// Set initial position and stats
				shipStats := game.ShipData[p.Ship]
				newState.Players[i].X = float64(game.TeamHomeX[p.Team]) + float64(i%4)*1000
				newState.Players[i].Y = float64(game.TeamHomeY[p.Team]) + float64(i/4)*1000
				newState.Players[i].Shields = shipStats.MaxShields
				newState.Players[i].Fuel = shipStats.MaxFuel
			}
		}
	}

	s.gameState = newState

	// Announce game reset
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": "Game has been reset! New round starting...",
			"type": "info",
		},
	}
}
