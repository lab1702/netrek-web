package server

import (
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// getVelocityAdjustedTorpRange calculates torpedo range adjusted for target velocity
// to prevent fuse expiry on fast-moving targets.
// This is used for pattern selection (close/mid/long range decisions).
func (s *Server) getVelocityAdjustedTorpRange(p *game.Player, target *game.Player) float64 {
	shipStats := game.ShipData[p.Ship]
	baseRange := float64(game.EffectiveTorpRangeForShip(p.Ship, shipStats))

	// Calculate target's speed as a fraction of maximum possible speed
	targetSpeed := target.Speed
	maxPossibleSpeed := float64(game.ShipData[game.ShipScout].MaxSpeed)
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

// canTorpReachTarget checks whether a torpedo can reach the intercept point
// before its fuse expires. This accounts for target movement direction and speed,
// preventing bots from firing torpedoes that will expire en route.
func (s *Server) canTorpReachTarget(p *game.Player, target *game.Player) bool {
	shipStats := game.ShipData[p.Ship]
	projSpeed := float64(shipStats.TorpSpeed * game.TorpUnitFactor)

	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)

	solution, ok := InterceptDirection(shooterPos, targetPos, targetVel, projSpeed)
	if !ok {
		// No intercept solution — target is moving too fast to catch.
		// Only allow firing at very close range for area denial.
		dist := game.Distance(p.X, p.Y, target.X, target.Y)
		maxRange := float64(game.MaxTorpRange(shipStats))
		return dist < maxRange*0.3
	}

	// Check if the torpedo fuse allows reaching the intercept point
	safetyMargin, exists := game.ShipSafetyFactor[p.Ship]
	if !exists {
		safetyMargin = game.DefaultTorpSafety
	}
	maxFuseTicks := float64(shipStats.TorpFuse) * safetyMargin

	return solution.TimeToIntercept <= maxFuseTicks
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

// countTeamPlanets counts planets owned by each team.
// Returns a map keyed by team flag. Must be called under gameState.Mu lock.
// Results are cached per game frame to avoid redundant iteration when called
// from multiple bots in the same tick.
func (s *Server) countTeamPlanets() map[int]int {
	if s.cachedTeamPlanetsFrame > 0 && s.cachedTeamPlanetsFrame == s.gameState.Frame && s.cachedTeamPlanets != nil {
		return s.cachedTeamPlanets
	}
	var counts [9]int // Indexed by team flag (0=None, 1=Fed, 2=Rom, 4=Kli, 8=Ori)
	for _, planet := range s.gameState.Planets {
		if planet.Owner >= 0 && planet.Owner < len(counts) {
			counts[planet.Owner]++
		}
	}
	s.cachedTeamPlanets = map[int]int{
		game.TeamNone: counts[game.TeamNone],
		game.TeamFed:  counts[game.TeamFed],
		game.TeamRom:  counts[game.TeamRom],
		game.TeamKli:  counts[game.TeamKli],
		game.TeamOri:  counts[game.TeamOri],
	}
	s.cachedTeamPlanetsFrame = s.gameState.Frame
	return s.cachedTeamPlanets
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
func (s *Server) selectBotShipType(team int) game.ShipType {
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
		options := []game.ShipType{game.ShipDestroyer, game.ShipCruiser, game.ShipBattleship, game.ShipAssault}
		return options[rand.Intn(len(options))]
	}

	// Prefer destroyers and cruisers for balance
	if shipCounts[game.ShipDestroyer] < 2 {
		return game.ShipDestroyer
	}
	if shipCounts[game.ShipCruiser] < 2 {
		return game.ShipCruiser
	}

	// Add assault ship if none exists
	if shipCounts[game.ShipAssault] == 0 && total > 3 {
		return game.ShipAssault
	}

	// Random from all combat ships for variety
	// Excludes Starbase (handled separately) and Galaxy (rare)
	combatShips := []game.ShipType{
		game.ShipScout, game.ShipDestroyer, game.ShipCruiser,
		game.ShipBattleship, game.ShipAssault,
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

// coordinateTeamAttack checks if multiple allies are attacking the same target
// and returns the maximum ally cooldown so all bots fire together (volley fire).
func (s *Server) coordinateTeamAttack(p *game.Player, target *game.Player) int {
	maxCooldown := 0
	found := false

	for _, ally := range s.gameState.Players {
		if ally.Status == game.StatusAlive && ally.Team == p.Team && ally.ID != p.ID && ally.IsBot {
			if ally.BotTarget == target.ID {
				found = true
				if ally.BotCooldown > maxCooldown {
					maxCooldown = ally.BotCooldown
				}
			}
		}
	}

	if !found {
		return -1 // No coordination needed
	}
	return maxCooldown
}

// targetSuggestion records a deferred target suggestion from one bot to another.
// These are buffered during UpdateBots and applied after all bots have been processed,
// preventing lower-index bots from overriding higher-index bots' targeting decisions.
type targetSuggestion struct {
	allyID   int
	targetID int
	lockTime int
	value    float64
}

// broadcastTargetToAllies collects high-value target suggestions for nearby allies.
// Suggestions are buffered in pendingSuggestions and applied after all bots are
// processed (see ApplyPendingTargetSuggestions), so bot processing order does not
// affect targeting decisions.
func (s *Server) broadcastTargetToAllies(p *game.Player, target *game.Player, targetValue float64) {
	// Only broadcast for high-value targets (carriers, heavily damaged ships)
	if targetValue <= BroadcastTargetMinValue && target.Armies <= 0 {
		return
	}

	for _, ally := range s.gameState.Players {
		if ally.Status != game.StatusAlive || ally.Team != p.Team || ally.ID == p.ID || !ally.IsBot {
			continue
		}

		dist := game.Distance(p.X, p.Y, ally.X, ally.Y)
		if dist >= BroadcastTargetRange {
			continue
		}

		// Only suggest to allies with no current target — never overwrite an existing lock
		if ally.BotTarget < 0 {
			s.pendingSuggestions = append(s.pendingSuggestions, targetSuggestion{
				allyID:   ally.ID,
				targetID: target.ID,
				lockTime: 20,
				value:    targetValue,
			})
		}
	}
}

// ApplyPendingTargetSuggestions applies buffered target suggestions after all bots
// have been processed, so processing order does not affect targeting decisions.
func (s *Server) ApplyPendingTargetSuggestions() {
	for _, suggestion := range s.pendingSuggestions {
		ally := s.gameState.Players[suggestion.allyID]
		// Re-check that ally still has no target (another suggestion may have set one)
		if ally.Status == game.StatusAlive && ally.IsBot && ally.BotTarget < 0 {
			ally.BotTarget = suggestion.targetID
			ally.BotTargetLockTime = suggestion.lockTime
			ally.BotTargetValue = suggestion.value
		}
	}
	s.pendingSuggestions = s.pendingSuggestions[:0]
}
