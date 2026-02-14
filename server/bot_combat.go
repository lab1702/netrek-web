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
		if p.Orbiting < game.MaxPlanets {
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

	// Try to phaser any plasma in range before evading
	if threats.closestPlasma < MaxSearchDistance {
		if s.tryPhaserNearbyPlasma(p) {
			p.BotCooldown = 5 // Short cooldown after phasering plasma
			// Continue with other combat logic, don't return
		}
	}

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

	// Enhanced team coordination with target broadcasting — only apply if the
	// coordinated cooldown is at least as high as the current cooldown, to prevent
	// integer division from making bots fire faster than their weapon logic intends.
	if coordinatedCooldown := s.coordinateTeamAttack(p, target); coordinatedCooldown > 0 && coordinatedCooldown >= p.BotCooldown {
		p.BotCooldown = coordinatedCooldown // Sync with team for volley fire
	}

	// Broadcast high-value targets to nearby allies
	s.broadcastTargetToAllies(p, target, p.BotTargetValue)

	// Check for torpedo detonation opportunities for area denial
	// Only detonate specific torpedoes that are passing by enemies, not all in-flight
	s.detonatePassingTorpedoes(p)

	// Weapon usage - no facing restrictions needed

	// Enhanced torpedo firing with prediction and spread patterns
	// Torpedoes can be fired in any direction regardless of ship facing
	// Use velocity-adjusted range to prevent fuse expiry on fast targets
	effectiveTorpRange := s.getVelocityAdjustedTorpRange(p, target)

	// Verify the torpedo can actually reach the intercept point before fuse expires.
	// This prevents firing at targets moving away where the intercept point is beyond range.
	canReachTarget := s.canTorpReachTarget(p, target)

	// Check for burst fire opportunity on vulnerable targets
	targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)
	burstFireMode := targetDamageRatio > 0.7 && dist < effectiveTorpRange*0.6 // Burst when target is heavily damaged and in close range
	firedTorps := false
	if canReachTarget && dist < effectiveTorpRange && p.NumTorps < game.MaxTorps-2 && p.Fuel > 1500 && p.WTemp < shipStats.MaxWpnTemp-100 {
		if burstFireMode && p.NumTorps < game.MaxTorps-6 && p.Fuel > 2500 {
			// Burst fire mode - rapid successive torpedoes for kill securing
			s.fireTorpedoSpread(p, target, 4) // Fire 4-torpedo burst
			p.BotCooldown = 2                 // Very short cooldown for follow-up
		} else {
			// Use spread pattern at medium range for area denial
			midRangeLow := effectiveTorpRange * 0.45  // ~45% of effective range
			midRangeHigh := effectiveTorpRange * 0.75 // ~75% of effective range
			if dist > midRangeLow && dist < midRangeHigh && p.NumTorps < game.MaxTorps-4 {
				s.fireTorpedoSpread(p, target, 3)
				p.BotCooldown = 5 // Reduced from 8 to 5 for higher fire rate
			} else {
				s.fireEnhancedTorpedo(p, target)
				p.BotCooldown = 3 // Reduced from 6 to 3 for aggressive combat
			}
		}
		firedTorps = true
	}

	// Fire when enemy is running away - only if we didn't already fire torpedoes above
	if !firedTorps && canReachTarget && dist < effectiveTorpRange && p.NumTorps < game.MaxTorps-3 && p.Fuel > 1000 {
		targetAngleToUs := math.Atan2(p.Y-target.Y, p.X-target.X)
		targetRunAngle := math.Abs(target.Dir - targetAngleToUs)
		if targetRunAngle > math.Pi {
			targetRunAngle = 2*math.Pi - targetRunAngle
		}
		if targetRunAngle < math.Pi/3 && target.Speed > float64(shipStats.MaxSpeed)*0.5 {
			s.fireBotTorpedoWithLead(p, target)
			p.BotCooldown = 4 // Reduced from 8 to 4
		}
	}

	// Enhanced phaser timing with kill securing
	// Phasers can be fired in any direction regardless of ship facing
	myPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0
	if dist < myPhaserRange {
		phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
		if p.Fuel >= phaserCost && p.WTemp < shipStats.MaxWpnTemp-100 { // Match human firing threshold
			targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)
			// Calculate if phaser would be a kill shot
			phaserDamage := float64(shipStats.PhaserDamage) * (1.0 - dist/myPhaserRange)
			wouldKill := target.Damage+int(phaserDamage) >= targetStats.MaxDamage

			// More aggressive phaser usage
			if wouldKill || targetDamageRatio > 0.5 || dist < 1500 || target.Cloaked {
				s.fireBotPhaser(p, target)
				p.BotCooldown = 5 // Reduced from 10 to 5
			}
		}
	}

	// Predictive shield management
	s.managePredictiveShields(p, target, dist, closestTorpDist)

	// Enhanced plasma usage for area control - use actual plasma range, not torpedo range
	if shipStats.HasPlasma && p.NumPlasma < 1 && p.Fuel > 3000 { // Lower fuel threshold
		// Use actual plasma maximum range to prevent fuse expiry
		maxPlasmaRange := game.MaxPlasmaRangeForShip(p.Ship)
		plasmaLongRange := game.EffectivePlasmaRange(p.Ship, 0.85)  // 85% of max for long range
		plasmaShortRange := game.EffectivePlasmaRange(p.Ship, 0.30) // 30% of max for minimum range
		plasmaKillRange := game.EffectivePlasmaRange(p.Ship, 0.75)  // 75% of max for kill shots

		// Only fire if target is within actual plasma range
		if dist < maxPlasmaRange &&
			((dist < plasmaLongRange && dist > plasmaShortRange && target.Speed < 4) ||
				(targetDamageRatio > 0.6 && dist < plasmaKillRange) || // Lower damage threshold
				(target.Orbiting >= 0 && dist < plasmaKillRange)) {
			s.fireBotPlasma(p, target)
			p.BotCooldown = 12 // Reduced from 20 to 12
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

// assessUniversalThreats evaluates all threats to the bot (for both combat and navigation)
func (s *Server) assessUniversalThreats(p *game.Player) CombatThreat {
	threat := CombatThreat{
		closestTorpDist: MaxSearchDistance,
		closestPlasma:   MaxSearchDistance,
		nearbyEnemies:   0,
		requiresEvasion: false,
		threatLevel:     0,
	}

	// Enhanced torpedo checking for all movement scenarios
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID && torp.Team != p.Team && torp.Status == game.TorpMove {
			dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if dist < threat.closestTorpDist {
				threat.closestTorpDist = dist
			}

			// Improved torpedo threat prediction
			if s.isTorpedoThreatening(p, torp) {
				threat.requiresEvasion = true
				threat.threatLevel += 4

				// Increase threat level based on proximity — always applied
				// regardless of planet proximity (fixes bug where open-space
				// torpedo threats were zeroed by the planet multiplier).
				baseThreatIncrease := 0
				if dist < 2000 {
					baseThreatIncrease = 3
				} else if dist < 4000 {
					baseThreatIncrease = 1
				}
				threat.threatLevel += baseThreatIncrease

				// Additional bonus when near a planet — torpedoes are more
				// dangerous in contested planet areas
				for _, planet := range s.gameState.Planets {
					pDistToPlanet := game.Distance(p.X, p.Y, planet.X, planet.Y)
					torpDistToPlanet := game.Distance(torp.X, torp.Y, planet.X, planet.Y)

					if pDistToPlanet < 10000 && torpDistToPlanet < 10000 {
						threat.threatLevel += baseThreatIncrease // Double the proximity bonus near planets
						break
					}
				}
			}
		}
	}

	// Check plasma threats (skip friendly plasma)
	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner != p.ID && plasma.Team != p.Team && plasma.Status == game.TorpMove {
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

	// Player position and velocity - use ACTUAL current movement for collision prediction
	// DesDir/DesSpeed represent where the ship wants to go, but Dir/Speed are
	// where it's actually going right now (turning is gradual)
	playerSpeed := p.Speed * 20 // Convert to units per tick
	playerVelX := playerSpeed * math.Cos(p.Dir)
	playerVelY := playerSpeed * math.Sin(p.Dir)

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

// selectCombatManeuver chooses the best combat maneuver based on situation
func (s *Server) selectCombatManeuver(p, target *game.Player, dist float64) CombatManeuver {
	// Validate target is still alive before computing maneuvers
	if target.Status != game.StatusAlive {
		// Target is dead — just maintain current heading
		return CombatManeuver{
			direction: p.DesDir,
			speed:     s.getOptimalCombatSpeed(p, dist),
			maneuver:  "idle",
		}
	}

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

// assessAndActivateShields provides comprehensive shield assessment for all bot scenarios.
// Uses BotShieldFrame to run at most once per game tick (avoids redundant iterations
// over torpedoes, plasmas, and enemies when called from multiple code paths).
func (s *Server) assessAndActivateShields(p *game.Player, primaryTarget *game.Player) {
	// Skip if already assessed this tick (Frame starts at 1 in game loop,
	// BotShieldFrame zero-value means "never assessed")
	if p.BotShieldFrame > 0 && p.BotShieldFrame == s.gameState.Frame {
		return
	}
	p.BotShieldFrame = s.gameState.Frame

	// Don't shield if critically low on fuel (emergency threshold)
	if p.Fuel < FuelCritical {
		p.Shields_up = false
		return
	}

	// Remove the orbit repair shield drop - bots were dying during base repair
	// Let threat assessment handle shield decisions even while repairing

	// Initialize threat assessment
	threatLevel := 0
	closestTorpDist := MaxSearchDistance
	closestEnemyDist := MaxSearchDistance
	immediateThreat := false

	// Check all torpedo threats (skip friendly torpedoes)
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID && torp.Team != p.Team && torp.Status == game.TorpMove {
			dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if dist < closestTorpDist {
				closestTorpDist = dist
			}

			// Torpedo threat levels based on distance and trajectory
			if dist < TorpedoClose {
				threatLevel += 2
				if s.isTorpedoThreatening(p, torp) {
					threatLevel += TorpedoThreatBonus
					immediateThreat = true
				}
			}

			// Very close torpedoes are always dangerous (increased range)
			if dist < TorpedoVeryClose {
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
			phaserRange := float64(game.PhaserDist) * float64(enemyStats.PhaserDamage) / 100.0

			// Within phaser range - high priority for shields
			// Phasers can be fired in any direction regardless of ship facing
			if dist < phaserRange {
				threatLevel += ThreatLevelMedium
				// Any enemy within phaser range is an immediate threat
				if dist < phaserRange*PhaserRangeFactor {
					threatLevel += ThreatLevelHigh
					immediateThreat = true
				}
			}

			// Very close enemies are dangerous regardless of facing
			if dist < EnemyVeryClose {
				threatLevel += CloseEnemyBonus
				immediateThreat = true
			}

			// Always treat any enemy within EnemyClose range as immediate threat
			if dist < EnemyClose {
				immediateThreat = true
				threatLevel += 2
			}
		}
	}

	// Check plasma threats (skip friendly plasma)
	for _, plasma := range s.gameState.Plasmas {
		if plasma.Owner != p.ID && plasma.Team != p.Team && plasma.Status == game.TorpMove {
			dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)
			if dist < PlasmaFar {
				threatLevel += ThreatLevelMedium
				if dist < PlasmaClose {
					threatLevel += ThreatLevelHigh
					immediateThreat = true
				}
			}
		}
	}

	// Shield decision logic based on threat assessment and fuel availability
	shouldShield := false

	// Immediate threats - shield if we have minimal fuel (much lower threshold)
	if immediateThreat && p.Fuel > FuelLow {
		shouldShield = true
	} else if threatLevel >= ThreatLevelImmediate && p.Fuel > FuelModerate {
		// High threat level - shield up
		shouldShield = true
	} else if threatLevel >= ThreatLevelMedium && p.Fuel > FuelGood {
		// Medium threat with good fuel reserves
		shouldShield = true
	} else if closestTorpDist < TorpedoVeryClose && p.Fuel > FuelLow {
		// Torpedo very close - be defensive with lower fuel requirement
		shouldShield = true
	} else if closestEnemyDist < EnemyClose && p.Fuel > FuelModerate {
		// Enemy nearby - be prepared with moderate fuel requirement
		shouldShield = true
	}

	// Special case: always shield when carrying armies and threatened (lower fuel requirement)
	if p.Armies > 0 && (closestEnemyDist < ArmyCarryingRange || closestTorpDist < TorpedoClose) && p.Fuel > FuelLow {
		shouldShield = true
	}

	// Special case: shield during planet defense when enemies are close (lower fuel requirement)
	if p.BotDefenseTarget >= 0 && (closestEnemyDist < DefenseShieldRange || closestTorpDist < TorpedoVeryClose) && p.Fuel > FuelLow {
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

	shipStats := game.ShipData[p.Ship]
	damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)

	// Cloak for ambush when approaching (only if lightly damaged)
	if dist > 3000 && dist < 7000 && damageRatio < 0.2 {
		return true
	}

	// Cloak to escape when damaged
	if damageRatio > 0.5 && dist > 2000 {
		return true
	}

	return false
}
