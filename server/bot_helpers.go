package server

import (
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

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
	minDist := MaxSearchDistance

	for i := range s.gameState.Players {
		other := s.gameState.Players[i]
		if other.Status == game.StatusAlive && other.Team != p.Team && i != p.ID && !other.Cloaked {
			dist := game.Distance(p.X, p.Y, other.X, other.Y)
			if dist < minDist {
				minDist = dist
				nearest = other
			}
		}
	}
	return nearest
}

// teamPlanetCounts is a reusable buffer for countTeamPlanets to avoid allocations.
// Indexed by team flag values (0=None, 1=Fed, 2=Rom, 4=Kli, 8=Ori).
// Max team flag is 8 (Ori), so we need 9 entries.
var teamPlanetCounts [9]int

// countTeamPlanets counts planets owned by each team.
// Returns a map for API compatibility. Uses a fixed-size array internally
// to avoid map allocation on every call (called multiple times per bot per tick).
func (s *Server) countTeamPlanets() map[int]int {
	// Clear the buffer
	for i := range teamPlanetCounts {
		teamPlanetCounts[i] = 0
	}
	for _, planet := range s.gameState.Planets {
		if planet.Owner >= 0 && planet.Owner < len(teamPlanetCounts) {
			teamPlanetCounts[planet.Owner]++
		}
	}
	// Return as map for compatibility with callers
	return map[int]int{
		game.TeamNone: teamPlanetCounts[game.TeamNone],
		game.TeamFed:  teamPlanetCounts[game.TeamFed],
		game.TeamRom:  teamPlanetCounts[game.TeamRom],
		game.TeamKli:  teamPlanetCounts[game.TeamKli],
		game.TeamOri:  teamPlanetCounts[game.TeamOri],
	}
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

	// Focus fire on damaged enemies (lower threshold for better coordination)
	if targetDamageRatio > 0.4 {
		return true
	}

	// Focus fire on carriers
	if target.Armies > 0 {
		return true
	}

	// Focus fire on high-value targets
	if target.Kills > 3 { // Lower threshold
		return true
	}

	// Focus on isolated targets
	isolated := true
	for _, enemy := range s.gameState.Players {
		if enemy.Status == game.StatusAlive && enemy.Team == target.Team && enemy.ID != target.ID {
			if game.Distance(target.X, target.Y, enemy.X, enemy.Y) < 5000 {
				isolated = false
				break
			}
		}
	}

	return isolated
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

	// Random from all combat ships for variety
	// Excludes Starbase (handled separately) and Galaxy (rare)
	combatShips := []int{
		int(game.ShipScout), int(game.ShipDestroyer), int(game.ShipCruiser),
		int(game.ShipBattleship), int(game.ShipAssault),
	}
	return combatShips[rand.Intn(len(combatShips))]
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
			if other.BotDefenseTarget >= 0 {
				defenders++
			} else if other.BotTarget >= 0 {
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

// selectBestCombatTarget selects the optimal combat target with persistence
func (s *Server) selectBestCombatTarget(p *game.Player) *game.Player {
	var bestTarget *game.Player
	bestScore := WorstScore
	var currentTargetScore float64

	// Check if we have a current target lock
	if p.BotTarget >= 0 && p.BotTargetLockTime > 0 {
		// Decay the lock timer
		p.BotTargetLockTime--

		// Check if current target is still valid
		if p.BotTarget < len(s.gameState.Players) {
			currentTarget := s.gameState.Players[p.BotTarget]
			if currentTarget.Status == game.StatusAlive && currentTarget.Team != p.Team {
				dist := game.Distance(p.X, p.Y, currentTarget.X, currentTarget.Y)
				if dist < 30000 { // Extended range for target persistence
					// Calculate current target's score with persistence bonus
					currentTargetScore = s.calculateTargetScore(p, currentTarget, dist)
					currentTargetScore += 3000 // Persistence bonus to prevent thrashing
					bestTarget = currentTarget
					bestScore = currentTargetScore
				}
			}
		}

		// Clear lock if target is invalid
		if bestTarget == nil {
			p.BotTargetLockTime = 0
			p.BotTarget = -1
		}
	}

	// Evaluate all potential targets
	for i, other := range s.gameState.Players {
		if other.Status != game.StatusAlive || other.Team == p.Team || i == p.ID {
			continue
		}

		dist := game.Distance(p.X, p.Y, other.X, other.Y)
		if dist > 25000 {
			continue // Too far
		}

		score := s.calculateTargetScore(p, other, dist)

		// Only switch targets if new target is significantly better
		if p.BotTargetLockTime > 0 && i == p.BotTarget {
			// This is our current target, already evaluated above
			continue
		}

		// Require 20% better score to switch targets when locked
		// Use absolute value to handle negative scores correctly
		if p.BotTargetLockTime > 0 {
			if score > bestScore+math.Abs(bestScore)*0.2 {
				bestScore = score
				bestTarget = other
			}
		} else {
			// No lock, switch to any better target
			if score > bestScore {
				bestScore = score
				bestTarget = other
			}
		}
	}

	// Update target lock
	if bestTarget != nil {
		if p.BotTarget != bestTarget.ID {
			// New target - establish lock
			p.BotTarget = bestTarget.ID
			p.BotTargetLockTime = 30 // 3 seconds at 10Hz
			p.BotTargetValue = bestScore
		} else if p.BotTargetLockTime < 10 {
			// Refresh lock on same target
			p.BotTargetLockTime = 10
		}
	}

	return bestTarget
}

// calculateTargetScore calculates the value score for a potential target
func (s *Server) calculateTargetScore(p *game.Player, target *game.Player, dist float64) float64 {
	// Multi-factor scoring
	score := 20000.0 / dist

	// Target prioritization
	targetStats := game.ShipData[target.Ship]
	damageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)

	// Prefer damaged enemies (higher bonus for nearly dead targets)
	if damageRatio > 0.8 {
		score += 8000 // Almost dead - high priority
	} else if damageRatio > 0.5 {
		score += damageRatio * 5000
	} else {
		score += damageRatio * 3000
	}

	// High priority: carriers
	if target.Armies > 0 {
		score += 10000 + float64(target.Armies)*1500
	}

	// Prefer enemies we can catch
	speedDiff := float64(game.ShipData[p.Ship].MaxSpeed - targetStats.MaxSpeed)
	if speedDiff > 0 {
		score += speedDiff * 300
	}

	// Avoid cloaked ships unless close
	if target.Cloaked {
		if dist > 2000 {
			score -= 6000
		} else {
			score += 2000 // Decloak them
		}
	}

	// Prefer isolated enemies
	isolated := true
	for _, ally := range s.gameState.Players {
		if ally.Status == game.StatusAlive && ally.Team == target.Team && ally.ID != target.ID {
			if game.Distance(target.X, target.Y, ally.X, ally.Y) < 5000 {
				isolated = false
				break
			}
		}
	}
	if isolated {
		score += 2000
	}

	return score
}

// coordinateTeamAttack checks if multiple allies are attacking the same target and coordinates timing
func (s *Server) coordinateTeamAttack(p *game.Player, target *game.Player) int {
	attackingAllies := 0
	totalCooldown := 0

	// Count allies attacking the same target
	for _, ally := range s.gameState.Players {
		if ally.Status == game.StatusAlive && ally.Team == p.Team && ally.ID != p.ID && ally.IsBot {
			// Check if ally is targeting the same enemy
			if ally.BotTarget == target.ID {
				attackingAllies++
				totalCooldown += ally.BotCooldown
			}
		}
	}

	// If multiple allies are attacking, synchronize cooldowns for volley fire
	if attackingAllies > 0 {
		// Average cooldown for synchronized firing
		return totalCooldown / (attackingAllies + 1)
	}

	return -1 // No coordination needed
}

// broadcastTargetToAllies shares high-value target information with nearby allies.
// Only suggests targets to allies that have no current target, to avoid overwriting
// targeting decisions that other bots already made this tick.
func (s *Server) broadcastTargetToAllies(p *game.Player, target *game.Player, targetValue float64) {
	// Only broadcast for high-value targets (carriers, heavily damaged ships)
	if targetValue <= 15000 && target.Armies <= 0 {
		return
	}

	for _, ally := range s.gameState.Players {
		if ally.Status != game.StatusAlive || ally.Team != p.Team || ally.ID == p.ID || !ally.IsBot {
			continue
		}

		dist := game.Distance(p.X, p.Y, ally.X, ally.Y)
		if dist >= 15000 {
			continue
		}

		// Only suggest to allies with no current target — never overwrite an existing lock
		if ally.BotTarget < 0 {
			ally.BotTarget = target.ID
			ally.BotTargetLockTime = 20
			ally.BotTargetValue = targetValue
		}
	}
}
