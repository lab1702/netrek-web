package server

import (
	"github.com/lab1702/netrek-web/game"
	"math"
)

// engageCombat handles combat engagement for hard bots
func (s *Server) engageCombat(p *game.Player, target *game.Player, dist float64) {
	shipStats := game.ShipData[p.Ship]
	targetStats := game.ShipData[target.Ship]

	// Consider breaking orbit when entering combat
	if p.Orbiting >= 0 {
		// Only break orbit if the planet doesn't need bombing or threat is extreme
		if p.Orbiting < len(s.gameState.Planets) {
			planet := s.gameState.Planets[p.Orbiting]
			// Only leave if planet is friendly, has no armies, or we're in extreme danger
			if planet.Owner == p.Team || planet.Armies == 0 ||
				(dist < 2000 && p.Damage > game.ShipData[p.Ship].MaxDamage/2) {
				p.Orbiting = -1
				p.Bombing = false
				p.Beaming = false
				p.BeamingUp = false
			} else if planet.Owner != p.Team && planet.Armies > 0 && dist > 4000 {
				// Continue bombing enemy planet if threat is not immediate
				p.Bombing = true
				p.BotCooldown = 5
				return // Stay and bomb
			}
		} else {
			p.Orbiting = -1
			p.Bombing = false
			p.Beaming = false
			p.BeamingUp = false
		}
	}

	// Check for nearby allies to avoid bunching up
	separationVector := s.calculateSeparationVector(p)

	// Calculate intercept course with enhanced prediction
	interceptDir := s.calculateEnhancedInterceptCourse(p, target)

	// Check for all threats (torpedoes, plasma, nearby enemies)
	threats := s.assessUniversalThreats(p)
	closestTorpDist := threats.closestTorpDist

	// Advanced dodge with threat prioritization
	if threats.requiresEvasion {
		dodgeDir := s.getAdvancedDodgeDirection(p, interceptDir, threats)
		p.DesDir = dodgeDir
		p.DesSpeed = s.getEvasionSpeed(p, threats)
		p.BotCooldown = 3
		return
	}

	// Combat maneuvering based on range and ship matchup
	combatManeuver := s.selectCombatManeuver(p, target, dist)

	// Apply separation adjustment if allies are too close
	if separationVector.magnitude > 0 {
		// Blend the combat direction with separation vector
		// Much higher weight for separation to prevent bunching
		separationWeight := math.Min(separationVector.magnitude/300.0, 0.75) // Increased max weight to 0.75
		combatWeight := 1.0 - separationWeight

		// Combine directions using weighted average
		desiredX := combatWeight*math.Cos(combatManeuver.direction) + separationWeight*separationVector.x
		desiredY := combatWeight*math.Sin(combatManeuver.direction) + separationWeight*separationVector.y
		p.DesDir = math.Atan2(desiredY, desiredX)

		// Also reduce speed when too close to allies for better separation
		if separationVector.magnitude > 2.0 {
			p.DesSpeed = combatManeuver.speed * 0.7 // Slow down to separate better
		} else {
			p.DesSpeed = combatManeuver.speed
		}
	} else {
		p.DesDir = combatManeuver.direction
		p.DesSpeed = combatManeuver.speed
	}

	// Adjust for damage and energy management
	if p.Damage > 0 {
		damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
		maxSpeed := (float64(shipStats.MaxSpeed) + 2) - (float64(shipStats.MaxSpeed)+1)*damageRatio
		if p.DesSpeed > maxSpeed {
			p.DesSpeed = maxSpeed
		}
	}

	// Check for team coordination opportunities
	if ally := s.findNearbyAlly(p, 10000); ally != nil {
		// Coordinate attacks on same target
		if ally.BotTarget == target.ID || s.shouldFocusFire(p, ally, target) {
			// Synchronized attack timing
			p.BotCooldown = (p.BotCooldown + ally.BotCooldown) / 2
		}
	}

	// Weapon usage - no facing restrictions needed

	// Enhanced torpedo firing with prediction and spread patterns
	// Torpedoes can be fired in any direction regardless of ship facing
	effectiveTorpRange := float64(game.EffectiveTorpRangeDefault(shipStats))
	if dist < effectiveTorpRange && p.NumTorps < game.MaxTorps-2 && p.Fuel > 2000 && p.WTemp < 80 {
		// Use spread pattern at medium range for area denial
		if dist > 3000 && dist < 5000 && p.NumTorps < game.MaxTorps-4 {
			s.fireTorpedoSpread(p, target, 3)
			p.BotCooldown = 8
		} else {
			s.fireEnhancedTorpedo(p, target)
			p.BotCooldown = 6
		}
	}

	// Fire when enemy is running away
	if dist < 7000 && p.NumTorps < game.MaxTorps-4 && p.Fuel > 1500 {
		targetAngleToUs := math.Atan2(p.Y-target.Y, p.X-target.X)
		targetRunAngle := math.Abs(target.Dir - targetAngleToUs)
		if targetRunAngle > math.Pi {
			targetRunAngle = 2*math.Pi - targetRunAngle
		}
		if targetRunAngle < math.Pi/3 && target.Speed > float64(shipStats.MaxSpeed)*0.5 {
			s.fireBotTorpedoWithLead(p, target)
			p.BotCooldown = 8
		}
	}

	// Enhanced phaser timing with kill securing
	// Phasers can be fired in any direction regardless of ship facing
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)
	if dist < myPhaserRange {
		phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
		if p.Fuel >= phaserCost*2 && p.WTemp < 80 {
			targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)
			// Calculate if phaser would be a kill shot
			phaserDamage := float64(shipStats.PhaserDamage) * (1.0 - dist/myPhaserRange)
			wouldKill := target.Damage+int(phaserDamage) >= targetStats.MaxDamage

			if wouldKill || targetDamageRatio > 0.6 || dist < 1200 || target.Cloaked {
				s.fireBotPhaser(p, target)
				p.BotCooldown = 10
			}
		}
	}

	// Variable for plasma usage
	targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)

	// Predictive shield management
	s.managePredictiveShields(p, target, dist, closestTorpDist)

	// Enhanced plasma usage for area control
	if shipStats.HasPlasma && p.NumPlasma < 1 && p.Fuel > 4000 {
		// Use plasma for area denial or finishing damaged enemies
		// Reuse the effective torpedo range for consistency
		if (dist < 7000 && dist > 2500 && target.Speed < 4) ||
			(targetDamageRatio > 0.7 && dist < 5000) ||
			(target.Orbiting >= 0 && dist < effectiveTorpRange) {
			s.fireBotPlasma(p, target)
			p.BotCooldown = 20
		}
	}

	// Cloaking tactics for scouts and destroyers
	if (p.Ship == game.ShipScout || p.Ship == game.ShipDestroyer) && p.Fuel > 3000 {
		if s.shouldUseCloaking(p, target, dist) {
			p.Cloaked = true
		} else if p.Cloaked && (p.Fuel < 1500 || dist < 1000) {
			p.Cloaked = false
		}
	}

	p.BotTarget = target.ID
}

