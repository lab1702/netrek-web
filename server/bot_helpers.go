package server

import (
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// randomJitterRad returns a random angle for torpedo firing jitter
// This function is already defined in bot_jitter.go, but we may need to use it here
// func randomJitterRad() float64 {
//     // Implementation is in bot_jitter.go
// }

// getVelocityAdjustedTorpRange calculates torpedo range adjusted for target velocity
// to prevent fuse expiry on fast-moving targets
func (s *Server) getVelocityAdjustedTorpRange(p *game.Player, target *game.Player) float64 {
	shipStats := game.ShipData[p.Ship]
	baseRange := float64(game.EffectiveTorpRangeForShip(p.Ship, shipStats))

	// Calculate target's speed as a fraction of maximum possible speed
	targetSpeed := target.Speed
	maxPossibleSpeed := 12.0 // Scout's max speed (highest in game)
	speedRatio := targetSpeed / maxPossibleSpeed

	// Reduce firing range for faster targets to ensure torpedo hits before fuse expiry
	// Fast targets (>75% max speed) get 10% range reduction
	// Very fast targets (>90% max speed) get 20% range reduction
	if speedRatio > 0.9 {
		return baseRange * 0.8 // 20% reduction for very fast targets
	} else if speedRatio > 0.75 {
		return baseRange * 0.9 // 10% reduction for fast targets
	}

	// For slower targets, use full effective range
	return baseRange
}

// getBotArmyCapacity returns army carrying capacity
func (s *Server) getBotArmyCapacity(p *game.Player) int {
	switch p.Ship {
	case game.ShipAssault:
		if p.Kills > 3 {
			return 6
		}
		return int(p.Kills) * 3
	case game.ShipStarbase:
		return 25
	default:
		if p.Kills > 2 {
			return 4
		}
		return int(p.Kills) * 2
	}
}

// findNearestEnemy finds the closest enemy player
func (s *Server) findNearestEnemy(p *game.Player) *game.Player {
	var nearest *game.Player
	minDist := 999999.0

	for i := range s.gameState.Players {
		other := s.gameState.Players[i]
		if other.Status == game.StatusAlive && other.Team != p.Team && i != p.ID {
			dist := game.Distance(p.X, p.Y, other.X, other.Y)
			if dist < minDist {
				minDist = dist
				nearest = other
			}
		}
	}
	return nearest
}

// countTeamPlanets counts planets owned by each team
func (s *Server) countTeamPlanets() map[int]int {
	counts := make(map[int]int)
	for _, planet := range s.gameState.Planets {
		counts[planet.Owner]++
	}
	return counts
}

// countPlanetsForTeam counts how many planets a team owns
func (s *Server) countPlanetsForTeam(team int) int {
	count := 0
	for _, planet := range s.gameState.Planets {
		if planet.Owner == team {
			count++
		}
	}
	return count
}

// calculateTeamCenter calculates the center of a team's territory
func (s *Server) calculateTeamCenter(team int) (float64, float64) {
	totalX, totalY := 0.0, 0.0
	count := 0

	for _, planet := range s.gameState.Planets {
		if planet.Owner == team {
			totalX += planet.X
			totalY += planet.Y
			count++
		}
	}

	if count > 0 {
		return totalX / float64(count), totalY / float64(count)
	}

	// Fallback to team home if no planets owned
	return float64(game.TeamHomeX[team]), float64(game.TeamHomeY[team])
}

// isCorePlanet checks if a planet is a core/home planet for a team
func (s *Server) isCorePlanet(planet *game.Planet, team int) bool {
	// Check if planet is close to team's home coordinates
	homeX := float64(game.TeamHomeX[team])
	homeY := float64(game.TeamHomeY[team])
	dist := game.Distance(planet.X, planet.Y, homeX, homeY)
	return dist < 25000 // Within 25k of home
}

// findNearbyAlly finds the nearest allied bot for coordination
func (s *Server) findNearbyAlly(p *game.Player, maxDist float64) *game.Player {
	var nearest *game.Player
	minDist := maxDist

	for i := range s.gameState.Players {
		other := s.gameState.Players[i]
		if other.Status == game.StatusAlive && other.Team == p.Team &&
			i != p.ID && other.IsBot {
			dist := game.Distance(p.X, p.Y, other.X, other.Y)
			if dist < minDist {
				minDist = dist
				nearest = other
			}
		}
	}

	return nearest
}

// shouldFocusFire determines if bots should focus fire on a target
func (s *Server) shouldFocusFire(p, ally, target *game.Player) bool {
	targetStats := game.ShipData[target.Ship]
	targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)

	// Focus fire on damaged enemies
	if targetDamageRatio > 0.5 {
		return true
	}

	// Focus fire on carriers
	if target.Armies > 0 {
		return true
	}

	// Focus fire on high-value targets
	if target.Kills > 5 {
		return true
	}

	return false
}

