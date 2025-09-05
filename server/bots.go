package server

import (
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"math"
	"math/rand"
)

// AddBot adds a new bot player to the game
func (s *Server) AddBot(team, ship int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find a free player slot
	var botID = -1
	for i := 0; i < game.MaxPlayers; i++ {
		if s.gameState.Players[i].Status == game.StatusFree && !s.gameState.Players[i].Connected {
			botID = i
			break
		}
	}

	if botID == -1 {
		return // No free slots
	}

	// Initialize bot player
	p := s.gameState.Players[botID]
	p.ID = botID
	p.Name = fmt.Sprintf("[BOT] %s", BotNames[rand.Intn(len(BotNames))])
	p.Team = team
	p.Ship = game.ShipType(ship)
	p.Status = game.StatusAlive
	p.NextShipType = -1 // Ensure no pending refit; preserve ship on respawn

	p.Connected = true
	p.IsBot = true
	p.BotTarget = -1
	p.BotPlanetApproachID = -1
	p.BotDefenseTarget = -1
	p.BotCooldown = 0

	// Set initial position based on team
	p.X = float64(game.TeamHomeX[team]) + float64(rand.Intn(10000)-5000)
	p.Y = float64(game.TeamHomeY[team]) + float64(rand.Intn(10000)-5000)
	p.Dir = rand.Float64() * 2 * math.Pi

	// Initialize ship stats
	shipStats := game.ShipData[p.Ship]
	p.Shields = shipStats.MaxShields
	p.Damage = 0
	p.Fuel = shipStats.MaxFuel
	p.WTemp = 0
	p.ETemp = 0
	p.Speed = 0
	p.DesSpeed = 0
	p.Shields_up = true
	p.NumTorps = 0
	p.NumPlasma = 0

	// Announce bot joined
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("%s has joined the game", formatPlayerName(p)),
			"type": "info",
			"from": botID,
		},
	}
}

// UpdateBots updates all bot players' AI
func (s *Server) UpdateBots() {
	for _, p := range s.gameState.Players {
		if !p.IsBot || p.Status != game.StatusAlive {
			continue
		}

		// Reduce cooldown
		if p.BotCooldown > 0 {
			p.BotCooldown--
		}

		// Safety check: Fix stuck bombing state
		if p.Bombing && p.Orbiting >= 0 && p.Orbiting < len(s.gameState.Planets) {
			planet := s.gameState.Planets[p.Orbiting]
			// Stop bombing if planet is friendly or has no armies
			if planet.Owner == p.Team || planet.Armies == 0 {
				p.Bombing = false
				p.BotCooldown = 5 // Small cooldown to re-evaluate
			}
		}

		// Run hard mode AI for all bots
		s.updateBotHard(p)
	}
}