// defendWhileCarrying handles defensive behavior when carrying armies
func (s *Server) defendWhileCarrying(p, enemy *game.Player) {
	if enemy == nil {
		return
	}

	shipStats := game.ShipData[p.Ship]
	dist := game.Distance(p.X, p.Y, enemy.X, enemy.Y)

	// Try to maintain distance while carrying
	if dist < 3000 {
		// Enemy too close - defensive maneuvers
		angleAway := math.Atan2(p.Y-enemy.Y, p.X-enemy.X)
		p.DesDir = angleAway
		p.DesSpeed = float64(shipStats.MaxSpeed)

		// Fire defensively
		if p.NumTorps < game.MaxTorps && p.Fuel > 2000 {
			// Fire torpedo behind us
			s.fireBotTorpedoWithLead(p, enemy)
		}

		// Use enhanced shield management when carrying armies
		s.assessAndActivateShields(p, enemy)
	}
}

// shouldDodge checks if the bot should dodge incoming threats
func (s *Server) shouldDodge(p *game.Player) bool {
	// Check for incoming torpedoes
	for _, torp := range s.gameState.Torps {
		if torp.Owner == p.ID {
			continue
		}

		// Check if torpedo is heading toward us
		dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
		if dist < 3000 {
			// Check angle to see if it's heading our way
			dx := p.X - torp.X
			dy := p.Y - torp.Y
			angleToUs := math.Atan2(dy, dx)
			angleDiff := math.Abs(angleToUs - torp.Dir)
			if angleDiff > math.Pi {
				angleDiff = 2*math.Pi - angleDiff
			}
			if angleDiff < math.Pi/6 { // Within 30 degrees
				return true
			}
		}
	}
	return false
}