// selectBotShipType chooses appropriate ship type based on team composition
func (s *Server) selectBotShipType(team int) int {
	// Count existing ship types on team
	shipCounts := make(map[game.ShipType]int)
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Team == team {
			shipCounts[p.Ship]++
		}
	}

	// Balanced composition strategy
	total := 0
	for _, count := range shipCounts {
		total += count
	}

	if total == 0 {
		// First bot - random from main combat ships (avoid too many scouts)
		options := []int{int(game.ShipDestroyer), int(game.ShipCruiser), int(game.ShipBattleship), int(game.ShipAssault)}
		return options[rand.Intn(len(options))]
	}

	// Prefer destroyers and cruisers for balance
	if shipCounts[game.ShipDestroyer] < 2 {
		return int(game.ShipDestroyer)
	}
	if shipCounts[game.ShipCruiser] < 2 {
		return int(game.ShipCruiser)
	}

	// Add assault ship if none exists
	if shipCounts[game.ShipAssault] == 0 && total > 3 {
		return int(game.ShipAssault)
	}

	// Random from main combat ships for variety (includes Scout, Destroyer, Cruiser, Battleship, Assault)
	// Note: This excludes Starbase (handled separately) and Galaxy (rare)
	return rand.Intn(5) // 0-4: Scout, Destroyer, Cruiser, Battleship, Assault
}

// selectBotBehavior determines bot behavior based on game state
func (s *Server) selectBotBehavior(p *game.Player) string {
	// Analyze game state
	teamPlanets := s.countTeamPlanets()
	totalPlanets := len(s.gameState.Planets)
	controlRatio := float64(teamPlanets[p.Team]) / float64(totalPlanets)

	// Count team composition
	hunters := 0
	defenders := 0
	for _, other := range s.gameState.Players {
		if other.Status == game.StatusAlive && other.Team == p.Team && other.IsBot {
			if other.BotTarget >= 0 {
				hunters++
			}
		}
	}

	// Dynamic role assignment
	if controlRatio < 0.2 {
		// Losing badly - focus on defense and raids
		if defenders < 2 {
			return "defender"
		}
		return "raider"
	} else if controlRatio > 0.6 {
		// Winning - aggressive hunting
		return "hunter"
	} else {
		// Balanced - mixed strategy
		if hunters > defenders+1 {
			return "defender"
		} else if p.KillsStreak >= game.ArmyKillRequirement {
			return "raider"
		}
		return "hunter"
	}
}

// selectBestCombatTarget selects the optimal combat target
func (s *Server) selectBestCombatTarget(p *game.Player) *game.Player {
	var bestTarget *game.Player
	bestScore := -999999.0

	for i, other := range s.gameState.Players {
		if other.Status != game.StatusAlive || other.Team == p.Team || i == p.ID {
			continue
		}

		dist := game.Distance(p.X, p.Y, other.X, other.Y)
		if dist > 25000 {
			continue // Too far
		}

		// Multi-factor scoring
		score := 20000.0 / dist

		// Target prioritization
		otherStats := game.ShipData[other.Ship]
		damageRatio := float64(other.Damage) / float64(otherStats.MaxDamage)

		// Prefer damaged enemies
		score += damageRatio * 4000

		// High priority: carriers
		if other.Armies > 0 {
			score += 10000 + float64(other.Armies)*1000
		}

		// Prefer enemies we can catch
		speedDiff := float64(game.ShipData[p.Ship].MaxSpeed - otherStats.MaxSpeed)
		if speedDiff > 0 {
			score += speedDiff * 200
		}

		// Avoid cloaked ships unless close
		if other.Cloaked {
			if dist > 2000 {
				score -= 6000
			} else {
				score += 2000 // Decloak them
			}
		}

		// Prefer isolated enemies
		isolated := true
		for _, ally := range s.gameState.Players {
			if ally.Status == game.StatusAlive && ally.Team == other.Team && ally.ID != other.ID {
				if game.Distance(other.X, other.Y, ally.X, ally.Y) < 5000 {
					isolated = false
					break
				}
			}
		}
		if isolated {
			score += 1500
		}

		if score > bestScore {
			bestScore = score
			bestTarget = other
		}
	}

	return bestTarget
}