// updateBotHard implements hard difficulty AI with different behaviors for tournament/non-tournament modes
func (s *Server) updateBotHard(p *game.Player) {
	if p.BotCooldown > 0 {
		return
	}

	// STARBASE-SPECIFIC AI: Cautious and defensive behavior
	if p.Ship == game.ShipStarbase {
		s.updateStarbaseBot(p)
		return
	}

	// HIGHEST PRIORITY: Planet defense - check for friendly planets under immediate threat
	if planet, enemy, enemyDist := s.getThreatenedFriendlyPlanet(p); planet != nil && enemy != nil {
		s.defendPlanet(p, planet, enemy, enemyDist)
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Find strategic planets (like borgmove.c find_planets)
	repairPlanet := s.findNearestRepairPlanet(p)
	fuelPlanet := s.findNearestFuelPlanet(p)
	armyPlanet := s.findNearestArmyPlanet(p)
	enemyArmyPlanet := s.findNearestEnemyArmyPlanet(p)
	takePlanet := s.findBestPlanetToTake(p)

	// Check repair/fuel needs with strategic decisions
	needRepair := p.Damage > shipStats.MaxDamage/2
	needFuel := p.Fuel < shipStats.MaxFuel/3
	criticalDamage := p.Damage > shipStats.MaxDamage*3/4

	nearestEnemy := s.findNearestEnemy(p)
	enemyDist := 999999.0
	if nearestEnemy != nil {
		enemyDist = game.Distance(p.X, p.Y, nearestEnemy.X, nearestEnemy.Y)
	}

	// Check if currently orbiting for repair/fuel
	if p.Orbiting >= 0 && p.Orbiting < len(s.gameState.Planets) {
		orbitPlanet := s.gameState.Planets[p.Orbiting]
		if orbitPlanet.Owner == p.Team {
			// Continue repairing if needed and safe
			if (needRepair || needFuel) && enemyDist > 8000 {
				p.DesSpeed = 0
				p.Shields_up = false
				// Activate repair mode if damaged over 50%
				if needRepair && !p.Repairing {
					p.Repairing = true
				}
				p.BotCooldown = 20
				return
			}
		}
	}

	// Use repair mode when safe and over 50% damaged even without orbiting
	if needRepair && enemyDist > 10000 && !p.Repairing && p.Speed < 2 {
		// Safe to repair - activate repair mode
		p.Repairing = true
		p.RepairRequest = false
		p.DesSpeed = 0
		p.Shields_up = false
		p.BotCooldown = 30
		return
	}

	// Cancel repair mode if threatened
	if p.Repairing && enemyDist < 8000 {
		p.Repairing = false
		p.RepairRequest = false
	}

	// Check if we were trying to approach a planet but got sidetracked fighting defenders
	if p.BotPlanetApproachID >= 0 && p.BotPlanetApproachID < len(s.gameState.Planets) {
		approachPlanet := s.gameState.Planets[p.BotPlanetApproachID]
		defenderInfo := s.detectPlanetDefenders(approachPlanet, p.Team)

		// Check if defenders are cleared or pushed far enough away
		defendersCleared := defenderInfo.DefenderCount == 0 || defenderInfo.MinDefenderDist > 10000

		// Check if our target is dead or far away (meaning we've successfully engaged)
		targetStillThreatening := false
		if p.BotTarget >= 0 && p.BotTarget < game.MaxPlayers {
			target := s.gameState.Players[p.BotTarget]
			if target.Status == game.StatusAlive {
				targetDist := game.Distance(p.X, p.Y, target.X, target.Y)
				planetDist := game.Distance(target.X, target.Y, approachPlanet.X, approachPlanet.Y)
				// Target is still threatening if alive, close to us, and near the planet
				targetStillThreatening = targetDist < 8000 && planetDist < 12000
			}
		}

		if defendersCleared && !targetStillThreatening {
			// Defenders are cleared, resume planet approach
			dist := game.Distance(p.X, p.Y, approachPlanet.X, approachPlanet.Y)
			if dist < OrbitDistance {
				// Close enough to planet, clear approach ID and let normal logic take over
				p.BotPlanetApproachID = -1
			} else {
				// Navigate back to the planet
				dx := approachPlanet.X - p.X
				dy := approachPlanet.Y - p.Y
				baseDir := math.Atan2(dy, dx)
				desiredSpeed := s.getOptimalSpeed(p, dist)

				s.applySafeNavigation(p, baseDir, desiredSpeed, "resuming planet approach")
				p.BotCooldown = 5
				return
			}
		}
	}

	// Repair/fuel decision based on threat level
	if (needRepair || needFuel) && (enemyDist > 15000 || criticalDamage) {
		var targetPlanet *game.Planet
		if needFuel && fuelPlanet != nil {
			targetPlanet = fuelPlanet
		} else if needRepair && repairPlanet != nil {
			targetPlanet = repairPlanet
		}

		if targetPlanet != nil {
			dist := game.Distance(p.X, p.Y, targetPlanet.X, targetPlanet.Y)
			if dist < OrbitDistance {
				// Start orbiting for repair
				p.Orbiting = targetPlanet.ID
				p.DesSpeed = 0
				p.Shields_up = false
				// Activate repair mode if damaged over 50%
				if needRepair && !p.Repairing {
					p.Repairing = true
				}
				p.BotCooldown = 30
				return
			} else {
				// Navigate to repair/fuel planet with torpedo dodging
				p.Orbiting = -1
				dx := targetPlanet.X - p.X
				dy := targetPlanet.Y - p.Y
				baseDir := math.Atan2(dy, dx)
				desiredSpeed := s.getOptimalSpeed(p, dist)

				// Use safe navigation with torpedo dodging
				s.applySafeNavigation(p, baseDir, desiredSpeed, "navigating to repair planet")
				return
			}
		}
	}

	// TOURNAMENT MODE: Prioritize planet conquest
	if s.gameState.T_mode {
		// In tournament mode, focus on strategic objectives

		// If carrying armies, prioritize delivering them to NEUTRAL planets
		if p.Armies > 0 {
			var targetPlanet *game.Planet

			// First, look for neutral planets only
			targetPlanet = s.findNearestNeutralPlanet(p)

			if targetPlanet != nil {
				dist := game.Distance(p.X, p.Y, targetPlanet.X, targetPlanet.Y)

				// Check for defenders around the target planet
				_ = s.detectPlanetDefenders(targetPlanet, p.Team) // Just detect, handled in navigation section

				if dist < OrbitDistance {
					// At planet
					if p.Orbiting != targetPlanet.ID {
						p.Orbiting = targetPlanet.ID
						p.DesSpeed = 0
					}

					// Neutral planets should have no armies, just beam down
					p.Bombing = false
					p.Beaming = true
					p.BeamingUp = false
					p.BotCooldown = 50
					return
				} else {
					// Navigate to neutral planet with torpedo dodging
					p.Orbiting = -1
					p.Bombing = false
					p.Beaming = false
					p.BeamingUp = false
					dx := targetPlanet.X - p.X
					dy := targetPlanet.Y - p.Y
					baseDir := math.Atan2(dy, dx)
					desiredSpeed := s.getOptimalSpeed(p, dist)

					// Use safe navigation with torpedo dodging
					s.applySafeNavigation(p, baseDir, desiredSpeed, "navigating to neutral planet")

					// Defend against nearby enemies while carrying
					if enemyDist < 5000 {
						s.defendWhileCarrying(p, nearestEnemy)
					}
					return
				}
			} else {
				// No neutral planets available - wait in safe area
				s.moveToSafeArea(p)
				return
			}
		}

		// Not carrying armies - get armies or bomb enemy planets
		var targetPlanet *game.Planet

		// Check if currently bombing an enemy planet - finish the job first
		if p.Bombing && p.Orbiting >= 0 && p.Orbiting < len(s.gameState.Planets) {
			currentPlanet := s.gameState.Planets[p.Orbiting]
			if currentPlanet.Owner != p.Team && currentPlanet.Owner != game.TeamNone && currentPlanet.Armies > 0 {
				// Still bombing an enemy planet - continue unless in extreme danger
				if enemyDist < 2000 && p.Damage > game.ShipData[p.Ship].MaxDamage*2/3 {
					// Only leave if in extreme danger
					p.Orbiting = -1
					p.Bombing = false
				} else {
					// Continue bombing
					p.BotCooldown = 10
					return
				}
			}
		}

		// First priority: Pick up armies if we have kills
		if p.KillsStreak >= game.ArmyKillRequirement && armyPlanet != nil {
			targetPlanet = armyPlanet
		} else if enemyArmyPlanet != nil {
			// Second priority: Bomb enemy planets with armies
			targetPlanet = enemyArmyPlanet
		} else if takePlanet != nil && p.KillsStreak >= game.ArmyKillRequirement {
			// Third priority: Take neutral/enemy planets (only if we have kills to potentially carry)
			targetPlanet = takePlanet
		} else if nearestEnemy != nil && enemyDist < 20000 {
			// Fourth priority: Find enemies to fight to get kills
			s.engageCombat(p, nearestEnemy, enemyDist)
			return
		}

		if targetPlanet != nil {
			dist := game.Distance(p.X, p.Y, targetPlanet.X, targetPlanet.Y)

			// Check for defenders around the target planet
			defenderInfo := s.detectPlanetDefenders(targetPlanet, p.Team)

			if dist < OrbitDistance {
				// At planet - perform appropriate action
				if p.Orbiting != targetPlanet.ID {
					p.Orbiting = targetPlanet.ID
					p.DesSpeed = 0
				}

				if targetPlanet.Owner == p.Team {
					// Friendly planet - beam up armies (leave at least 1 for defense)
					// Requires 2 kills to pick up armies in classic Netrek
					if targetPlanet.Armies > 1 && p.Armies < s.getBotArmyCapacity(p) && p.KillsStreak >= game.ArmyKillRequirement {
						p.Bombing = false // Stop bombing if planet is now friendly
						p.Beaming = true
						p.BeamingUp = true
						p.BotCooldown = 50
					} else {
						// Can't beam up (no kill streak or full), leave orbit and find enemies
						p.Bombing = false
						p.Beaming = false
						p.BeamingUp = false
						p.Orbiting = -1
						p.BotCooldown = 10
						// Look for combat opportunities
						if nearestEnemy != nil && p.KillsStreak < game.ArmyKillRequirement {
							s.engageCombat(p, nearestEnemy, enemyDist)
							return
						}
					}
				} else {
					// Enemy or neutral planet - check if it still needs bombing
					if targetPlanet.Owner != game.TeamNone && targetPlanet.Owner != p.Team && targetPlanet.Armies > 0 {
						// Enemy planet with armies - keep bombing it
						p.Bombing = true
						p.Beaming = false
						p.BeamingUp = false
						p.BotCooldown = 10 // Reduced from 100 to re-evaluate sooner
					} else if targetPlanet.Armies == 0 || targetPlanet.Owner == game.TeamNone {
						// No armies or neutral planet
						p.Bombing = false // Stop bombing if no armies left
						if p.Armies > 0 {
							// Beam down to take it
							p.Beaming = true
							p.BeamingUp = false
							p.BotCooldown = 10 // Reduced from 50 to be more responsive
						} else {
							// No armies to beam down, leave orbit
							p.Beaming = false
							p.BeamingUp = false
							p.Orbiting = -1
							p.BotCooldown = 10
						}
					}
				}
				return
			} else {
				// Determine if we should engage defenders before approaching planet
				const DANGER_THRESHOLD = 2500.0  // Defense score threshold for engaging
				const MIN_SAFE_DISTANCE = 6000.0 // Distance threshold for safe approach

				shouldEngageDefenders := false
				var primaryDefender *game.Player = nil

				// Check if we should engage defenders first
				if defenderInfo.DefenderCount > 0 {
					// Engage if defense score is high or closest defender is too close
					if defenderInfo.DefenseScore > DANGER_THRESHOLD || defenderInfo.MinDefenderDist < MIN_SAFE_DISTANCE {
						shouldEngageDefenders = true

						// Select primary defender: prioritize carriers, then closest
						for _, defender := range defenderInfo.Defenders {
							if defender.Armies > 0 {
								primaryDefender = defender
								break // Carriers are top priority
							}
						}
						if primaryDefender == nil {
							primaryDefender = defenderInfo.ClosestDefender
						}
					}

					// Abort if too many defenders and no allies nearby
					if defenderInfo.DefenderCount >= 3 {
						alliesNearby := 0
						for _, ally := range s.gameState.Players {
							if ally.Status == game.StatusAlive && ally.Team == p.Team && ally.ID != p.ID {
								allyDist := game.Distance(p.X, p.Y, ally.X, ally.Y)
								if allyDist < 15000 {
									alliesNearby++
								}
							}
						}
						if alliesNearby == 0 {
							// Too dangerous, abort this planet
							p.BotPlanetApproachID = -1
							p.BotCooldown = 50 // Look for different target
							return
						}
					}
				}

				if shouldEngageDefenders && primaryDefender != nil {
					// Set planet approach ID so we can resume after clearing defender
					p.BotPlanetApproachID = targetPlanet.ID

					// Clear planet-specific states
					p.Orbiting = -1
					p.Bombing = false
					p.Beaming = false
					p.BeamingUp = false

					// Engage the primary defender instead of going to planet
					defenderDist := game.Distance(p.X, p.Y, primaryDefender.X, primaryDefender.Y)
					s.engageCombat(p, primaryDefender, defenderDist)
					return
				} else {
					// Safe to approach planet directly or no significant defenders
					p.Orbiting = -1
					p.Bombing = false
					p.Beaming = false
					p.BeamingUp = false
					p.BotPlanetApproachID = targetPlanet.ID // Track our objective

					dx := targetPlanet.X - p.X
					dy := targetPlanet.Y - p.Y
					baseDir := math.Atan2(dy, dx)
					desiredSpeed := s.getOptimalSpeed(p, dist)

					// Use safe navigation with torpedo dodging
					s.applySafeNavigation(p, baseDir, desiredSpeed, "navigating to planet")

					// Still engage if closest enemy gets too close while navigating
					if enemyDist < 4000 {
						s.engageCombat(p, nearestEnemy, enemyDist)
						return
					}
				}
				return
			}
		}

		// No good planet targets, engage nearest enemy
		if nearestEnemy != nil && enemyDist < 15000 {
			s.engageCombat(p, nearestEnemy, enemyDist)
			return
		}
	}

	// NON-TOURNAMENT MODE: Prioritize combat for practice
	if !s.gameState.T_mode {
		// Skip behavior role switching while actively defending planets
		if p.BotDefenseTarget >= 0 {
			// Currently defending a planet - continue current behavior
			// This prevents bots from abandoning defense mid-fight
			p.BotCooldown = 10
			return
		}

		// Dynamic behavior based on situation
		behavior := s.selectBotBehavior(p)

		switch behavior {
		case "hunter":
			// Aggressive enemy hunting
			if target := s.selectBestCombatTarget(p); target != nil {
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				s.engageCombat(p, target, dist)
				return
			}

		case "defender":
			// Defend friendly planets
			if planet := s.findPlanetToDefend(p); planet != nil {
				dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
				if dist > 5000 {
					// Move to defend with torpedo dodging
					dx := planet.X - p.X
					dy := planet.Y - p.Y
					baseDir := math.Atan2(dy, dx)
					desiredSpeed := float64(shipStats.MaxSpeed)

					// Use safe navigation with torpedo dodging
					s.applySafeNavigation(p, baseDir, desiredSpeed, "moving to defend planet")
				} else {
					// Patrol around planet with torpedo dodging
					patrolAngle := math.Mod(float64(rand.Intn(360))*math.Pi/180, math.Pi*2)
					desiredSpeed := float64(shipStats.MaxSpeed) * 0.7

					// Use safe navigation with torpedo dodging
					s.applySafeNavigation(p, patrolAngle, desiredSpeed, "patrolling around planet")
				}
				return
			}

		case "raider":
			// Hit and run tactics on enemy infrastructure
			if planet := s.findPlanetToRaid(p); planet != nil {
				dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
				if dist < OrbitDistance {
					// Quick bomb and run
					if planet.Armies > 0 && planet.Owner != p.Team {
						p.Orbiting = planet.ID
						p.Bombing = true
						p.DesSpeed = 0
						p.BotCooldown = 30
					} else {
						// Move to next target
						p.BotCooldown = 5
					}
				} else {
					// Approach at high speed with torpedo dodging
					dx := planet.X - p.X
					dy := planet.Y - p.Y
					baseDir := math.Atan2(dy, dx)
					desiredSpeed := float64(shipStats.MaxSpeed)

					// Use safe navigation with torpedo dodging
					s.applySafeNavigation(p, baseDir, desiredSpeed, "approaching planet to raid")
				}
				return
			}
		}

		// Fallback to combat if no specific role
		if nearestEnemy != nil {
			s.engageCombat(p, nearestEnemy, enemyDist)
			return
		}

		// Intelligent patrol patterns
		s.executePatrol(p)
		return
	}
}

// updateStarbaseBot implements specialized AI for starbase bots
// Starbases are cautious, defensive, and focused on protecting territory
func (s *Server) updateStarbaseBot(p *game.Player) {
	shipStats := game.ShipData[p.Ship]

	// HIGHEST PRIORITY: Planet defense - check for friendly planets under immediate threat
	if planet, enemy, enemyDist := s.getThreatenedFriendlyPlanet(p); planet != nil && enemy != nil {
		s.starbaseDefendPlanet(p, planet, enemy, enemyDist)
		return
	}

	// Basic needs assessment
	needRepair := p.Damage > shipStats.MaxDamage/3 // More conservative repair threshold
	needFuel := p.Fuel < shipStats.MaxFuel/2       // More conservative fuel threshold
	criticalDamage := p.Damage > shipStats.MaxDamage*2/3

	nearestEnemy := s.findNearestEnemy(p)
	enemyDist := 999999.0
	if nearestEnemy != nil {
		enemyDist = game.Distance(p.X, p.Y, nearestEnemy.X, nearestEnemy.Y)
	}

	// Priority 2: Combat overrides all other behaviors when enemy is in detection range
	if nearestEnemy != nil && enemyDist < game.StarbaseEnemyDetectRange {
		s.starbaseDefensiveCombat(p, nearestEnemy, enemyDist)
		return
	}

	// Count team's planet ownership to determine strategic posture
	teamPlanets := s.countPlanetsForTeam(p.Team)
	totalPlanets := len(s.gameState.Planets)
	teamOwnership := float64(teamPlanets) / float64(totalPlanets)

	// Find strategic locations
	corePlanet := s.findNearestCorePlanet(p)
	repairPlanet := s.findNearestRepairPlanet(p)
	fuelPlanet := s.findNearestFuelPlanet(p)
	threatenedPlanet := s.findMostThreatenedFriendlyPlanet(p)

	// Critical needs - get to safety first
	if criticalDamage || (needRepair && enemyDist < game.StarbaseEnemyDetectRange) {
		var safetyPlanet *game.Planet
		if repairPlanet != nil {
			safetyPlanet = repairPlanet
		} else if fuelPlanet != nil {
			safetyPlanet = fuelPlanet
		} else if corePlanet != nil {
			safetyPlanet = corePlanet
		}

		if safetyPlanet != nil {
			dist := game.Distance(p.X, p.Y, safetyPlanet.X, safetyPlanet.Y)
			if dist < OrbitDistance {
				// Safe at friendly planet - repair
				p.Orbiting = safetyPlanet.ID
				p.DesSpeed = 0
				p.Shields_up = false
				if needRepair {
					p.Repairing = true
				}
				p.BotCooldown = 30
				return
			} else if enemyDist > game.StarbaseEnemyDetectRange {
				// Move cautiously to safety
				p.Orbiting = -1
				p.Repairing = false
				dx := safetyPlanet.X - p.X
				dy := safetyPlanet.Y - p.Y
				p.DesDir = math.Atan2(dy, dx)
				p.DesSpeed = 2 // Very slow, cautious movement
				p.Shields_up = true
				return
			}
		}
	}

	// Currently orbiting - stay put if it's beneficial
	if p.Orbiting >= 0 && p.Orbiting < len(s.gameState.Planets) {
		orbitPlanet := s.gameState.Planets[p.Orbiting]
		if orbitPlanet.Owner == p.Team {
			// At friendly planet - consider staying
			isCorePlanet := s.isCorePlanet(orbitPlanet, p.Team)
			isSafe := enemyDist > game.StarbaseEnemyDetectRange+3000 || (enemyDist > game.StarbaseTorpRange && isCorePlanet)

			if (needRepair || needFuel) && isSafe {
				// Stay and repair/refuel
				p.DesSpeed = 0
				p.Shields_up = false
				if needRepair {
					p.Repairing = true
				}
				p.BotCooldown = 25
				return
			} else if isSafe && (isCorePlanet || teamOwnership < 0.4) {
				// Stay at core planets or if team doesn't control much territory
				p.DesSpeed = 0
				p.Shields_up = enemyDist < 15000
				// Combat handled by priority check above, no need for duplicate logic
				p.BotCooldown = 15
				return
			}
		}
	}

	// Strategic decision making based on team ownership
	if teamOwnership >= 0.25 {
		// Team controls significant territory - can be more aggressive in defense
		if threatenedPlanet != nil {
			// Move to defend threatened planet
			dist := game.Distance(p.X, p.Y, threatenedPlanet.X, threatenedPlanet.Y)
			if dist > 4000 {
				// Move closer to threatened planet
				p.Orbiting = -1
				p.Repairing = false
				dx := threatenedPlanet.X - p.X
				dy := threatenedPlanet.Y - p.Y
				p.DesDir = math.Atan2(dy, dx)
				p.DesSpeed = 2 // Slow, deliberate movement
				p.Shields_up = true
				return
			} else {
				// Close to threatened planet - defend it
				p.Orbiting = threatenedPlanet.ID
				p.DesSpeed = 0
				// Combat handled by priority check above, no need for duplicate logic
				p.BotCooldown = 10
				return
			}
		}
	} else {
		// Team controls less than 1/4 - stay near core planets
		if corePlanet != nil {
			dist := game.Distance(p.X, p.Y, corePlanet.X, corePlanet.Y)
			if dist > 3000 {
				// Move back to core area
				p.Orbiting = -1
				p.Repairing = false
				dx := corePlanet.X - p.X
				dy := corePlanet.Y - p.Y
				p.DesDir = math.Atan2(dy, dx)
				p.DesSpeed = 2
				p.Shields_up = true
				return
			} else {
				// Near core planet - defend it
				p.Orbiting = corePlanet.ID
				p.DesSpeed = 0
				p.Shields_up = enemyDist < game.StarbaseEnemyDetectRange
				// Combat handled by priority check above, no need for duplicate logic
				p.BotCooldown = 20
				return
			}
		}
	}

	// No specific objective - patrol defensively near friendly planets
	s.starbaseDefensivePatrol(p)
}

// starbaseDefensiveCombat handles combat for starbases - conservative and defensive
func (s *Server) starbaseDefensiveCombat(p *game.Player, enemy *game.Player, dist float64) {
	shipStats := game.ShipData[p.Ship]

	// Always shields up in combat
	p.Shields_up = true

	// Calculate firing solution
	angleToEnemy := math.Atan2(enemy.Y-p.Y, enemy.X-p.X)
	angleDiff := math.Abs(p.Dir - angleToEnemy)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	// Turn towards enemy slowly (starbases turn slowly)
	if angleDiff > 0.1 {
		p.DesDir = angleToEnemy
	}

	// Stay put - starbases don't chase
	p.DesSpeed = 0

	// Fire weapons when aligned and enemy is in range
	if angleDiff < 0.3 {
		// Fire torpedoes at long range
		if dist < game.StarbaseTorpRange && p.NumTorps < game.MaxTorps-1 && p.Fuel > 3000 && p.WTemp < 600 {
			s.fireBotTorpedoWithLead(p, enemy)
			p.BotCooldown = 8 // Faster firing rate for better offense
		}

		// Fire phasers at medium range or to finish enemies
		if dist < game.StarbasePhaserRange && p.Fuel > 2000 && p.WTemp < 700 {
			enemyDamageRatio := float64(enemy.Damage) / float64(game.ShipData[enemy.Ship].MaxDamage)
			if enemyDamageRatio > 0.6 || dist < 4000 {
				s.fireBotPhaser(p, enemy)
				p.BotCooldown = 10 // Faster firing rate
			}
		}

		// Use plasma for area denial
		if shipStats.HasPlasma && p.NumPlasma < 1 && dist < game.StarbasePlasmaMaxRange && dist > game.StarbasePlasmaMinRange && p.Fuel > 4000 {
			s.fireBotPlasma(p, enemy)
			p.BotCooldown = 20 // Slightly faster plasma cycling
		}
	}
}

// starbaseDefensivePatrol makes starbase patrol defensively near friendly territory
func (s *Server) starbaseDefensivePatrol(p *game.Player) {
	// Find center of friendly territory
	centerX, centerY := s.calculateTeamCenter(p.Team)
	dist := game.Distance(p.X, p.Y, centerX, centerY)

	// Stay within 15000 units of team center
	if dist > 15000 {
		// Move back towards team center
		p.Orbiting = -1
		dx := centerX - p.X
		dy := centerY - p.Y
		p.DesDir = math.Atan2(dy, dx)
		p.DesSpeed = 2
		p.Shields_up = true
	} else {
		// Slow patrol pattern
		patrolAngle := math.Mod(float64(s.gameState.Frame)*0.01, 2*math.Pi)
		p.DesDir = patrolAngle
		p.DesSpeed = 1 // Very slow patrol
		p.Shields_up = false
	}

	p.BotCooldown = 30 // Slow decision making
}

// Helper functions for starbase AI

// findNearestCorePlanet finds the nearest core (home) planet
func (s *Server) findNearestCorePlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	nearestDist := 999999.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet != nil && planet.Owner == p.Team && s.isCorePlanet(planet, p.Team) {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < nearestDist {
				nearestDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

// findMostThreatenedFriendlyPlanet finds friendly planet most at risk
func (s *Server) findMostThreatenedFriendlyPlanet(p *game.Player) *game.Planet {
	var mostThreatened *game.Planet
	highestThreat := 0.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet == nil || planet.Owner != p.Team {
			continue
		}

		// Calculate threat level based on nearby enemies
		threatLevel := 0.0
		for _, enemy := range s.gameState.Players {
			if enemy.Team == p.Team || enemy.Status != game.StatusAlive {
				continue
			}
			dist := game.Distance(planet.X, planet.Y, enemy.X, enemy.Y)
			if dist < 15000 {
				// Closer enemies are more threatening
				threatLevel += (15000 - dist) / 15000
				// Carriers are extra threatening
				if enemy.Armies > 0 {
					threatLevel += 0.5
				}
			}
		}

		if threatLevel > highestThreat {
			highestThreat = threatLevel
			mostThreatened = planet
		}
	}

	return mostThreatened
}

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

// RemoveBot removes a bot player from the game
func (s *Server) RemoveBot(botID int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := s.gameState.Players[botID]
	if !p.IsBot {
		return
	}

	p.Status = game.StatusFree
	p.Connected = false
	p.IsBot = false

	// Announce bot left
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("%s has left the game", formatPlayerName(p)),
			"type": "info",
			"from": botID,
		},
	}
}