// assessUniversalThreats evaluates all threats to the bot (for both combat and navigation)
func (s *Server) assessUniversalThreats(p *game.Player) CombatThreat {
	threat := CombatThreat{
		closestTorpDist: 999999.0,
		closestPlasma:   999999.0,
		nearbyEnemies:   0,
		requiresEvasion: false,
		threatLevel:     0,
	}

	// Enhanced torpedo checking for all movement scenarios
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID && torp.Status == 1 {
			dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if dist < threat.closestTorpDist {
				threat.closestTorpDist = dist
			}

			// Improved torpedo threat prediction
			if s.isTorpedoThreatening(p, torp) {
				threat.requiresEvasion = true
				threat.threatLevel += 4

				// Increase threat level based on proximity
				baseThreatIncrease := 0
				if dist < 2000 {
					baseThreatIncrease = 3
				} else if dist < 4000 {
					baseThreatIncrease = 1
				}

				// Check if we're near a planet - torpedoes are more dangerous in planet areas
				planetProximityBonus := 0.0
				for _, planet := range s.gameState.Planets {
					pDistToPlanet := game.Distance(p.X, p.Y, planet.X, planet.Y)
					torpDistToPlanet := game.Distance(torp.X, torp.Y, planet.X, planet.Y)

					// If both bot and torp are near same planet, increase danger
					if pDistToPlanet < 10000 && torpDistToPlanet < 10000 {
						planetProximityBonus = 1.5
						break
					}
				}

				threat.threatLevel += int(float64(baseThreatIncrease) * planetProximityBonus)
			}
		}
	}

	// Check plasma threats
	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner != p.ID && plasma.Status == 1 {
			dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)
			if dist < threat.closestPlasma {
				threat.closestPlasma = dist
			}
			if dist < 4000 {
				threat.requiresEvasion = true
				threat.threatLevel += 5
			}
		}
	}

	// Check nearby enemies for additional context
	for _, enemy := range s.gameState.Players {
		if enemy.Status == game.StatusAlive && enemy.Team != p.Team {
			dist := game.Distance(p.X, p.Y, enemy.X, enemy.Y)
			if dist < 5000 {
				threat.nearbyEnemies++
				threat.threatLevel++

				// Check if enemy is facing us (potential phaser threat)
				angleToUs := math.Atan2(p.Y-enemy.Y, p.X-enemy.X)
				angleDiff := math.Abs(enemy.Dir - angleToUs)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}
				if dist < 2000 && angleDiff < math.Pi/6 {
					threat.requiresEvasion = true
					threat.threatLevel += 2
				}
			}
		}
	}

	return threat
}

