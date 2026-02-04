package server

import (
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// calculateInterceptCourse calculates the intercept course to hit a moving target
// Based on borgmove.c calc_icept_course function
func (s *Server) calculateInterceptCourse(p, target *game.Player) float64 {
	dist := game.Distance(p.X, p.Y, target.X, target.Y)

	// If target is far, cloaked, or slow, just aim directly
	if dist > 20000 || target.Cloaked || target.Speed < 1 || p.DesSpeed < 1 {
		return math.Atan2(target.Y-p.Y, target.X-p.X)
	}

	// Calculate intercept using relative velocity
	vxa := target.X - p.X
	vya := target.Y - p.Y
	l := math.Hypot(vxa, vya)

	if l > 0 {
		vxa /= l
		vya /= l
	}

	// Target velocity components
	vxs := target.Speed * math.Cos(target.Dir) * 20 // 20 units per tick
	vys := target.Speed * math.Sin(target.Dir) * 20

	// Dot product for parallel component
	dp := vxs*vxa + vys*vya
	dx := vxs - dp*vxa
	dy := vys - dp*vya

	// Calculate intercept
	l = math.Hypot(dx, dy)
	mySpeed := p.DesSpeed * 20
	l = mySpeed*mySpeed - l*l

	if l < 0 {
		// Can't intercept, aim directly
		return math.Atan2(target.Y-p.Y, target.X-p.X)
	}

	l = math.Sqrt(l)
	vxt := l*vxa + dx
	vyt := l*vya + dy

	return math.Atan2(vyt, vxt)
}

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
	torpSpeed := float64(game.ShipData[p.Ship].TorpSpeed) * 20
	timeToIntercept := dist / torpSpeed

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

	// Check if target is likely to turn (near planets or walls)
	if s.isNearObstacle(target) {
		// Predict turn away from obstacle
		turnPrediction := s.predictTurnDirection(target)
		predictX += math.Cos(turnPrediction) * 500
		predictY += math.Sin(turnPrediction) * 500
	}

	return math.Atan2(predictY-p.Y, predictX-p.X)
}

// getBestDodgeDirection calculates the best direction to dodge torpedoes
// Based on borgmove.c get_best_dir function
func (s *Server) getBestDodgeDirection(p *game.Player, wantedDir float64) float64 {
	shipStats := game.ShipData[p.Ship]
	bestDir := p.Dir
	bestDamage := 999999

	// Test different dodge angles
	for delta := 0.0; delta < math.Pi/2; delta += math.Pi / 16 {
		for sign := -1; sign <= 1; sign += 2 {
			if delta == 0 && sign == -1 {
				continue
			}

			testDir := wantedDir + float64(sign)*delta
			damage := s.calculateDamageAtDirection(p, testDir, p.DesSpeed)

			if damage < bestDamage {
				bestDamage = damage
				bestDir = testDir
			}

			// Also test with speed changes
			if p.DesSpeed < float64(shipStats.MaxSpeed) {
				damage = s.calculateDamageAtDirection(p, testDir, p.DesSpeed+1)
				if damage < bestDamage {
					bestDamage = damage
					bestDir = testDir
				}
			}

			if p.DesSpeed > 2 {
				damage = s.calculateDamageAtDirection(p, testDir, p.DesSpeed-1)
				if damage < bestDamage {
					bestDamage = damage
					bestDir = testDir
				}
			}
		}
	}

	return bestDir
}

