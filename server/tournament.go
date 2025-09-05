package server

import (
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// checkTournamentMode checks if tournament mode should be active
func (s *Server) checkTournamentMode() {
	// Count players per team
	teamCounts := make(map[int]int)
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Connected {
			teamCounts[p.Team]++
		}
	}

	// Check if we have at least 4v4 (minimum 4 players on at least 2 teams)
	teamsWithEnough := 0
	for _, count := range teamCounts {
		if count >= 4 {
			teamsWithEnough++
		}
	}

	wasInTMode := s.gameState.T_mode
	shouldBeInTMode := teamsWithEnough >= 2

	if !wasInTMode && shouldBeInTMode {
		// Entering tournament mode
		s.gameState.T_mode = true
		s.gameState.T_start = s.gameState.Frame
		s.gameState.T_remain = 1800 // 30 minutes in seconds

		// Reset galaxy to ensure fair start
		// Re-initialize planets to startup state
		game.InitPlanets(s.gameState)
		game.InitINLPlanetFlags(s.gameState)

		// Clear all torpedoes and plasmas for clean start
		s.gameState.Torps = make([]*game.Torpedo, 0)
		s.gameState.Plasmas = make([]*game.Plasma, 0)

		// Reset all active players to spawn positions
		for i := range s.gameState.Players {
			p := s.gameState.Players[i]
			if p.Status == game.StatusAlive && p.Connected {
				// Initialize tournament stats
				s.gameState.TournamentStats[p.ID] = &game.TournamentPlayerStats{}

				// Reset ship state
				shipStats := game.ShipData[p.Ship]
				p.Shields = shipStats.MaxShields
				p.Damage = 0
				p.Fuel = shipStats.MaxFuel
				p.WTemp = 0
				p.ETemp = 0
				p.Speed = 0
				p.DesSpeed = 0
				p.SubDir = 0  // Reset fractional turn accumulator
				p.AccFrac = 0 // Reset fractional acceleration accumulator

				// Reset kills and deaths for fair tournament start
				p.Kills = 0
				p.KillsStreak = 0
				p.Deaths = 0
				p.Shields_up = false
				p.Cloaked = false
				p.Tractoring = -1
				p.Pressoring = -1
				p.Orbiting = -1
				p.Bombing = false
				p.Beaming = false
				p.BeamingUp = false
				p.Repairing = false
				p.RepairRequest = false
				p.RepairCounter = 0
				p.EngineOverheat = false
				p.OverheatTimer = 0
				p.Armies = 0 // Clear any armies being carried
				p.NumTorps = 0
				p.NumPlasma = 0

				// Reset lock-on
				p.LockType = "none"
				p.LockTarget = -1

				// Reset death tracking (in case they were exploding)
				p.ExplodeTimer = 0
				p.KilledBy = -1
				p.WhyDead = 0

				// Reset position to near home world
				homeX := float64(game.TeamHomeX[p.Team])
				homeY := float64(game.TeamHomeY[p.Team])

				// Add random offset to prevent ships spawning on top of each other
				offsetX := float64(rand.Intn(10000) - 5000)
				offsetY := float64(rand.Intn(10000) - 5000)
				p.X = homeX + offsetX
				p.Y = homeY + offsetY

				// Random starting direction
				p.Dir = rand.Float64() * 2 * math.Pi
				p.DesDir = p.Dir

				// Reset alert level
				p.AlertLevel = "green"

				// Clear bot-specific state
				if p.IsBot {
					p.BotTarget = -1
					p.BotCooldown = 0
					p.BotGoalX = 0
					p.BotGoalY = 0
				}
			}
		}

		// Announce T-mode
		s.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "⚔️ TOURNAMENT MODE ACTIVATED! 4v4 minimum reached. 30 minute time limit. Galaxy and all ships reset for fair start!",
				"type": "info",
			},
		}
	} else if wasInTMode && !shouldBeInTMode {
		// Leaving tournament mode
		s.gameState.T_mode = false

		// Announce T-mode end
		s.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Tournament mode deactivated - not enough players",
				"type": "info",
			},
		}
	}

	// Update tournament timer if in T-mode
	if s.gameState.T_mode {
		elapsedFrames := s.gameState.Frame - s.gameState.T_start
		elapsedSeconds := elapsedFrames / 10 // 10 ticks per second
		s.gameState.T_remain = 1800 - int(elapsedSeconds)

		// Check for time limit
		if s.gameState.T_remain <= 0 && !s.gameState.GameOver {
			// Time's up - determine winner by planets owned
			maxPlanets := 0
			winningTeam := 0
			for i, count := range s.gameState.TeamPlanets {
				if count > maxPlanets {
					maxPlanets = count
					winningTeam = 1 << i
				}
			}

			if winningTeam > 0 {
				s.gameState.GameOver = true
				s.gameState.Winner = winningTeam
				s.gameState.WinType = "timeout"
				s.announceVictory()
			}
		}

		// Announce time warnings
		if s.gameState.T_remain == 600 && s.gameState.Frame%10 == 0 { // 10 minutes
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 10 minutes remaining in tournament!",
					"type": "warning",
				},
			}
		} else if s.gameState.T_remain == 300 && s.gameState.Frame%10 == 0 { // 5 minutes
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 5 minutes remaining in tournament!",
					"type": "warning",
				},
			}
		} else if s.gameState.T_remain == 60 && s.gameState.Frame%10 == 0 { // 1 minute
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 1 minute remaining in tournament!",
					"type": "warning",
				},
			}
		}
	}
}