// isTorpedoThreatening checks if a torpedo poses a real threat using enhanced prediction
func (s *Server) isTorpedoThreatening(p *game.Player, torp *game.Torpedo) bool {
	// Distance check - only consider nearby torpedoes
	dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
	if dist > 5000 { // Increased detection range to catch more threats
		return false
	}

	// Calculate relative positions and velocities
	// Torpedo position and velocity
	torpX, torpY := torp.X, torp.Y
	torpSpeed := torp.Speed
	torpDir := torp.Dir
	torpVelX := torpSpeed * math.Cos(torpDir)
	torpVelY := torpSpeed * math.Sin(torpDir)

	// Player position and velocity - use INTENDED movement, not current
	// This fixes the critical bug where bots use stale direction data
	playerDir := p.DesDir          // Use desired direction instead of current
	playerSpeed := p.DesSpeed * 20 // Use desired speed, convert to units per tick
	// Fallback to current values if desired values aren't set
	if p.DesSpeed == 0 {
		playerSpeed = p.Speed * 20
	}
	playerVelX := playerSpeed * math.Cos(playerDir)
	playerVelY := playerSpeed * math.Sin(playerDir)

	// Simulate future positions to check for collision
	for t := 0.0; t < 5.0; t += 0.2 { // Check next 5 ticks in finer increments for better accuracy
		// Future torpedo position
		futTorpX := torpX + torpVelX*t
		futTorpY := torpY + torpVelY*t

		// Future player position (using intended course)
		futPlayerX := p.X + playerVelX*t
		futPlayerY := p.Y + playerVelY*t

		// Check for collision with larger safety margin
		collisionDist := game.Distance(futPlayerX, futPlayerY, futTorpX, futTorpY)
		if collisionDist < 800 { // Increased safety threshold from 600 to 800
			return true
		}
	}

	// Also check if torpedo is generally heading towards our area
	// Vector from torpedo to player
	dx := p.X - torpX
	dy := p.Y - torpY
	angleToPlayer := math.Atan2(dy, dx)

	// Calculate angle difference
	angleDiff := math.Abs(angleToPlayer - torpDir)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	// If torpedo is heading somewhat towards us, it's a threat
	if angleDiff < math.Pi/2.5 && dist < 4000 { // Within ~72 degrees and closer range
		return true
	}

	// Also consider very close torpedoes regardless of heading
	if dist < 1500 {
		return true
	}

	return false
}

// assessCombatThreats evaluates all threats to the bot
func (s *Server) assessCombatThreats(p *game.Player) CombatThreat {
	threat := CombatThreat{
		closestTorpDist: 999999.0,
		closestPlasma:   999999.0,
		nearbyEnemies:   0,
		requiresEvasion: false,
		threatLevel:     0,
	}

	// Check torpedoes
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID && torp.Status == 1 {
			dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if dist < threat.closestTorpDist {
				threat.closestTorpDist = dist
			}

			// Check if heading toward us
			if dist < 3000 {
				dx := p.X - torp.X
				dy := p.Y - torp.Y
				angleToUs := math.Atan2(dy, dx)
				angleDiff := math.Abs(angleToUs - torp.Dir)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}
				if angleDiff < math.Pi/4 {
					threat.requiresEvasion = true
					threat.threatLevel += 3
				}
			}
		}
	}

	// Check plasma
	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner != p.ID && plasma.Status == 1 {
			dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)
			if dist < threat.closestPlasma {
				threat.closestPlasma = dist
			}
			if dist < 4000 {
				threat.requiresEvasion = true
				threat.threatLevel += 5
			}
		}
	}

	// Check nearby enemies
	for _, enemy := range s.gameState.Players {
		if enemy.Status == game.StatusAlive && enemy.Team != p.Team {
			dist := game.Distance(p.X, p.Y, enemy.X, enemy.Y)
			if dist < 5000 {
				threat.nearbyEnemies++
				threat.threatLevel++

				// Check if enemy is facing us (potential phaser threat)
				angleToUs := math.Atan2(p.Y-enemy.Y, p.X-enemy.X)
				angleDiff := math.Abs(enemy.Dir - angleToUs)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}
				if dist < 2000 && angleDiff < math.Pi/6 {
					threat.requiresEvasion = true
					threat.threatLevel += 2
				}
			}
		}
	}

	return threat
}

