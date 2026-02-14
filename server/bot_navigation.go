package server

import (
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// planetPos is a lightweight position used by pre-filtered planet lists
// to avoid passing full *game.Planet through clearance checks.
type planetPos struct{ x, y float64 }

// calculateEnhancedInterceptCourse calculates intercept with acceleration prediction
func (s *Server) calculateEnhancedInterceptCourse(p, target *game.Player) float64 {
	dist := game.Distance(p.X, p.Y, target.X, target.Y)

	// If target is far or cloaked, use basic intercept
	if dist > 20000 || target.Cloaked || target.Speed < 1 {
		return math.Atan2(target.Y-p.Y, target.X-p.X)
	}

	// Track target's acceleration (speed changes)
	targetAccel := 0.0
	if target.Speed != target.DesSpeed {
		if target.DesSpeed > target.Speed {
			targetAccel = 1.0 // Accelerating
		} else {
			targetAccel = -1.0 // Decelerating
		}
	}

	// Enhanced prediction including acceleration
	// Use ship speed (not torpedo speed) since this is for navigation intercept
	mySpeed := math.Max(p.DesSpeed, 2) * 20 // Convert to units/tick, minimum warp 2
	timeToIntercept := dist / mySpeed

	// Clamp prediction horizon to prevent unrealistic extrapolation at long range.
	// Without this cap, distant targets get wildly overestimated speed predictions
	// (e.g., +31 warp units at 15000 distance) leading to poor navigation courses.
	if timeToIntercept > 15 {
		timeToIntercept = 15 // Cap at ~1.5 seconds (15 ticks at 10 FPS)
	}

	// Predict future position with acceleration
	futureSpeed := target.Speed + targetAccel*timeToIntercept*0.5
	if futureSpeed < 0 {
		futureSpeed = 0
	}
	if futureSpeed > float64(game.ShipData[target.Ship].MaxSpeed) {
		futureSpeed = float64(game.ShipData[target.Ship].MaxSpeed)
	}

	predictX := target.X + futureSpeed*math.Cos(target.Dir)*timeToIntercept*20
	predictY := target.Y + futureSpeed*math.Sin(target.Dir)*timeToIntercept*20

	return math.Atan2(predictY-p.Y, predictX-p.X)
}

// getAdvancedDodgeDirection calculates optimal dodge considering multiple threats
func (s *Server) getAdvancedDodgeDirection(p *game.Player, wantedDir float64, threats CombatThreat) float64 {
	bestDir := p.Dir
	bestScore := WorstScore

	// Pre-compute nearby planets once (within 12k of player) to avoid
	// scanning all 40 planets in every calculateClearance call (~25 calls).
	var nearbyPlanets []planetPos
	for _, planet := range s.gameState.Planets {
		if game.Distance(p.X, p.Y, planet.X, planet.Y) < 12000 {
			nearbyPlanets = append(nearbyPlanets, planetPos{x: planet.X, y: planet.Y})
		}
	}

	// Test different dodge angles
	for delta := 0.0; delta < math.Pi; delta += math.Pi / 12 {
		for sign := -1; sign <= 1; sign += 2 {
			if delta == 0 && sign == -1 {
				continue
			}

			testDir := wantedDir + float64(sign)*delta

			// Score this direction
			score := 0.0

			// Avoid torpedoes
			torpDanger := s.calculateTorpedoDanger(p, testDir)
			score -= torpDanger * 10

			// Avoid plasma — directional check like torpedoes
			plasmaDanger := s.calculatePlasmaDanger(p, testDir)
			score -= plasmaDanger * 10

			// Prefer directions that maintain some angle to target
			angleDiff := math.Abs(game.NormalizeAngle(testDir) - game.NormalizeAngle(wantedDir))
			if angleDiff > math.Pi {
				angleDiff = 2*math.Pi - angleDiff
			}
			score -= angleDiff * 100

			// Check wall and planet proximity using pre-filtered planets
			clearance := calculateClearanceWithPlanets(p, testDir, nearbyPlanets)
			if clearance < 3000 {
				score -= (3000 - clearance) * 2
			}

			if score > bestScore {
				bestScore = score
				bestDir = testDir
			}
		}
	}

	return bestDir
}

// calculateTorpedoDanger estimates torpedo danger in a direction
func (s *Server) calculateTorpedoDanger(p *game.Player, dir float64) float64 {
	danger := 0.0
	speed := p.Speed * 20 // Use actual current speed, not max

	for _, torp := range s.gameState.Torps {
		if torp.Owner == p.ID || torp.Team == p.Team || torp.Status != game.TorpMove {
			continue
		}

		// Simulate movement
		for t := 0.0; t < 3.0; t += 0.5 {
			myX := p.X + speed*math.Cos(dir)*t
			myY := p.Y + speed*math.Sin(dir)*t
			torpX := torp.X + torp.Speed*math.Cos(torp.Dir)*t
			torpY := torp.Y + torp.Speed*math.Sin(torp.Dir)*t

			dist := game.Distance(myX, myY, torpX, torpY)
			if dist < 700 {
				danger += (700 - dist) / 100
			}
		}
	}

	return danger
}

// calculatePlasmaDanger estimates plasma danger in a direction (mirrors calculateTorpedoDanger)
func (s *Server) calculatePlasmaDanger(p *game.Player, dir float64) float64 {
	danger := 0.0
	speed := p.Speed * 20 // Use actual current speed, not max

	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner == p.ID || plasma.Team == p.Team || plasma.Status != game.TorpMove {
			continue
		}

		for t := 0.0; t < 3.0; t += 0.5 {
			myX := p.X + speed*math.Cos(dir)*t
			myY := p.Y + speed*math.Sin(dir)*t
			plasmaX := plasma.X + plasma.Speed*math.Cos(plasma.Dir)*t
			plasmaY := plasma.Y + plasma.Speed*math.Sin(plasma.Dir)*t

			dist := game.Distance(myX, myY, plasmaX, plasmaY)
			if dist < 1000 {
				danger += (1000 - dist) / 100
			}
		}
	}

	return danger
}

