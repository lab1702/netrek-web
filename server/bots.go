package server

import (
	"fmt"
	"math"
	"math/rand"
	"github.com/lab1702/netrek-web/game"
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
				p.BotCooldown = 20
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
				p.BotCooldown = 30
				return
			} else {
				// Navigate to repair/fuel planet
				p.Orbiting = -1
				dx := targetPlanet.X - p.X
				dy := targetPlanet.Y - p.Y
				p.DesDir = math.Atan2(dy, dx)
				p.DesSpeed = s.getOptimalSpeed(p, dist)
				return
			}
		}
	}

	// TOURNAMENT MODE: Prioritize planet conquest
	if s.gameState.T_mode {
		// In tournament mode, focus on strategic objectives

		// If carrying armies, prioritize delivering them
		if p.Armies > 0 {
			var targetPlanet *game.Planet
			// Find best planet to take (enemy or neutral)
			targetPlanet = s.findBestPlanetToTake(p)

			if targetPlanet != nil && targetPlanet.Owner != p.Team {
				dist := game.Distance(p.X, p.Y, targetPlanet.X, targetPlanet.Y)
				if dist < OrbitDistance {
					// At planet - beam down armies
					if p.Orbiting != targetPlanet.ID {
						p.Orbiting = targetPlanet.ID
						p.DesSpeed = 0
					}
					p.Beaming = true
					p.BeamingUp = false
					p.BotCooldown = 50
					return
				} else {
					// Navigate to target planet
					p.Orbiting = -1
					p.Bombing = false
					p.Beaming = false
					p.BeamingUp = false
					dx := targetPlanet.X - p.X
					dy := targetPlanet.Y - p.Y
					p.DesDir = math.Atan2(dy, dx)
					p.DesSpeed = s.getOptimalSpeed(p, dist)

					// Defend against nearby enemies while carrying
					if enemyDist < 5000 {
						s.defendWhileCarrying(p, nearestEnemy)
					}
					return
				}
			}
		}

		// Not carrying armies - get armies or bomb enemy planets
		var targetPlanet *game.Planet

		// First priority: Pick up armies if we have kills
		if p.Kills >= game.ArmyKillRequirement && armyPlanet != nil {
			targetPlanet = armyPlanet
		} else if enemyArmyPlanet != nil {
			// Second priority: Bomb enemy planets with armies
			targetPlanet = enemyArmyPlanet
		} else if takePlanet != nil {
			// Third priority: Take neutral/enemy planets
			targetPlanet = takePlanet
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
					if targetPlanet.Armies > 1 && p.Armies < s.getBotArmyCapacity(p) && p.Kills >= game.ArmyKillRequirement {
						p.Bombing = false // Stop bombing if planet is now friendly
						p.Beaming = true
						p.BeamingUp = true
						p.BotCooldown = 50
					} else {
						// Can't beam up, find something else to do
						p.Bombing = false
						p.Beaming = false
						p.BeamingUp = false
						p.Orbiting = -1
						p.BotCooldown = 10
					}
				} else {
					// Enemy or neutral planet
					if targetPlanet.Owner != game.TeamNone && targetPlanet.Armies > 0 {
						// Enemy planet with armies - bomb it
						p.Bombing = true
						p.Beaming = false
						p.BeamingUp = false
						p.BotCooldown = 100
					} else if targetPlanet.Armies == 0 || targetPlanet.Owner == game.TeamNone {
						// No armies or neutral planet
						p.Bombing = false // Stop bombing if no armies left
						if p.Armies > 0 {
							// Beam down to take it
							p.Beaming = true
							p.BeamingUp = false
							p.BotCooldown = 50
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
				p.DesDir = math.Atan2(dy, dx)
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
		// Focus on finding and fighting enemies

		// Advanced target selection for combat practice
		bestTarget := -1
		bestScore := -999999.0

		for i, other := range s.gameState.Players {
			if other.Status != game.StatusAlive || other.Team == p.Team || i == p.ID {
				continue
			}

			dist := game.Distance(p.X, p.Y, other.X, other.Y)

			// Score based on multiple factors for good combat practice
			score := 15000.0 / dist // Heavily weight distance for aggressive hunting

			// Prefer damaged enemies for easier kills
			otherStats := game.ShipData[other.Ship]
			damageRatio := float64(other.Damage) / float64(otherStats.MaxDamage)
			score += damageRatio * 3000

			// Prioritize carriers to prevent planet taking
			if other.Armies > 0 {
				score += 8000
			}

			// Bonus for enemies with high kill counts (better practice)
			score += float64(other.Kills) * 500

			// Avoid cloaked ships unless very close
			if other.Cloaked && dist > 3000 {
				score -= 5000
			}

			if score > bestScore {
				bestScore = score
				bestTarget = i
			}
		}

		if bestTarget != -1 {
			// Found a target - hunt and engage
			target := s.gameState.Players[bestTarget]
			dist := game.Distance(p.X, p.Y, target.X, target.Y)

			// Always pursue enemies aggressively in practice mode
			s.engageCombat(p, target, dist)
			return
		}

		// No enemies found - patrol aggressively
		if p.BotGoalX == 0 && p.BotGoalY == 0 {
			// Set a new patrol point in enemy territory
			enemyTeam := (p.Team % 4) + 1
			if enemyTeam > 4 {
				enemyTeam = 1
			}
			p.BotGoalX = float64(game.TeamHomeX[enemyTeam]) + float64(rand.Intn(20000)-10000)
			p.BotGoalY = float64(game.TeamHomeY[enemyTeam]) + float64(rand.Intn(20000)-10000)
		}

		// Navigate to patrol point
		dx := p.BotGoalX - p.X
		dy := p.BotGoalY - p.Y
		dist := math.Hypot(dx, dy)

		if dist < 5000 {
			// Reached patrol point, set new one
			enemyTeam := (p.Team % 4) + 1
			if enemyTeam > 4 {
				enemyTeam = 1
			}
			p.BotGoalX = float64(game.TeamHomeX[enemyTeam]) + float64(rand.Intn(20000)-10000)
			p.BotGoalY = float64(game.TeamHomeY[enemyTeam]) + float64(rand.Intn(20000)-10000)
		} else {
			p.DesDir = math.Atan2(dy, dx)
			p.DesSpeed = float64(shipStats.MaxSpeed)
		}
		return
	}
}

// engageCombat handles combat engagement for hard bots
func (s *Server) engageCombat(p *game.Player, target *game.Player, dist float64) {
	shipStats := game.ShipData[p.Ship]

	// Break orbit when entering combat
	if p.Orbiting >= 0 {
		p.Orbiting = -1
		p.Bombing = false
		p.Beaming = false
		p.BeamingUp = false
	}

	// Calculate intercept course
	interceptDir := s.calculateInterceptCourse(p, target)

	// Check for torpedo threats
	var closestTorpDist float64 = 999999.0
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID {
			torpDist := game.Distance(p.X, p.Y, torp.X, torp.Y)
			if torpDist < closestTorpDist {
				closestTorpDist = torpDist
			}
		}
	}

	// Advanced dodge calculation
	if s.shouldDodgeAdvanced(p, interceptDir) {
		dodgeDir := s.getBestDodgeDirection(p, interceptDir)
		p.DesDir = dodgeDir
		p.DesSpeed = float64(shipStats.MaxSpeed)
		p.BotCooldown = 5
		return
	}

	p.DesDir = interceptDir
	p.DesSpeed = s.getOptimalCombatSpeed(p, dist)

	// Adjust for damage
	if p.Damage > 0 {
		damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
		maxSpeed := (float64(shipStats.MaxSpeed) + 2) - (float64(shipStats.MaxSpeed)+1)*damageRatio
		if p.DesSpeed > maxSpeed {
			p.DesSpeed = maxSpeed
		}
	}

	// Precision weapon usage
	angleDiff := math.Abs(p.Dir - interceptDir)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	// Torpedo firing with leading
	if dist < 5000 && angleDiff < 0.3 {
		if p.NumTorps < game.MaxTorps-2 && p.Fuel > 2000 && p.WTemp < 80 {
			s.fireBotTorpedoWithLead(p, target)
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

	// Phaser at optimal range (using correct formula: PHASEDIST * phaserdamage / 100)
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)
	if dist < myPhaserRange && angleDiff < 0.4 {
		phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
		if p.Fuel >= phaserCost*2 && p.WTemp < 80 {
			targetStats := game.ShipData[target.Ship]
			targetDamageRatio := float64(target.Damage) / float64(targetStats.MaxDamage)
			if targetDamageRatio > 0.5 || dist < 1500 {
				s.fireBotPhaser(p, target)
				p.BotCooldown = 10
			}
		}
	}

	// Smart shield management
	s.manageBotShields(p, dist, closestTorpDist)

	// Use plasma strategically
	if shipStats.HasPlasma && p.NumPlasma < 1 && dist < 6000 && dist > 2000 && p.Fuel > 4000 {
		s.fireBotPlasma(p, target)
		p.BotCooldown = 20
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
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
}

// fireBotPhaser fires a phaser from a bot
func (s *Server) fireBotPhaser(p *game.Player, target *game.Player) {
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
		target.Deaths++        // Increment death count
		p.Kills += 1

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

		// Score based on value and distance
		score := 10000.0 / dist

		// Prefer planets with less armies
		if planet.Armies < 8 {
			score += 2000
		}

		// Prefer agricultural planets
		if (planet.Flags & game.PlanetAgri) != 0 {
			score += 1000
		}

		// Check for defenders
		defenders := 0
		for _, other := range s.gameState.Players {
			if other.Status == game.StatusAlive && other.Team == planet.Owner {
				defDist := game.Distance(planet.X, planet.Y, other.X, other.Y)
				if defDist < 8000 {
					defenders++
				}
			}
		}
		score -= float64(defenders) * 500

		if score > bestScore {
			bestScore = score
			best = planet
		}
	}

	return best
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

	// Balance teams by adding bots
	teams := []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri}
	for _, team := range teams {
		deficit := maxCount - teamCounts[team]
		for deficit > 0 {
			// Add a bot to this team
			ship := rand.Intn(5) // Random ship type (0-4: Scout, Destroyer, Cruiser, Battleship, Assault)
			s.AddBot(team, ship)
			deficit--
		}
	}
}