// Helper functions for bot AI

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

// Planet finding functions
func (s *Server) findNearestRepairPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team && (planet.Flags&game.PlanetRepair) != 0 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

func (s *Server) findNearestFuelPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team && (planet.Flags&game.PlanetFuel) != 0 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

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

// AutoBalanceBots adds or removes bots to balance teams
func (s *Server) AutoBalanceBots() {
	// Count human players per team
	teamCounts := make(map[int]int)
	teamBots := make(map[int][]int)

	for i, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Connected {
			if p.IsBot {
				teamBots[p.Team] = append(teamBots[p.Team], i)
			} else {
				teamCounts[p.Team]++
			}
		}
	}

	// Find team with most players
	maxCount := 0
	for _, count := range teamCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	// Balance teams by adding bots with appropriate ship types
	teams := []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri}
	for _, team := range teams {
		deficit := maxCount - teamCounts[team]
		for deficit > 0 {
			// Choose ship type based on team needs
			ship := s.selectBotShipType(team)
			s.AddBot(team, ship)
			deficit--
		}
	}
}

// starbaseDefendPlanet handles planet defense for starbase bots
func (s *Server) starbaseDefendPlanet(p *game.Player, planet *game.Planet, enemy *game.Player, enemyDist float64) {
	// Set defense target
	p.BotDefenseTarget = planet.ID

	// Starbases don't chase - they position and hold
	p.DesSpeed = 0

	// Use comprehensive shield management for starbase defense
	s.assessAndActivateShields(p, enemy)

	// Turn to face the threat
	angleToEnemy := math.Atan2(enemy.Y-p.Y, enemy.X-p.X)
	p.DesDir = angleToEnemy

	// Use starbase weapon logic (more aggressive than normal combat)
	s.starbaseDefenseWeaponLogic(p, enemy, enemyDist)

	// Check if threat is gone
	if enemy.Status != game.StatusAlive || enemyDist > game.StarbaseEnemyDetectRange+5000 {
		if threatenedPlanet, _, _ := s.getThreatenedFriendlyPlanet(p); threatenedPlanet == nil {
			p.BotDefenseTarget = -1
			p.BotCooldown = 15
			return
		}
	}

	p.BotCooldown = 5
}