// getOptimalSpeed returns optimal speed for given distance (like borgmove.c optimal_speed)
func (s *Server) getOptimalSpeed(p *game.Player, dist float64) float64 {
	if dist < 200 || p.Ship == game.ShipStarbase {
		return 2
	}

	// Deceleration factors match ship DecInt values from game/types.go.
	// Starbase is handled by the early return above.
	var decelerationFactor float64
	switch p.Ship {
	case game.ShipScout:
		decelerationFactor = 270
	case game.ShipDestroyer:
		decelerationFactor = 300
	case game.ShipBattleship:
		decelerationFactor = 180
	case game.ShipAssault, game.ShipCruiser:
		decelerationFactor = 200
	default:
		decelerationFactor = 200 // Safe fallback for any future ship type
	}

	// Calculate optimal speed to decelerate in time
	optimalSpeed := math.Sqrt((dist - 200) * decelerationFactor / 11500)
	if optimalSpeed > float64(game.ShipData[p.Ship].MaxSpeed) {
		optimalSpeed = float64(game.ShipData[p.Ship].MaxSpeed)
	}
	if optimalSpeed < 2 {
		optimalSpeed = 2
	}

	return optimalSpeed
}

// getOptimalCombatSpeed returns optimal combat speed based on distance
func (s *Server) getOptimalCombatSpeed(p *game.Player, dist float64) float64 {
	shipStats := game.ShipData[p.Ship]

	if dist > 6000 {
		return float64(shipStats.MaxSpeed)
	} else if dist > 3000 {
		return float64(shipStats.MaxSpeed) * 0.6
	} else if dist > 1500 {
		return float64(shipStats.MaxSpeed) * 0.4
	} else {
		return float64(shipStats.MaxSpeed) * 0.3
	}
}

// getEvasionSpeed returns optimal speed for evasion
func (s *Server) getEvasionSpeed(p *game.Player, threats CombatThreat) float64 {
	shipStats := game.ShipData[p.Ship]
	baseSpeed := float64(shipStats.MaxSpeed)

	// High threat - maximum speed
	if threats.threatLevel > 5 {
		return baseSpeed
	}

	// Medium threat - variable speed for unpredictability
	if threats.threatLevel > 2 {
		return baseSpeed * (0.6 + rand.Float64()*0.4)
	}

	// Low threat - maintain combat speed
	return s.getOptimalCombatSpeed(p, 3000)
}

