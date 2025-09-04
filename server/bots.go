package server

import (
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"log"
	"math"
	"math/rand"
)

const (
	// OrbitDistance is the distance required to orbit a planet
	OrbitDistance = 2000.0
)

// BotNames for generating random bot names
var BotNames = []string{
	"HAL-9000", "R2-D2", "C-3PO", "Data", "Bishop", "T-800",
	"Johnny-5", "WALL-E", "EVE", "Optimus", "Bender", "K-2SO",
	"BB-8", "IG-88", "HK-47", "GLaDOS", "SHODAN", "Cortana",
	"Friday", "Jarvis", "Vision", "Ultron", "Skynet", "Agent-Smith",
}

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

	// Debug logging for bot creation
	log.Printf("DEBUG: AddBot creating bot %s (ID: %d) with ship type %d (%s)",
		p.Name, p.ID, ship, game.ShipData[game.ShipType(ship)].Name)
	p.Connected = true
	p.IsBot = true
	p.BotTarget = -1
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
				// Navigate to repair/fuel planet
				p.Orbiting = -1
				dx := targetPlanet.X - p.X
				dy := targetPlanet.Y - p.Y
				baseDir := math.Atan2(dy, dx)

				// Apply separation from allies even when moving to objectives
				separationVector := s.calculateSeparationVector(p)
				if separationVector.magnitude > 0 {
					// Blend navigation with separation
					weight := math.Min(separationVector.magnitude/300.0, 0.5) // Increased weight
					navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
					navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
					p.DesDir = math.Atan2(navY, navX)
				} else {
					p.DesDir = baseDir
				}
				p.DesSpeed = s.getOptimalSpeed(p, dist)
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
					// Navigate to neutral planet
					p.Orbiting = -1
					p.Bombing = false
					p.Beaming = false
					p.BeamingUp = false
					dx := targetPlanet.X - p.X
					dy := targetPlanet.Y - p.Y
					baseDir := math.Atan2(dy, dx)

					// Apply separation to avoid bunching while moving to objectives
					separationVector := s.calculateSeparationVector(p)
					if separationVector.magnitude > 0 {
						weight := math.Min(separationVector.magnitude/300.0, 0.45) // Increased weight
						navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
						navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
						p.DesDir = math.Atan2(navY, navX)
					} else {
						p.DesDir = baseDir
					}
					p.DesSpeed = s.getOptimalSpeed(p, dist)

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
				// Navigate to planet
				p.Orbiting = -1
				p.Bombing = false
				p.Beaming = false
				p.BeamingUp = false
				dx := targetPlanet.X - p.X
				dy := targetPlanet.Y - p.Y
				baseDir := math.Atan2(dy, dx)

				// Apply separation to avoid bunching while moving to objectives
				separationVector := s.calculateSeparationVector(p)
				if separationVector.magnitude > 0 {
					weight := math.Min(separationVector.magnitude/500.0, 0.25)
					navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
					navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
					p.DesDir = math.Atan2(navY, navX)
				} else {
					p.DesDir = baseDir
				}
				p.DesSpeed = s.getOptimalSpeed(p, dist)

				// Engage enemies if they're threatening our objective
				if enemyDist < 8000 {
					// Enemy is close, might need to fight
					if enemyDist < 4000 || nearestEnemy.Armies > 0 {
						// Fight if enemy is very close or is a carrier
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
					// Move to defend with separation
					dx := planet.X - p.X
					dy := planet.Y - p.Y
					baseDir := math.Atan2(dy, dx)

					// Apply separation
					separationVector := s.calculateSeparationVector(p)
					if separationVector.magnitude > 0 {
						weight := math.Min(separationVector.magnitude/300.0, 0.4)
						navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
						navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
						p.DesDir = math.Atan2(navY, navX)
					} else {
						p.DesDir = baseDir
					}
					p.DesSpeed = float64(shipStats.MaxSpeed)
				} else {
					// Patrol around planet with separation
					patrolAngle := math.Mod(float64(rand.Intn(360))*math.Pi/180, math.Pi*2)

					// Apply separation even when patrolling
					separationVector := s.calculateSeparationVector(p)
					if separationVector.magnitude > 0 {
						weight := math.Min(separationVector.magnitude/250.0, 0.5)
						navX := math.Cos(patrolAngle)*(1.0-weight) + separationVector.x*weight
						navY := math.Sin(patrolAngle)*(1.0-weight) + separationVector.y*weight
						p.DesDir = math.Atan2(navY, navX)
					} else {
						p.DesDir = patrolAngle
					}
					p.DesSpeed = float64(shipStats.MaxSpeed) * 0.7
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
					// Approach at high speed with separation
					dx := planet.X - p.X
					dy := planet.Y - p.Y
					baseDir := math.Atan2(dy, dx)

					// Apply separation for raiders too
					separationVector := s.calculateSeparationVector(p)
					if separationVector.magnitude > 0 {
						weight := math.Min(separationVector.magnitude/350.0, 0.35) // Lighter weight for raiders
						navX := math.Cos(baseDir)*(1.0-weight) + separationVector.x*weight
						navY := math.Sin(baseDir)*(1.0-weight) + separationVector.y*weight
						p.DesDir = math.Atan2(navY, navX)
					} else {
						p.DesDir = baseDir
					}
					p.DesSpeed = float64(shipStats.MaxSpeed)
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

	// Basic needs assessment
	needRepair := p.Damage > shipStats.MaxDamage/3 // More conservative repair threshold
	needFuel := p.Fuel < shipStats.MaxFuel/2       // More conservative fuel threshold
	criticalDamage := p.Damage > shipStats.MaxDamage*2/3

	nearestEnemy := s.findNearestEnemy(p)
	enemyDist := 999999.0
	if nearestEnemy != nil {
		enemyDist = game.Distance(p.X, p.Y, nearestEnemy.X, nearestEnemy.Y)
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
	if criticalDamage || (needRepair && enemyDist < 12000) {
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
			} else if enemyDist > 8000 {
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
			isSafe := enemyDist > 10000 || (enemyDist > 6000 && isCorePlanet)

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
				// Defend from orbit if enemy approaches
				if nearestEnemy != nil && enemyDist < 8000 {
					s.starbaseDefensiveCombat(p, nearestEnemy, enemyDist)
				}
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
				if nearestEnemy != nil && enemyDist < 10000 {
					s.starbaseDefensiveCombat(p, nearestEnemy, enemyDist)
				}
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
				p.Shields_up = enemyDist < 12000
				if nearestEnemy != nil && enemyDist < 8000 {
					s.starbaseDefensiveCombat(p, nearestEnemy, enemyDist)
				}
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
		// Fire torpedoes conservatively
		if dist < 7000 && p.NumTorps < game.MaxTorps-1 && p.Fuel > 3000 && p.WTemp < 600 {
			s.fireBotTorpedoWithLead(p, enemy)
			p.BotCooldown = 12 // Slower firing rate
		}

		// Fire phasers for close threats or to finish enemies
		if dist < 4000 && p.Fuel > 2000 && p.WTemp < 700 {
			enemyDamageRatio := float64(enemy.Damage) / float64(game.ShipData[enemy.Ship].MaxDamage)
			if enemyDamageRatio > 0.6 || dist < 2500 {
				s.fireBotPhaser(p, enemy)
				p.BotCooldown = 15
			}
		}

		// Use plasma for area denial
		if shipStats.HasPlasma && p.NumPlasma < 1 && dist < 6000 && dist > 2000 && p.Fuel > 4000 {
			s.fireBotPlasma(p, enemy)
			p.BotCooldown = 25
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

// isCorePlanet checks if a planet is a core/home planet for a team
func (s *Server) isCorePlanet(planet *game.Planet, team int) bool {
	// Check if planet is close to team's home coordinates
	homeX := float64(game.TeamHomeX[team])
	homeY := float64(game.TeamHomeY[team])
	dist := game.Distance(planet.X, planet.Y, homeX, homeY)
	return dist < 25000 // Within 25k of home
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
	threats := s.assessCombatThreats(p)
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

	// Precision weapon usage
	angleDiff := math.Abs(p.Dir - interceptDir)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	// Enhanced torpedo firing with prediction and spread patterns
	if dist < 6000 && angleDiff < 0.4 {
		if p.NumTorps < game.MaxTorps-2 && p.Fuel > 2000 && p.WTemp < 80 {
			// Use spread pattern at medium range for area denial
			if dist > 3000 && dist < 5000 && p.NumTorps < game.MaxTorps-4 {
				s.fireTorpedoSpread(p, target, 3)
				p.BotCooldown = 8
			} else {
				s.fireEnhancedTorpedo(p, target)
				p.BotCooldown = 6
			}
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
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)
	if dist < myPhaserRange && angleDiff < 0.4 {
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
		if (dist < 7000 && dist > 2500 && target.Speed < 4) ||
			(targetDamageRatio > 0.7 && dist < 5000) ||
			(target.Orbiting >= 0 && dist < 6000) {
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

		// Shields up when carrying and threatened
		p.Shields_up = true
	}
}

// fireBotTorpedo fires a torpedo from a bot
func (s *Server) fireBotTorpedo(p *game.Player, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Calculate lead angle
	dist := game.Distance(p.X, p.Y, target.X, target.Y)
	timeToTarget := dist / float64(shipStats.TorpSpeed*20) // 20 units per tick

	// Predict where target will be
	predictX := target.X + target.Speed*math.Cos(target.Dir)*timeToTarget
	predictY := target.Y + target.Speed*math.Sin(target.Dir)*timeToTarget

	// Fire torpedo toward predicted position
	dx := predictX - p.X
	dy := predictY - p.Y
	fireDir := math.Atan2(dy, dx)

	// Create torpedo
	torp := &game.Torpedo{
		ID:     len(s.gameState.Torps),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.TorpSpeed * 20), // 20 units per tick
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: 1,      // Moving
		Team:   p.Team, // Set team color
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
}

// fireBotPhaser fires a phaser from a bot
func (s *Server) fireBotPhaser(p *game.Player, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]
	dist := game.Distance(p.X, p.Y, target.X, target.Y)

	// Calculate phaser range using original formula: PHASEDIST * phaserdamage / 100
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)

	if dist > myPhaserRange {
		return
	}

	// Calculate damage based on distance using original formula
	damage := float64(shipStats.PhaserDamage) * (1.0 - dist/myPhaserRange)

	// Apply damage to target
	target.Damage += int(damage)

	// Check if target destroyed
	targetStats := game.ShipData[target.Ship]
	if target.Damage >= targetStats.MaxDamage {
		target.Status = game.StatusExplode
		target.ExplodeTimer = 10
		target.KilledBy = p.ID
		target.WhyDead = game.KillPhaser
		target.Bombing = false // Stop bombing when destroyed
		target.Orbiting = -1   // Break orbit when destroyed
		// Clear lock-on when destroyed
		target.LockType = "none"
		target.LockTarget = -1
		target.Deaths++ // Increment death count
		p.Kills += 1
		p.KillsStreak += 1

		// Send death message
		s.broadcastDeathMessage(target, p)
	}

	// Create phaser visual
	s.broadcast <- ServerMessage{
		Type: "phaser",
		Data: map[string]interface{}{
			"from": p.ID,
			"to":   target.ID,
		},
	}

	p.Fuel -= shipStats.PhaserDamage * shipStats.PhaserFuelMult
}

// fireBotPlasma fires a plasma torpedo from a bot
func (s *Server) fireBotPlasma(p *game.Player, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	if !shipStats.HasPlasma {
		return
	}

	// Calculate lead angle
	dist := game.Distance(p.X, p.Y, target.X, target.Y)
	timeToTarget := dist / float64(shipStats.PlasmaSpeed*20) // 20 units per tick

	predictX := target.X + target.Speed*math.Cos(target.Dir)*timeToTarget
	predictY := target.Y + target.Speed*math.Sin(target.Dir)*timeToTarget

	dx := predictX - p.X
	dy := predictY - p.Y
	fireDir := math.Atan2(dy, dx)

	// Create plasma
	plasma := &game.Plasma{
		ID:     len(s.gameState.Plasmas),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.PlasmaSpeed * 20), // 20 units per tick
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse, // Use original fuse value directly
		Status: 1,                    // Moving
		Team:   p.Team,               // Set team color
	}

	s.gameState.Plasmas = append(s.gameState.Plasmas, plasma)
	p.NumPlasma++
	p.Fuel -= shipStats.PlasmaDamage * shipStats.PlasmaFuelMult
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

// shouldDodgeAdvanced checks if dodging is necessary with more sophisticated logic
func (s *Server) shouldDodgeAdvanced(p *game.Player, desiredDir float64) bool {
	// Check current damage at desired direction
	damage := s.calculateDamageAtDirection(p, desiredDir, p.DesSpeed)
	return damage > 0
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

// fireBotTorpedoWithLead fires torpedo with advanced leading
func (s *Server) fireBotTorpedoWithLead(p, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Calculate intercept similar to borgmove.c BorgTorpEnemy
	torpSpeed := float64(shipStats.TorpSpeed) * 20

	// Relative position
	vxa := target.X - p.X
	vya := target.Y - p.Y
	l := math.Hypot(vxa, vya)

	if l > 0 {
		vxa /= l
		vya /= l
	}

	// Target velocity
	vxs := target.Speed * math.Cos(target.Dir) * 20
	vys := target.Speed * math.Sin(target.Dir) * 20

	// Calculate intercept
	dp := vxs*vxa + vys*vya
	dx := vxs - dp*vxa
	dy := vys - dp*vya

	l = math.Hypot(dx, dy)
	l = torpSpeed*torpSpeed - l*l

	if l < 0 {
		// Can't intercept, fire directly
		s.fireBotTorpedo(p, target)
		return
	}

	l = math.Sqrt(l)
	vxt := l*vxa + dx
	vyt := l*vya + dy

	fireDir := math.Atan2(vyt, vxt)

	// Create torpedo with calculated direction
	torp := &game.Torpedo{
		ID:     len(s.gameState.Torps),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  torpSpeed,
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: 1,      // Moving
		Team:   p.Team, // Set team color
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
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

func (s *Server) findNearestArmyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

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

func (s *Server) findNearestEnemyArmyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

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

func (s *Server) findBestPlanetToTake(p *game.Player) *game.Planet {
	var best *game.Planet
	bestScore := -999999.0

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

		// Check for defenders and allies
		defenders := 0
		allies := 0
		for _, other := range s.gameState.Players {
			if other.Status == game.StatusAlive {
				otherDist := game.Distance(planet.X, planet.Y, other.X, other.Y)
				if otherDist < 10000 {
					if other.Team == planet.Owner {
						defenders++
						// Heavily penalize if defender is at the planet
						if otherDist < 2000 {
							defenders += 2
						}
					} else if other.Team == p.Team && other.ID != p.ID {
						allies++
					}
				}
			}
		}
		score -= float64(defenders) * 800
		score += float64(allies) * 300 // Bonus for allied support

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

// countTeamPlanets counts planets owned by each team
func (s *Server) countTeamPlanets() map[int]int {
	counts := make(map[int]int)
	for _, planet := range s.gameState.Planets {
		counts[planet.Owner]++
	}
	return counts
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

func (s *Server) findNearestFriendlyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner == p.Team {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

func (s *Server) findNearestEnemyPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

	for i := range s.gameState.Planets {
		planet := s.gameState.Planets[i]
		if planet.Owner != p.Team && planet.Owner != 0 {
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < minDist {
				minDist = dist
				nearest = planet
			}
		}
	}
	return nearest
}

// findNearestNeutralPlanet finds the closest neutral planet
func (s *Server) findNearestNeutralPlanet(p *game.Player) *game.Planet {
	var nearest *game.Planet
	minDist := 999999.0

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

// CombatThreat tracks various combat threats
type CombatThreat struct {
	closestTorpDist float64
	closestPlasma   float64
	nearbyEnemies   int
	requiresEvasion bool
	threatLevel     int
}

// SeparationVector represents the direction and magnitude to separate from allies
type SeparationVector struct {
	x         float64
	y         float64
	magnitude float64
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

// CombatManeuver represents a tactical movement decision
type CombatManeuver struct {
	direction float64
	speed     float64
	maneuver  string
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

	// Check planets
	for _, planet := range s.gameState.Planets {
		planetDist := game.Distance(testX, testY, planet.X, planet.Y) - 1000
		if planetDist < clearance {
			clearance = planetDist
		}
	}

	return clearance
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

// getEvasionSpeed returns optimal speed for evasion
func (s *Server) getEvasionSpeed(p *game.Player, threats CombatThreat) float64 {
	shipStats := game.ShipData[p.Ship]

	// High threat - maximum speed
	if threats.threatLevel > 5 {
		return float64(shipStats.MaxSpeed)
	}

	// Medium threat - variable speed for unpredictability
	if threats.threatLevel > 2 {
		return float64(shipStats.MaxSpeed) * (0.6 + rand.Float64()*0.4)
	}

	// Low threat - maintain combat speed
	return s.getOptimalCombatSpeed(p, 3000)
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

// fireTorpedoSpread fires multiple torpedoes in a spread pattern
func (s *Server) fireTorpedoSpread(p, target *game.Player, count int) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]
	baseDir := s.calculateEnhancedInterceptCourse(p, target)
	spreadAngle := math.Pi / 16 // Spread angle between torpedoes

	for i := 0; i < count; i++ {
		if p.NumTorps >= game.MaxTorps {
			break
		}

		// Calculate spread direction
		offset := float64(i-count/2) * spreadAngle
		fireDir := baseDir + offset

		// Create torpedo
		torp := &game.Torpedo{
			ID:     len(s.gameState.Torps),
			Owner:  p.ID,
			X:      p.X,
			Y:      p.Y,
			Dir:    fireDir,
			Speed:  float64(shipStats.TorpSpeed * 20),
			Damage: shipStats.TorpDamage,
			Fuse:   shipStats.TorpFuse,
			Status: 1,
			Team:   p.Team,
		}

		s.gameState.Torps = append(s.gameState.Torps, torp)
		p.NumTorps++
		p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
	}
}

// fireEnhancedTorpedo fires a torpedo with enhanced prediction
func (s *Server) fireEnhancedTorpedo(p, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	fireDir := s.calculateEnhancedInterceptCourse(p, target)
	shipStats := game.ShipData[p.Ship]

	torp := &game.Torpedo{
		ID:     len(s.gameState.Torps),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.TorpSpeed * 20),
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: 1,
		Team:   p.Team,
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
}

// managePredictiveShields manages shields with prediction
func (s *Server) managePredictiveShields(p, target *game.Player, enemyDist, torpDist float64) {
	shipStats := game.ShipData[p.Ship]

	// Calculate incoming damage potential
	incomingDamage := 0

	// Check torpedo threats
	if torpDist < 2500 {
		incomingDamage += 45 // Average torpedo damage
	}

	// Check phaser threat from target
	if target != nil {
		targetStats := game.ShipData[target.Ship]
		phaserRange := float64(game.PhaserDist * targetStats.PhaserDamage / 100)
		if enemyDist < phaserRange {
			// Check if target is facing us
			angleToUs := math.Atan2(p.Y-target.Y, p.X-target.X)
			angleDiff := math.Abs(target.Dir - angleToUs)
			if angleDiff > math.Pi {
				angleDiff = 2*math.Pi - angleDiff
			}
			if angleDiff < math.Pi/4 {
				incomingDamage += targetStats.PhaserDamage / 2
			}
		}
	}

	// Shield decision based on damage vs fuel
	shouldShield := false

	if incomingDamage > 30 && p.Fuel > 1500 {
		shouldShield = true
	} else if torpDist < 1500 && p.Fuel > 1000 {
		shouldShield = true
	} else if enemyDist < 2000 && p.Fuel > 2000 {
		shouldShield = true
	}

	// Don't shield if very low on fuel
	if p.Fuel < 800 {
		shouldShield = false
	}

	// Don't shield if we're trying to repair
	if p.Orbiting >= 0 && p.Damage > shipStats.MaxDamage/3 {
		shouldShield = false
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

// findPlanetToDefend finds a friendly planet that needs defense
func (s *Server) findPlanetToDefend(p *game.Player) *game.Planet {
	var best *game.Planet
	bestScore := -999999.0

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
	bestScore := -999999.0

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
		magnitudeScale := 1.0 + float64(nearbyAllies)*0.3 // More allies = stronger separation
		separationVec.x = totalRepelX * magnitudeScale
		separationVec.y = totalRepelY * magnitudeScale
		separationVec.magnitude = math.Sqrt(separationVec.x*separationVec.x + separationVec.y*separationVec.y)

		// Normalize but keep the magnitude for weighting
		if separationVec.magnitude > 0 {
			normalizedX := separationVec.x / separationVec.magnitude
			normalizedY := separationVec.y / separationVec.magnitude
			separationVec.x = normalizedX
			separationVec.y = normalizedY
			// Keep magnitude for weight calculations
		}
	}

	return separationVec
}

// executePatrol implements intelligent patrol patterns
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