// getThreatenedFriendlyPlanet scans for friendly planets under threat
// Returns the most threatened planet, the closest enemy to it, and the distance
func (s *Server) getThreatenedFriendlyPlanet(p *game.Player) (*game.Planet, *game.Player, float64) {
	var bestPlanet *game.Planet
	var bestEnemy *game.Player
	var bestEnemyDist float64 = 999999.0
	bestThreatScore := 0.0

	// Check each friendly planet within bot's scanning range
	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner != p.Team {
			continue
		}

		// Only check planets within bot's detection range
		botToPlanetDist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if botToPlanetDist > PlanetDefenseDetectRadius {
			continue
		}

		// Find the closest threatening enemy to this planet
		var closestEnemy *game.Player
		closestEnemyDist := 999999.0
		threatScore := 0.0

		for j := range s.gameState.Players {
			enemy := s.gameState.Players[j]
			if enemy.Status != game.StatusAlive || enemy.Team == p.Team || enemy.Cloaked {
				continue
			}

			enemyToPlanetDist := game.Distance(enemy.X, enemy.Y, planet.X, planet.Y)
			isThreatening := false
			currentThreatScore := 0.0

			// Check if enemy is within bombing range + intercept buffer
			if enemyToPlanetDist < (PlanetBombRange + PlanetDefenseInterceptBuffer) {
				isThreatening = true
				currentThreatScore = (PlanetBombRange + PlanetDefenseInterceptBuffer - enemyToPlanetDist) * 0.1
			} else {
				// Check if enemy is moving toward the planet (vector analysis)
				if enemy.Speed > 1.0 && enemyToPlanetDist < 12000 {
					// Calculate if enemy heading is toward planet
					angleToPlanet := math.Atan2(planet.Y-enemy.Y, planet.X-enemy.X)
					angleDiff := math.Abs(enemy.Dir - angleToPlanet)
					if angleDiff > math.Pi {
						angleDiff = 2*math.Pi - angleDiff
					}

					// If enemy is heading roughly toward planet (within 45 degrees)
					if angleDiff < math.Pi/4 {
						isThreatening = true
						currentThreatScore = (12000 - enemyToPlanetDist) * 0.05
					}
				}
			}

			if isThreatening {
				// Add extra threat weight for carriers (enemies with armies)
				if enemy.Armies > 0 {
					currentThreatScore += float64(enemy.Armies) * 2.0
				}

				// Add threat weight based on enemy damage (damaged enemies are easier to kill but might be desperate)
				enemyStats := game.ShipData[enemy.Ship]
				damageRatio := float64(enemy.Damage) / float64(enemyStats.MaxDamage)
				if damageRatio > 0.7 {
					currentThreatScore += 1.0 // Desperate enemies are more threatening
				}

				threatScore += currentThreatScore

				// Track closest threatening enemy to this planet
				if enemyToPlanetDist < closestEnemyDist {
					closestEnemyDist = enemyToPlanetDist
					closestEnemy = enemy
				}
			}
		}

		// Consider this planet if it has threats and higher priority than current best
		if threatScore > bestThreatScore && closestEnemy != nil {
			bestThreatScore = threatScore
			bestPlanet = planet
			bestEnemy = closestEnemy
			bestEnemyDist = closestEnemyDist
		}
	}

	if bestPlanet != nil && bestEnemy != nil {
		return bestPlanet, bestEnemy, bestEnemyDist
	}
	return nil, nil, 0.0
}