// calculateClearanceWithPlanets calculates how much clear space in a direction,
// using a pre-filtered list of nearby planet positions to avoid scanning all 40 planets.
func calculateClearanceWithPlanets(p *game.Player, dir float64, nearbyPlanets []planetPos) float64 {
	testDist := 5000.0
	testX := p.X + math.Cos(dir)*testDist
	testY := p.Y + math.Sin(dir)*testDist

	// Check walls
	clearance := math.Min(testX, game.GalaxyWidth-testX)
	clearance = math.Min(clearance, testY)
	clearance = math.Min(clearance, game.GalaxyHeight-testY)

	// Check planets - treat planet surface as wall (discourage suicide dives)
	for _, np := range nearbyPlanets {
		planetDist := game.Distance(testX, testY, np.x, np.y)
		// Treat anything within 2000 units of planet surface as blocked
		planetClearance := planetDist - 2000
		if planetClearance < 0 {
			planetClearance = 0 // Hit the planet body
		}
		if planetClearance < clearance {
			clearance = planetClearance
		}
	}

	return clearance
}

// applySafeNavigation applies torpedo dodging and threat assessment to any navigation
func (s *Server) applySafeNavigation(p *game.Player, desiredDir float64, desiredSpeed float64) {
	// Always check for threats regardless of what the bot is doing
	threats := s.assessUniversalThreats(p)

	// If immediate torpedo evasion is required, override everything
	if threats.requiresEvasion {
		// Use advanced dodging but try to maintain general objective direction
		dodgeDir := s.getAdvancedDodgeDirection(p, desiredDir, threats)
		p.DesDir = dodgeDir
		p.DesSpeed = s.getEvasionSpeed(p, threats)

		// Shorter cooldown for immediate re-evaluation after dodging
		p.BotCooldown = 2
		return
	}

	// No immediate threat - apply desired navigation with separation
	separationVector := s.calculateSeparationVector(p)
	p.DesDir = blendWithSeparation(desiredDir, separationVector, 300.0, 0.5)

	// Apply desired speed
	p.DesSpeed = desiredSpeed

	// Apply comprehensive shield management for navigation threats
	s.assessAndActivateShields(p)

	// Check for medium-term torpedo threats and adjust speed
	if threats.closestTorpDist < 3000 {
		// Slight speed increase for better dodging options
		if p.DesSpeed < float64(game.ShipData[p.Ship].MaxSpeed)*0.8 {
			p.DesSpeed = math.Min(p.DesSpeed*1.2, float64(game.ShipData[p.Ship].MaxSpeed))
		}
	}
}

// blendWithSeparation blends a desired direction with a separation vector.
// divisor controls how quickly separation kicks in (lower = stronger).
// maxWeight caps the separation influence (e.g. 0.5 = 50% max).
// Returns the blended direction, or baseDir unchanged if separation has no magnitude.
func blendWithSeparation(baseDir float64, sep SeparationVector, divisor, maxWeight float64) float64 {
	if sep.magnitude <= 0 {
		return baseDir
	}
	weight := math.Min(sep.magnitude/divisor, maxWeight)
	navX := math.Cos(baseDir)*(1.0-weight) + sep.x*weight
	navY := math.Sin(baseDir)*(1.0-weight) + sep.y*weight
	return math.Atan2(navY, navX)
}

