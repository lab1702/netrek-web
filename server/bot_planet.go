package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// Planet Management AI Functions
// This file contains all functions related to planet operations:
// - Planet finding and selection
// - Strategic planet evaluation
// - Planet defense logic
// - Orbital operations

// findBestPlanetToTake finds the best planet to attack based on strategic value
func (s *Server) findBestPlanetToTake(p *game.Player) *game.Planet {
	var best *game.Planet
	bestScore := WorstScore

	// Get strategic context
	teamPlanets := s.countTeamPlanets()
	totalPlanets := len(s.gameState.Planets)
	controlRatio := float64(teamPlanets[p.Team]) / float64(totalPlanets)

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]

		// Consider enemy or neutral planets
		if planet.Owner == p.Team {
			continue
		}

		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist > 30000 {
			continue // Too far
		}

		// Base score on distance
		score := 15000.0 / dist

		// Strategic value assessment
		strategicValue := s.assessPlanetStrategicValue(planet, p.Team)
		score += strategicValue * 1000

		// Prefer planets with fewer armies (easier to take)
		armyDifficulty := float64(planet.Armies)
		if p.Armies > 0 {
			// We can take it if we have enough armies
			if p.Armies > planet.Armies {
				score += 3000
			}
		} else if planet.Armies < 5 {
			// Easy to bomb
			score += 2000 - armyDifficulty*200
		}

		// Prefer agricultural planets (they produce armies)
		if (planet.Flags & game.PlanetAgri) != 0 {
			score += 2000
		}

		// Prefer repair/fuel planets when we control few planets
		if controlRatio < 0.3 {
			if (planet.Flags & game.PlanetRepair) != 0 {
				score += 1500
			}
			if (planet.Flags & game.PlanetFuel) != 0 {
				score += 1000
			}
		}

		// Check for defenders using improved detection system
		defenderInfo := s.detectPlanetDefenders(planet, p.Team)

		// Count allies near planet
		allies := 0
		for _, other := range s.gameState.Players {
			if other.Status == game.StatusAlive && other.Team == p.Team && other.ID != p.ID {
				otherDist := game.Distance(planet.X, planet.Y, other.X, other.Y)
				if otherDist < 10000 {
					allies++
				}
			}
		}

		// Apply defender penalties with enhanced scoring
		score -= defenderInfo.DefenseScore * 0.8 // Scale down for planet selection

		// Heavy penalty if 2+ defenders and no allies (avoid suicide runs)
		if defenderInfo.DefenderCount >= 2 && allies == 0 {
			score -= 5000
		}

		// Bonus for allied support
		score += float64(allies) * 300

		// Frontline bonus - prefer planets near the battle
		if s.isPlanetOnFrontline(planet, p.Team) {
			score += 1000
		}

		if score > bestScore {
			bestScore = score
			best = planet
		}
	}

	return best
}

// assessPlanetStrategicValue evaluates a planet's strategic importance
func (s *Server) assessPlanetStrategicValue(planet *game.Planet, team int) float64 {
	value := 0.0

	// Check proximity to team's core planets
	nearbyFriendly := 0
	nearbyEnemy := 0

	for _, other := range s.gameState.Planets {
		if other.ID == planet.ID {
			continue
		}

		dist := game.Distance(planet.X, planet.Y, other.X, other.Y)
		if dist < 15000 {
			if other.Owner == team {
				nearbyFriendly++
			} else if other.Owner != 0 {
				nearbyEnemy++
			}
		}
	}

	// Planet that connects friendly territories is valuable
	if nearbyFriendly > 1 {
		value += float64(nearbyFriendly) * 0.5
	}

	// Planet that cuts enemy territories is valuable
	if nearbyEnemy > 2 {
		value += float64(nearbyEnemy) * 0.3
	}

	// Central planets are more valuable for map control
	distFromCenter := game.Distance(planet.X, planet.Y, game.GalaxyWidth/2, game.GalaxyHeight/2)
	if distFromCenter < 20000 {
		value += (20000 - distFromCenter) / 5000
	}

	return value
}