// selectCombatManeuver chooses the best combat maneuver based on situation
func (s *Server) selectCombatManeuver(p, target *game.Player, dist float64) CombatManeuver {
	shipStats := game.ShipData[p.Ship]
	targetStats := game.ShipData[target.Ship]

	// Default to intercept
	maneuver := CombatManeuver{
		direction: s.calculateEnhancedInterceptCourse(p, target),
		speed:     s.getOptimalCombatSpeed(p, dist),
		maneuver:  "intercept",
	}

	// Analyze ship matchup
	speedAdvantage := float64(shipStats.MaxSpeed - targetStats.MaxSpeed)
	// Use max speed as proxy for maneuverability (scouts turn better than battleships)
	maneuverAdvantage := speedAdvantage

	if dist < 3000 {
		// Close range - use angular velocity matching for dogfight
		if maneuverAdvantage > 0 {
			// We turn better - circle strafe
			perpendicularAngle := math.Atan2(target.Y-p.Y, target.X-p.X) + math.Pi/2
			maneuver.direction = perpendicularAngle
			maneuver.speed = float64(shipStats.MaxSpeed) * 0.7
			maneuver.maneuver = "circle-strafe"
		} else {
			// They turn better - maintain distance
			if speedAdvantage > 0 {
				// We're faster - boom and zoom
				angleAway := math.Atan2(p.Y-target.Y, p.X-target.X)
				maneuver.direction = angleAway
				maneuver.speed = float64(shipStats.MaxSpeed)
				maneuver.maneuver = "boom-zoom"
			}
		}
	} else if dist > 6000 {
		// Long range - use speed to close or maintain
		if speedAdvantage < 0 && target.Speed > float64(targetStats.MaxSpeed)*0.5 {
			// They're faster and closing - angle for better position
			offsetAngle := s.calculateEnhancedInterceptCourse(p, target) + math.Pi/8
			maneuver.direction = offsetAngle
			maneuver.speed = float64(shipStats.MaxSpeed)
			maneuver.maneuver = "offset-approach"
		}
	}

	return maneuver
}

// managePredictiveShields manages shields with prediction
func (s *Server) managePredictiveShields(p, target *game.Player, enemyDist, torpDist float64) {
	s.assessAndActivateShields(p, target)
}