// calculateSeparationVector calculates a vector to maintain safe distance from allies
func (s *Server) calculateSeparationVector(p *game.Player) SeparationVector {
	separationVec := SeparationVector{x: 0, y: 0, magnitude: 0}

	nearbyAllies := 0
	totalRepelX := 0.0
	totalRepelY := 0.0

	for i, ally := range s.gameState.Players {
		// Skip self, non-alive, or enemy players
		if i == p.ID || ally.Status != game.StatusAlive || ally.Team != p.Team {
			continue
		}

		// Also skip if ally is orbiting (they're stationary)
		if ally.Orbiting >= 0 {
			continue
		}

		dist := game.Distance(p.X, p.Y, ally.X, ally.Y)

		// Consider all allies within extended range for separation
		if dist < SepMinSafeDistance && dist > 0 {
			nearbyAllies++

			// Normalized vector away from ally
			dx := p.X - ally.X
			dy := p.Y - ally.Y

			// Normalize
			norm := math.Sqrt(dx*dx + dy*dy)
			if norm > 0 {
				dx /= norm
				dy /= norm
			}

			// Repulsion strength based on distance zone
			var strength float64
			if dist < SepCriticalDistance {
				// Emergency separation - extremely strong repulsion
				strength = SepCriticalStrength * (SepCriticalDistance - dist) / SepCriticalDistance
			} else if dist < SepIdealDistance {
				// Strong separation to maintain ideal distance
				strength = SepIdealStrength * (SepIdealDistance - dist) / SepIdealDistance
			} else {
				// Moderate separation for distances beyond ideal
				strength = SepModerateStrength * (SepMinSafeDistance - dist) / SepMinSafeDistance
			}

			// Extra repulsion if both bots are moving toward the same target
			if p.BotTarget >= 0 && ally.BotTarget == p.BotTarget {
				strength *= SepSameTargetMult
			}

			// Weight more heavily if ally is damaged (more likely to explode)
			if ally.Damage > 0 {
				allyShipStats := game.ShipData[ally.Ship]
				damageRatio := float64(ally.Damage) / float64(allyShipStats.MaxDamage)
				if damageRatio > 0.5 {
					strength *= SepDamagedAllyHighMult
				} else if damageRatio > 0.3 {
					strength *= SepDamagedAllyLowMult
				}
			}

			// Extra force when multiple allies are nearby (breaks up clusters of 3+)
			if nearbyAllies >= 2 {
				strength *= SepClusterMult
			}

			totalRepelX += dx * strength
			totalRepelY += dy * strength
		}
	}

	// Calculate final separation vector with stronger magnitude
	if nearbyAllies > 0 {
		// Scale up the magnitude for more aggressive separation
		magnitudeScale := math.Min(1.0+float64(nearbyAllies)*0.3, SepMagnitudeCap)
		separationVec.x = totalRepelX * magnitudeScale
		separationVec.y = totalRepelY * magnitudeScale
		separationVec.magnitude = math.Sqrt(separationVec.x*separationVec.x + separationVec.y*separationVec.y)

		// Normalize x/y to unit vector for directional blending,
		// but preserve magnitude separately for weight calculations
		if separationVec.magnitude > 0 {
			separationVec.x /= separationVec.magnitude
			separationVec.y /= separationVec.magnitude
		}
	}

	return separationVec
}

// moveToSafeArea moves the bot to a safe area when no neutral planets are available
func (s *Server) moveToSafeArea(p *game.Player) {
	// Find the center of friendly space
	var friendlyX, friendlyY float64
	friendlyCount := 0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team {
			friendlyX += planet.X
			friendlyY += planet.Y
			friendlyCount++
		}
	}

	if friendlyCount > 0 {
		// Move towards the center of friendly space
		friendlyX /= float64(friendlyCount)
		friendlyY /= float64(friendlyCount)

		// Add some offset to avoid crowding the exact center
		angleOffset := float64(p.ID) * 0.5 // Spread bots out based on their ID
		offsetDist := 3000.0
		targetX := friendlyX + offsetDist*math.Cos(angleOffset)
		targetY := friendlyY + offsetDist*math.Sin(angleOffset)

		// Navigate to safe position
		dx := targetX - p.X
		dy := targetY - p.Y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist > 1000 {
			// Move towards safe area
			baseDir := math.Atan2(dy, dx)
			p.DesSpeed = float64(game.ShipData[p.Ship].MaxSpeed) * 0.5 // Move at half speed to conserve fuel

			// Apply separation to avoid bunching
			separationVector := s.calculateSeparationVector(p)
			p.DesDir = blendWithSeparation(baseDir, separationVector, 300.0, 0.5)
		} else {
			// At safe area - orbit slowly
			p.DesSpeed = 2.0
			p.DesDir = math.Mod(p.Dir+0.1, 2*math.Pi) // Gentle turn
		}

		// Clear any combat/planet actions
		p.Orbiting = -1
		p.Bombing = false
		p.Beaming = false
		p.BeamingUp = false
		p.BotCooldown = 10
	} else {
		// No friendly planets - just stop and wait
		p.DesSpeed = 0
		p.BotCooldown = 20
	}
}