// detectPlanetDefenders finds enemy ships defending a planet
func (s *Server) detectPlanetDefenders(planet *game.Planet, team int) *PlanetDefenderInfo {
	// Constants for defender detection and scoring
	const (
		DEFENDER_RADIUS = 10000.0 // Range to consider ships as defenders
		BASE_SCORE      = 1000.0  // Base defense score
		DIST_FACTOR     = 0.15    // Weight for distance factor
		CARRIER_BONUS   = 2000.0  // Bonus for carriers
	)

	info := &PlanetDefenderInfo{
		Defenders:       make([]*game.Player, 0),
		DefenderCount:   0,
		MinDefenderDist: MaxSearchDistance,
	}

	// Find all enemy ships near the planet
	for i := range s.gameState.Players {
		player := s.gameState.Players[i]
		if player.Status == game.StatusAlive && player.Team != team && !player.Cloaked {
			dist := game.Distance(planet.X, planet.Y, player.X, player.Y)
			if dist <= DEFENDER_RADIUS {
				// Add to defenders list
				info.Defenders = append(info.Defenders, player)
				info.DefenderCount++

				// Track closest defender
				if dist < info.MinDefenderDist {
					info.MinDefenderDist = dist
					info.ClosestDefender = player
				}

				// Check if defender has armies (carrier)
				if player.Armies > 0 {
					info.HasCarrierDefense = true
				}
			}
		}
	}

	// Calculate defense score based on number of defenders, distance, and carriers
	info.DefenseScore = 0
	if info.DefenderCount > 0 {
		// Basic score based on number of defenders
		info.DefenseScore = float64(info.DefenderCount) * BASE_SCORE

		// Add distance factor (closer defenders make it more dangerous)
		if info.MinDefenderDist < DEFENDER_RADIUS {
			info.DefenseScore += (DEFENDER_RADIUS - info.MinDefenderDist) * DIST_FACTOR
		}

		// Add carrier bonus
		if info.HasCarrierDefense {
			info.DefenseScore += CARRIER_BONUS
		}
	}

	return info
}

// isPlanetOnFrontline checks if a planet is on the frontline
func (s *Server) isPlanetOnFrontline(planet *game.Planet, team int) bool {
	hasEnemyNearby := false
	hasFriendlyNearby := false

	for _, other := range s.gameState.Planets {
		if other.ID == planet.ID {
			continue
		}

		dist := game.Distance(planet.X, planet.Y, other.X, other.Y)
		if dist < 12000 {
			if other.Owner == team {
				hasFriendlyNearby = true
			} else if other.Owner != 0 && other.Owner != team {
				hasEnemyNearby = true
			}
		}
	}

	// Frontline has both friendly and enemy planets nearby
	return hasEnemyNearby && hasFriendlyNearby
}

// findNearestNeutralPlanet finds the closest neutral planet
func (s *Server) findNearestNeutralPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := MaxSearchDistance

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == 0 { // 0 is neutral team
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

// findNearestArmyPlanet finds the closest friendly planet with armies
func (s *Server) findNearestArmyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := MaxSearchDistance

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team && planet.Armies > 4 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

// findNearestEnemyArmyPlanet finds the closest enemy planet with armies
func (s *Server) findNearestEnemyArmyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := MaxSearchDistance

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner != p.Team && planet.Owner != 0 && planet.Armies > 4 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

// findPlanetToDefend finds a friendly planet that needs defense
func (s *Server) findPlanetToDefend(p *game.Player) *game.Planet {
	var best *game.Planet
	bestScore := WorstScore

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner != p.Team {
			continue
		}

		// Check for threats
		threatLevel := 0.0
		for _, enemy := range s.gameState.Players {
			if enemy.Status == game.StatusAlive && enemy.Team != p.Team {
				dist := game.Distance(planet.X, planet.Y, enemy.X, enemy.Y)
				if dist < 10000 {
					threatLevel += (10000 - dist) / 1000
					if enemy.Armies > 0 {
						threatLevel += 5
					}
				}
			}
		}

		if threatLevel > 0 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			score := threatLevel*1000 - dist/10

			// Prioritize important planets
			if (planet.Flags & game.PlanetAgri) != 0 {
				score += 500
			}
			if (planet.Flags & game.PlanetRepair) != 0 {
				score += 300
			}

			if score > bestScore {
				bestScore = score
				best = planet
			}
		}
	}

	return best
}