// getAdvancedDodgeDirection calculates optimal dodge considering multiple threats
func (s *Server) getAdvancedDodgeDirection(p *game.Player, wantedDir float64, threats CombatThreat) float64 {
	bestDir := p.Dir
	bestScore := -999999.0

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

			// Avoid plasma
			if threats.closestPlasma < 5000 {
				plasmaDanger := 5000 - threats.closestPlasma
				score -= plasmaDanger
			}

			// Prefer directions that maintain some angle to target
			angleDiff := math.Abs(testDir - wantedDir)
			if angleDiff > math.Pi {
				angleDiff = 2*math.Pi - angleDiff
			}
			score -= angleDiff * 100

			// Check wall proximity
			clearance := s.calculateClearance(p, testDir)
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

// calculateDamageAtDirection estimates damage if moving in a direction
func (s *Server) calculateDamageAtDirection(p *game.Player, dir, speed float64) int {
	totalDamage := 0
	px, py := p.X, p.Y

	// Simulate movement
	dx := speed * math.Cos(dir) * 20
	dy := speed * math.Sin(dir) * 20

	// Check collision with torpedoes over next few ticks
	for _, torp := range s.gameState.Torps {
		if torp.Owner == p.ID {
			continue
		}

		tx, ty := torp.X, torp.Y
		tdx := torp.Speed * math.Cos(torp.Dir)
		tdy := torp.Speed * math.Sin(torp.Dir)

		// Simulate next 5 ticks
		for tick := 0; tick < 5; tick++ {
			mx := px + dx*float64(tick)
			my := py + dy*float64(tick)
			ttx := tx + tdx*float64(tick)
			tty := ty + tdy*float64(tick)

			dist := game.Distance(mx, my, ttx, tty)
			if dist < 500 {
				totalDamage += torp.Damage
			}
		}
	}

	// Check wall proximity
	if px < 3500 || px > game.GalaxyWidth-3500 ||
		py < 3500 || py > game.GalaxyHeight-3500 {
		// Near wall, penalize directions toward wall
		if (px < 3500 && dir > math.Pi/2 && dir < 3*math.Pi/2) ||
			(px > game.GalaxyWidth-3500 && (dir < math.Pi/2 || dir > 3*math.Pi/2)) ||
			(py < 3500 && dir > math.Pi) ||
			(py > game.GalaxyHeight-3500 && dir < math.Pi) {
			totalDamage += 60
		}
	}

	return totalDamage
}

// calculateTorpedoDanger estimates torpedo danger in a direction
func (s *Server) calculateTorpedoDanger(p *game.Player, dir float64) float64 {
	danger := 0.0
	speed := float64(game.ShipData[p.Ship].MaxSpeed) * 20

	for _, torp := range s.gameState.Torps {
		if torp.Owner == p.ID || torp.Status != 1 {
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

// getOptimalSpeed returns optimal speed for given distance (like borgmove.c optimal_speed)
func (s *Server) getOptimalSpeed(p *game.Player, dist float64) float64 {
	if dist < 200 || p.Ship == game.ShipStarbase {
		return 2
	}

	var decelerationFactor float64
	switch p.Ship {
	case game.ShipScout:
		decelerationFactor = 270
	case game.ShipDestroyer:
		decelerationFactor = 300
	case game.ShipBattleship:
		decelerationFactor = 180
	case game.ShipAssault:
		decelerationFactor = 200
	case game.ShipStarbase:
		decelerationFactor = 150
	default:
		decelerationFactor = 200
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

	// Check if we're near a planet - may need higher speed for better dodging
	planetProximity := false
	for _, planet := range s.gameState.Planets {
		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist < 8000 {
			planetProximity = true
			break
		}
	}

	// Planet proximity bonus - slight speed increase for better dodging options
	planetSpeedMultiplier := 1.0
	if planetProximity {
		planetSpeedMultiplier = 1.15
	}

	// High threat - maximum speed
	if threats.threatLevel > 5 {
		return baseSpeed * planetSpeedMultiplier
	}

	// Medium threat - variable speed for unpredictability
	if threats.threatLevel > 2 {
		return baseSpeed * planetSpeedMultiplier * (0.6 + rand.Float64()*0.4)
	}

	// Low threat - maintain combat speed
	return s.getOptimalCombatSpeed(p, 3000)
}

// isNearObstacle checks if player is near walls or planets
func (s *Server) isNearObstacle(p *game.Player) bool {
	// Check walls
	if p.X < 5000 || p.X > game.GalaxyWidth-5000 ||
		p.Y < 5000 || p.Y > game.GalaxyHeight-5000 {
		return true
	}

	// Check planets
	for _, planet := range s.gameState.Planets {
		if game.Distance(p.X, p.Y, planet.X, planet.Y) < 3000 {
			return true
		}
	}

	return false
}

// predictTurnDirection predicts which way a player will turn to avoid obstacles
func (s *Server) predictTurnDirection(p *game.Player) float64 {
	bestDir := p.Dir
	bestClearance := 0.0

	// Test various turn angles
	for angle := -math.Pi / 2; angle <= math.Pi/2; angle += math.Pi / 8 {
		testDir := p.Dir + angle
		clearance := s.calculateClearance(p, testDir)
		if clearance > bestClearance {
			bestClearance = clearance
			bestDir = testDir
		}
	}

	return bestDir
}

// calculateClearance calculates how much clear space in a direction
func (s *Server) calculateClearance(p *game.Player, dir float64) float64 {
	testDist := 5000.0
	testX := p.X + math.Cos(dir)*testDist
	testY := p.Y + math.Sin(dir)*testDist

	// Check walls
	clearance := math.Min(testX, game.GalaxyWidth-testX)
	clearance = math.Min(clearance, testY)
	clearance = math.Min(clearance, game.GalaxyHeight-testY)

	// Check planets - treat planet surface as wall (discourage suicide dives)
	for _, planet := range s.gameState.Planets {
		planetDist := game.Distance(testX, testY, planet.X, planet.Y)
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
func (s *Server) applySafeNavigation(p *game.Player, desiredDir float64, desiredSpeed float64, objective string) {
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

		// For debugging/logging - comment this out in production
		// fmt.Printf("Bot %d dodging torpedo while %s\n", p.ID, objective)
		return
	}

	// No immediate threat - apply desired navigation with separation
	separationVector := s.calculateSeparationVector(p)
	if separationVector.magnitude > 0 {
		// Blend desired direction with separation
		weight := math.Min(separationVector.magnitude/300.0, 0.5)
		navX := math.Cos(desiredDir)*(1.0-weight) + separationVector.x*weight
		navY := math.Sin(desiredDir)*(1.0-weight) + separationVector.y*weight
		p.DesDir = math.Atan2(navY, navX)
	} else {
		p.DesDir = desiredDir
	}

	// Apply desired speed
	p.DesSpeed = desiredSpeed

	// Apply comprehensive shield management for navigation threats
	s.assessAndActivateShields(p, nil)

	// Check for medium-term torpedo threats and adjust speed
	if threats.closestTorpDist < 3000 {
		// Slight speed increase for better dodging options
		if p.DesSpeed < float64(game.ShipData[p.Ship].MaxSpeed)*0.8 {
			p.DesSpeed = math.Min(p.DesSpeed*1.2, float64(game.ShipData[p.Ship].MaxSpeed))
		}
	}
}

// calculateSeparationVector calculates a vector to maintain safe distance from allies
func (s *Server) calculateSeparationVector(p *game.Player) SeparationVector {
	separationVec := SeparationVector{x: 0, y: 0, magnitude: 0}

	// Increased distances for better separation
	// We want bots to maintain much larger distances to prevent bunching
	minSafeDistance := 4000.0  // Increased from 1500 to 4000
	idealDistance := 2500.0    // Ideal spacing between bots
	criticalDistance := 1200.0 // Emergency separation distance (increased from 800)

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
		if dist < minSafeDistance && dist > 0 {
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

			// Much stronger repulsion forces
			var strength float64
			if dist < criticalDistance {
				// Emergency separation - extremely strong repulsion
				strength = 5.0 * (criticalDistance - dist) / criticalDistance
			} else if dist < idealDistance {
				// Strong separation to maintain ideal distance
				strength = 2.0 * (idealDistance - dist) / idealDistance
			} else {
				// Moderate separation for distances beyond ideal
				strength = 0.8 * (minSafeDistance - dist) / minSafeDistance
			}

			// Extra repulsion if both bots are moving toward the same target
			if p.BotTarget >= 0 && ally.BotTarget == p.BotTarget {
				strength *= 1.8 // Much stronger separation when targeting same enemy
			}

			// Weight more heavily if ally is damaged (more likely to explode)
			if ally.Damage > 0 {
				allyShipStats := game.ShipData[ally.Ship]
				damageRatio := float64(ally.Damage) / float64(allyShipStats.MaxDamage)
				if damageRatio > 0.5 {
					strength *= 2.0 // Doubled from 1.5
				} else if damageRatio > 0.3 {
					strength *= 1.5
				}
			}

			// If ally is also very close to another ally, increase separation
			// This helps break up clusters of 3+ bots
			for j, other := range s.gameState.Players {
				if j != i && j != p.ID && other.Status == game.StatusAlive &&
					other.Team == p.Team && other.Orbiting < 0 {
					otherDist := game.Distance(ally.X, ally.Y, other.X, other.Y)
					if otherDist < idealDistance {
						strength *= 1.3 // Extra force to break up clusters
						break
					}
				}
			}

			totalRepelX += dx * strength
			totalRepelY += dy * strength
		}
	}

	// Calculate final separation vector with stronger magnitude
	if nearbyAllies > 0 {
		// Scale up the magnitude for more aggressive separation
		// Cap at 3.0 to prevent erratic scattering with many nearby allies
		magnitudeScale := math.Min(1.0+float64(nearbyAllies)*0.3, 3.0)
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
			p.DesDir = math.Atan2(dy, dx)
			p.DesSpeed = float64(game.ShipData[p.Ship].MaxSpeed) * 0.5 // Move at half speed to conserve fuel

			// Apply separation to avoid bunching
			separationVector := s.calculateSeparationVector(p)
			if separationVector.magnitude > 0 {
				weight := math.Min(separationVector.magnitude/300.0, 0.5)
				navX := math.Cos(p.DesDir)*(1.0-weight) + separationVector.x*weight
				navY := math.Sin(p.DesDir)*(1.0-weight) + separationVector.y*weight
				p.DesDir = math.Atan2(navY, navX)
			}
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
	if p.BotGoalX == 0 && p.BotGoalY == 0 {
		// Choose patrol destination based on strategy
		teamPlanets := s.countTeamPlanets()
		controlRatio := float64(teamPlanets[p.Team]) / float64(len(s.gameState.Planets))

		if controlRatio < 0.3 {
			// Defensive patrol near home
			p.BotGoalX = float64(game.TeamHomeX[p.Team]) + float64(rand.Intn(15000)-7500)
			p.BotGoalY = float64(game.TeamHomeY[p.Team]) + float64(rand.Intn(15000)-7500)
		} else {
			// Offensive patrol in contested areas
			// Find a frontline planet
			var frontlinePlanet *game.Planet
			for i := range s.gameState.Planets {
				planet := s.gameState.Planets[i]
				if s.isPlanetOnFrontline(planet, p.Team) {
					frontlinePlanet = planet
					break
				}
			}

			if frontlinePlanet != nil {
				p.BotGoalX = frontlinePlanet.X + float64(rand.Intn(10000)-5000)
				p.BotGoalY = frontlinePlanet.Y + float64(rand.Intn(10000)-5000)
			} else {
				// Random enemy territory
				enemyTeam := (p.Team % 4) + 1
				if enemyTeam > 4 {
					enemyTeam = 1
				}
				p.BotGoalX = float64(game.TeamHomeX[enemyTeam]) + float64(rand.Intn(20000)-10000)
				p.BotGoalY = float64(game.TeamHomeY[enemyTeam]) + float64(rand.Intn(20000)-10000)
			}
		}

		// Clamp patrol destination to galaxy boundaries with margin
		margin := 5000.0 // Keep away from edges
		p.BotGoalX = math.Max(margin, math.Min(game.GalaxyWidth-margin, p.BotGoalX))
		p.BotGoalY = math.Max(margin, math.Min(game.GalaxyHeight-margin, p.BotGoalY))
	}

	// Check if bot is stuck at galaxy edge and reset patrol
	edgeMargin := 2000.0
	if p.X < edgeMargin || p.X > game.GalaxyWidth-edgeMargin ||
		p.Y < edgeMargin || p.Y > game.GalaxyHeight-edgeMargin {
		// Bot is at edge, reset patrol destination
		p.BotGoalX = 0
		p.BotGoalY = 0
		p.BotCooldown = 10
		return
	}

	// Navigate to patrol point
	dx := p.BotGoalX - p.X
	dy := p.BotGoalY - p.Y
	dist := math.Hypot(dx, dy)

	if dist < 3000 {
		// Reached patrol point, set new one
		p.BotGoalX = 0
		p.BotGoalY = 0
	} else {
		baseDir := math.Atan2(dy, dx)

		// Apply separation during patrol to spread bots across the map
		separationVector := s.calculateSeparationVector(p)
		if separationVector.magnitude > 0 {
			// Stronger weight during patrol to ensure better spread
			weight := math.Min(separationVector.magnitude/200.0, 0.6) // Much stronger for patrol
			navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
			navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
			p.DesDir = math.Atan2(navY, navX)
		} else {
			p.DesDir = baseDir
		}
		p.DesSpeed = float64(shipStats.MaxSpeed) * 0.8 // Sustainable cruise speed
	}
}