// executePatrol implements intelligent patrol patterns for bots
func (s *Server) executePatrol(p *game.Player) {
	shipStats := game.ShipData[p.Ship]

	// Dynamic patrol based on game state
	if !p.BotHasGoal {
		// Choose patrol destination based on strategy
		teamPlanets := s.countTeamPlanets()
		controlRatio := float64(teamPlanets[p.Team]) / float64(len(s.gameState.Planets))

		if controlRatio < 0.3 {
			// Defensive patrol near home
			p.BotGoalX = float64(game.TeamHomeX[p.Team]) + float64(rand.Intn(15000)-7500)
			p.BotGoalY = float64(game.TeamHomeY[p.Team]) + float64(rand.Intn(15000)-7500)
		} else {
			// Offensive patrol in contested areas
			// Collect all frontline planets and pick one randomly
			// to spread bots across different contested areas
			var frontlineCandidates []*game.Planet
			for i := range s.gameState.Planets {
				planet := s.gameState.Planets[i]
				if s.isPlanetOnFrontline(planet, p.Team) {
					frontlineCandidates = append(frontlineCandidates, planet)
				}
			}
			var frontlinePlanet *game.Planet
			if len(frontlineCandidates) > 0 {
				frontlinePlanet = frontlineCandidates[rand.Intn(len(frontlineCandidates))]
			}

			if frontlinePlanet != nil {
				p.BotGoalX = frontlinePlanet.X + float64(rand.Intn(10000)-5000)
				p.BotGoalY = frontlinePlanet.Y + float64(rand.Intn(10000)-5000)
			} else {
				// Random enemy territory - pick a valid enemy team using bit flag constants
				allTeams := []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri}
				var enemyTeams []int
				for _, t := range allTeams {
					if t != p.Team {
						enemyTeams = append(enemyTeams, t)
					}
				}
				enemyTeam := enemyTeams[rand.Intn(len(enemyTeams))]
				p.BotGoalX = float64(game.TeamHomeX[enemyTeam]) + float64(rand.Intn(20000)-10000)
				p.BotGoalY = float64(game.TeamHomeY[enemyTeam]) + float64(rand.Intn(20000)-10000)
			}
		}

		// Clamp patrol destination to galaxy boundaries with margin
		margin := 5000.0 // Keep away from edges
		p.BotGoalX = math.Max(margin, math.Min(game.GalaxyWidth-margin, p.BotGoalX))
		p.BotGoalY = math.Max(margin, math.Min(game.GalaxyHeight-margin, p.BotGoalY))
		p.BotHasGoal = true
	}

	// Check if bot is stuck at galaxy edge and reset patrol
	edgeMargin := 2000.0
	if p.X < edgeMargin || p.X > game.GalaxyWidth-edgeMargin ||
		p.Y < edgeMargin || p.Y > game.GalaxyHeight-edgeMargin {
		// Bot is at edge, reset patrol destination
		p.BotHasGoal = false
		p.BotCooldown = 10
		return
	}

	// Navigate to patrol point
	dx := p.BotGoalX - p.X
	dy := p.BotGoalY - p.Y
	dist := math.Hypot(dx, dy)

	if dist < 3000 {
		// Reached patrol point, set new one
		p.BotHasGoal = false
	} else {
		baseDir := math.Atan2(dy, dx)

		// Apply separation during patrol to spread bots across the map
		// Stronger weight (200 divisor, 0.6 max) during patrol for better spread
		separationVector := s.calculateSeparationVector(p)
		p.DesDir = blendWithSeparation(baseDir, separationVector, 200.0, 0.6)
		p.DesSpeed = float64(shipStats.MaxSpeed) * 0.8 // Sustainable cruise speed
	}
}