// findPlanetToRaid finds an enemy planet suitable for raiding
func (s *Server) findPlanetToRaid(p *game.Player) *game.Planet {
	var best *game.Planet
	bestScore := WorstScore

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team || planet.Owner == 0 {
			continue
		}

		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist > 20000 {
			continue
		}

		// Look for undefended planets
		defenders := 0
		for _, enemy := range s.gameState.Players {
			if enemy.Status == game.StatusAlive && enemy.Team == planet.Owner {
				if game.Distance(planet.X, planet.Y, enemy.X, enemy.Y) < 5000 {
					defenders++
				}
			}
		}

		if defenders == 0 && planet.Armies > 2 {
			score := 10000.0/dist + float64(planet.Armies)*500

			if score > bestScore {
				bestScore = score
				best = planet
			}
		}
	}

	return best
}

// defendPlanet handles planet defense maneuvering and combat for regular ships
func (s *Server) defendPlanet(p *game.Player, planet *game.Planet, enemy *game.Player, enemyDist float64) {
	shipStats := game.ShipData[p.Ship]

	// Set defense target to persist until threat is gone
	p.BotDefenseTarget = planet.ID

	// Clear any other bot states that would interfere
	p.Orbiting = -1
	p.Bombing = false
	p.Beaming = false
	p.BeamingUp = false
	p.BotPlanetApproachID = -1

	// Use comprehensive shield management for planet defense
	s.assessAndActivateShields(p)

	// Calculate intercept position between enemy and planet
	// We want to position ourselves optimally between the enemy and planet
	enemyToPlanetDir := math.Atan2(planet.Y-enemy.Y, planet.X-enemy.X)

	// Optimal intercept distance (3-5k from enemy, between enemy and planet)
	optimalInterceptDist := 4000.0
	if enemyDist < 6000 {
		optimalInterceptDist = 3500.0 // Closer for better weapon accuracy
	}

	// Calculate intercept position
	interceptX := enemy.X + math.Cos(enemyToPlanetDir)*optimalInterceptDist*0.7
	interceptY := enemy.Y + math.Sin(enemyToPlanetDir)*optimalInterceptDist*0.7

	// Check if we're positioned well (between enemy and planet)
	distToIntercept := game.Distance(p.X, p.Y, interceptX, interceptY)

	// Movement logic
	if distToIntercept > 1500 || enemyDist > 6000 {
		// Move to intercept position or chase enemy if too far
		navDir := math.Atan2(interceptY-p.Y, interceptX-p.X)

		// If enemy is far, move at full speed to close distance
		var desiredSpeed float64
		if enemyDist > 6000 {
			desiredSpeed = float64(shipStats.MaxSpeed)
		} else {
			desiredSpeed = s.getOptimalCombatSpeed(p, enemyDist)
		}

		// Use safe navigation with torpedo dodging
		s.applySafeNavigation(p, navDir, desiredSpeed)
	} else {
		// We're in position - engage with combat maneuvering
		angleToEnemy := math.Atan2(enemy.Y-p.Y, enemy.X-p.X)

		// Check if we're too close - use lateral movement to maintain range
		if enemyDist < 2000 {
			// Too close - move laterally to maintain better tactical position
			lateralDir := angleToEnemy + math.Pi/2 // Perpendicular to enemy direction
			p.DesDir = lateralDir
			p.DesSpeed = s.getOptimalCombatSpeed(p, enemyDist)
		} else {
			// Good range - face enemy for tactical positioning
			p.DesDir = angleToEnemy
			p.DesSpeed = s.getOptimalCombatSpeed(p, enemyDist)
		}
	}

	// Aggressive weapon usage for planet defense
	s.planetDefenseWeaponLogic(p, enemy, enemyDist)
}