// assessAndActivateShields provides comprehensive shield assessment for all bot scenarios
func (s *Server) assessAndActivateShields(p *game.Player, primaryTarget *game.Player) {
	shipStats := game.ShipData[p.Ship]

	// Don't shield if very low on fuel (emergency threshold)
	if p.Fuel < 600 {
		p.Shields_up = false
		return
	}

	// Don't shield if we're trying to repair at a starbase with high damage
	if p.Orbiting >= 0 && p.Damage > shipStats.MaxDamage/2 {
		p.Shields_up = false
		return
	}

	// Initialize threat assessment
	threatLevel := 0
	closestTorpDist := 999999.0
	closestEnemyDist := 999999.0
	immediateThreat := false

	// Check all torpedo threats
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID && torp.Status == 1 {
			dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if dist < closestTorpDist {
				closestTorpDist = dist
			}

			// Torpedo threat levels based on distance and trajectory
			if dist < 3000 {
				threatLevel += 2
				if s.isTorpedoThreatening(p, torp) {
					threatLevel += 4
					immediateThreat = true
				}
			}

			// Very close torpedoes are always dangerous
			if dist < 1500 {
				threatLevel += 5
				immediateThreat = true
			}
		}
	}

	// Check all enemy players for phaser threats and proximity
	for _, enemy := range s.gameState.Players {
		if enemy.Status == game.StatusAlive && enemy.Team != p.Team {
			dist := game.Distance(p.X, p.Y, enemy.X, enemy.Y)
			if dist < closestEnemyDist {
				closestEnemyDist = dist
			}

			enemyStats := game.ShipData[enemy.Ship]
			phaserRange := float64(game.PhaserDist * enemyStats.PhaserDamage / 100)

			// Within phaser range - high priority for shields
			// Phasers can be fired in any direction regardless of ship facing
			if dist < phaserRange {
				threatLevel += 3
				// Any enemy within phaser range is an immediate threat
				if dist < phaserRange*0.8 { // Within 80% of phaser range
					threatLevel += 4
					immediateThreat = true
				}
			}

			// Very close enemies are dangerous regardless of facing
			if dist < 1800 {
				threatLevel += 3
				immediateThreat = true
			}
		}
	}

	// Check plasma threats
	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner != p.ID && plasma.Status == 1 {
			dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)
			if dist < 4000 {
				threatLevel += 3
				if dist < 2000 {
					threatLevel += 4
					immediateThreat = true
				}
			}
		}
	}

	// Shield decision logic based on threat assessment and fuel availability
	shouldShield := false

	// Immediate threats - shield if we have minimal fuel
	if immediateThreat && p.Fuel > 800 {
		shouldShield = true
	} else if threatLevel >= 6 && p.Fuel > 1200 {
		// High threat level - shield up
		shouldShield = true
	} else if threatLevel >= 3 && p.Fuel > 1800 {
		// Medium threat with good fuel reserves
		shouldShield = true
	} else if closestTorpDist < 2000 && p.Fuel > 1000 {
		// Torpedo nearby - be defensive
		shouldShield = true
	} else if closestEnemyDist < 2500 && p.Fuel > 1500 {
		// Enemy nearby - be prepared
		shouldShield = true
	}

	// Special case: always shield when carrying armies and threatened
	if p.Armies > 0 && (closestEnemyDist < 3500 || closestTorpDist < 3000) && p.Fuel > 1000 {
		shouldShield = true
	}

	// Special case: shield during planet defense when enemies are close
	if p.BotDefenseTarget >= 0 && (closestEnemyDist < 3000 || closestTorpDist < 2500) && p.Fuel > 1200 {
		shouldShield = true
	}

	p.Shields_up = shouldShield
}

// shouldUseCloaking determines if bot should cloak
func (s *Server) shouldUseCloaking(p, target *game.Player, dist float64) bool {
	// Don't cloak if too close (they can see us)
	if dist < 1500 {
		return false
	}

	// Cloak for ambush when approaching
	if dist > 3000 && dist < 7000 && p.Damage < 20 {
		return true
	}

	// Cloak to escape when damaged
	shipStats := game.ShipData[p.Ship]
	if p.Damage > shipStats.MaxDamage/2 && dist > 2000 {
		return true
	}

	return false
}

// shouldDodgeAdvanced checks if dodging is necessary with more sophisticated logic
func (s *Server) shouldDodgeAdvanced(p *game.Player, desiredDir float64) bool {
	// Check current damage at desired direction
	damage := s.calculateDamageAtDirection(p, desiredDir, p.DesSpeed)
	return damage > 0
}

// manageBotShields manages shields intelligently based on threats
func (s *Server) manageBotShields(p *game.Player, enemyDist, torpDist float64) {
	// Shield up if:
	// - Torpedoes are close
	// - Enemy is close and we're in combat
	// - Taking damage
	shouldShield := false

	if torpDist < 2100 && p.Fuel > 1000 {
		shouldShield = true
	} else if enemyDist < 3500 && p.Fuel > 2000 {
		shouldShield = true
	} else if enemyDist < 6000 && p.Fuel > 5000 {
		shouldShield = true
	}

	// Shield down if low on fuel
	if p.Fuel < 1000 {
		shouldShield = false
	} else if p.Fuel < 2000 && enemyDist > 8000 {
		shouldShield = false
	}

	p.Shields_up = shouldShield
}
