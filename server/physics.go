package server

import (
	"fmt"
	"math"

	"github.com/lab1702/netrek-web/game"
)

// updatePlayerPhysics handles all movement, positioning, and physics for a single player
func (s *Server) updatePlayerPhysics(p *game.Player, i int) {
	if p.Status != game.StatusAlive {
		return
	}

	// Update direction using original Netrek turning algorithm
	if p.Dir != p.DesDir {
		shipStats := game.ShipData[p.Ship]

		// Calculate turn increment based on speed (original NEWTURN=0 algorithm)
		var turnIncrement int
		speed := int(p.Speed)
		if speed < 30 {
			// Use bit shift: turnRate / (2^speed)
			turnIncrement = shipStats.TurnRate >> uint(speed)
		} else {
			// Very high speeds get minimal turning
			turnIncrement = 0
		}

		// Add to fractional accumulator
		p.SubDir += turnIncrement

		// Extract whole direction units and keep remainder
		ticks := p.SubDir / 1000
		p.SubDir = p.SubDir % 1000

		// Convert direction to 0-255 scale for calculation (like original)
		currentDir256 := int(p.Dir * 256.0 / (2 * math.Pi))
		desiredDir256 := int(p.DesDir * 256.0 / (2 * math.Pi))

		// Calculate shortest turn direction
		diff := desiredDir256 - currentDir256
		if diff > 128 {
			diff -= 256
		} else if diff < -128 {
			diff += 256
		}

		// Apply turn
		if math.Abs(float64(diff)) <= float64(ticks) {
			p.Dir = p.DesDir
		} else if diff > 0 {
			currentDir256 = (currentDir256 + ticks) % 256
			p.Dir = float64(currentDir256) * 2 * math.Pi / 256.0
		} else {
			currentDir256 = (currentDir256 - ticks + 256) % 256
			p.Dir = float64(currentDir256) * 2 * math.Pi / 256.0
		}
	}

	// Update speed (with damage-based max speed enforcement)
	if p.Speed != p.DesSpeed {
		// Calculate max speed based on damage
		shipStats := game.ShipData[p.Ship]
		maxSpeed := float64(shipStats.MaxSpeed)
		if p.Damage > 0 {
			// Formula from original Netrek: maxspeed = (max + 2) - (max + 1) * (damage / maxdamage)
			damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
			maxSpeed = float64(shipStats.MaxSpeed+2) - float64(shipStats.MaxSpeed+1)*damageRatio
			maxSpeed = math.Max(1, maxSpeed) // Minimum speed of 1
		}

		// Engine overheat limits speed to 1 (from original daemon.c)
		if p.EngineOverheat {
			maxSpeed = 1
			p.DesSpeed = math.Min(p.DesSpeed, 1)
		}

		// Enforce max speed limit
		actualDesSpeed := math.Min(p.DesSpeed, maxSpeed)

		// Ship-specific acceleration/deceleration using fractional accumulator
		// Based on original Netrek's acceleration system
		if actualDesSpeed > p.Speed {
			// Accelerating
			p.AccFrac += shipStats.AccInt
			// Each 1000 units of accumulator = 1 speed unit change (original Netrek uses 1000)
			if p.AccFrac >= 1000 {
				speedInc := p.AccFrac / 1000
				p.Speed = math.Min(p.Speed+float64(speedInc), actualDesSpeed)
				p.AccFrac = p.AccFrac % 1000
			}
		} else if actualDesSpeed < p.Speed {
			// Decelerating
			p.AccFrac += shipStats.DecInt
			// Each 1000 units of accumulator = 1 speed unit change (original Netrek uses 1000)
			if p.AccFrac >= 1000 {
				speedDec := p.AccFrac / 1000
				p.Speed = math.Max(p.Speed-float64(speedDec), actualDesSpeed)
				p.AccFrac = p.AccFrac % 1000
			}
		}
	}

	// Update position
	if p.Speed > 0 {
		// Convert speed to game units per tick
		// NOTE: Original Netrek uses WARP1=60, but we use 20 to maintain game balance
		// This difference is compensated by scaling factors elsewhere
		unitsPerTick := p.Speed * 20.0
		p.X += unitsPerTick * math.Cos(p.Dir)
		p.Y += unitsPerTick * math.Sin(p.Dir)

		// Bounce off galaxy edges
		if p.X < 0 {
			p.X = 0
			// Reverse X component of direction (bounce off left wall)
			p.Dir = math.Pi - p.Dir
			p.DesDir = p.Dir // Update desired direction to match bounced direction
		} else if p.X > game.GalaxyWidth {
			p.X = game.GalaxyWidth
			// Reverse X component of direction (bounce off right wall)
			p.Dir = math.Pi - p.Dir
			p.DesDir = p.Dir // Update desired direction to match bounced direction
		}
		if p.Y < 0 {
			p.Y = 0
			// Reverse Y component of direction (bounce off top wall)
			p.Dir = -p.Dir
			p.DesDir = p.Dir // Update desired direction to match bounced direction
		} else if p.Y > game.GalaxyHeight {
			p.Y = game.GalaxyHeight
			// Reverse Y component of direction (bounce off bottom wall)
			p.Dir = -p.Dir
			p.DesDir = p.Dir // Update desired direction to match bounced direction
		}
	}
}

// updatePlayerOrbit handles orbital mechanics for a single player
func (s *Server) updatePlayerOrbit(p *game.Player) {
	if p.Orbiting < 0 {
		return
	}

	// Orbit mechanics matching original Netrek
	if p.Orbiting >= len(s.gameState.Planets) {
		fmt.Printf("ERROR: Player %s orbiting invalid planet %d\n", p.Name, p.Orbiting)
		p.Orbiting = -1
		return
	}
	planet := s.gameState.Planets[p.Orbiting]

	// Original increments direction by 2 units at 10 updates/sec (major updates)
	// where 256 units = 2*PI radians, so 2 units = 2*PI/256 = PI/128
	// Since we run at 10 FPS, we match the major update rate
	p.Dir += math.Pi / 64 // Double the speed to match original timing
	if p.Dir > 2*math.Pi {
		p.Dir -= 2 * math.Pi
	}
	p.DesDir = p.Dir

	// Calculate position from direction
	// Ship direction points tangent to orbit, so actual angle from planet is dir - PI/2
	angleFromPlanet := p.Dir - math.Pi/2
	p.X = planet.X + float64(game.OrbitDist)*math.Cos(angleFromPlanet)
	p.Y = planet.Y + float64(game.OrbitDist)*math.Sin(angleFromPlanet)
	p.Speed = 0
	p.DesSpeed = 0
}

// updatePlayerLockOn handles lock-on course tracking for a single player
func (s *Server) updatePlayerLockOn(p *game.Player) {
	if p.LockType == "none" || p.LockTarget < 0 || p.Orbiting >= 0 {
		return // Don't track while orbiting
	}

	var targetX, targetY float64
	validTarget := false

	if p.LockType == "planet" {
		if p.LockTarget < game.MaxPlanets {
			planet := s.gameState.Planets[p.LockTarget]
			targetX = planet.X
			targetY = planet.Y
			validTarget = true

			// Auto-orbit when close to locked planet
			dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
			if dist < 3000 && p.Speed < 4 {
				// Close enough and slow enough to orbit
				p.Orbiting = p.LockTarget
				p.Speed = 0
				p.DesSpeed = 0

				// Clear lock when entering orbit
				p.LockType = "none"
				p.LockTarget = -1

				// Update planet info - team now has scouted this planet
				planet.Info |= p.Team

				// Send orbit confirmation
				s.broadcast <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": fmt.Sprintf("%s is orbiting %s", formatPlayerName(p), planet.Name),
						"type": "info",
					},
				}
			} else if dist > 5000 {
				// Far from planet - go fast (unless we need to turn)
				p.DesSpeed = float64(game.ShipData[p.Ship].MaxSpeed)
			} else {
				// Approaching planet - slow down based on distance
				maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)
				// Slow down from max speed to 3 as we approach from 5000 to 3000 units
				speedRatio := (dist - 3000) / 2000 // 0 to 1 as we get closer
				p.DesSpeed = 3 + (maxSpeed-3)*speedRatio
				p.DesSpeed = math.Max(3, math.Min(maxSpeed, p.DesSpeed))
			}
		}
	}

	if validTarget && p.Orbiting < 0 {
		// Update desired course toward target (ship will turn at normal rate)
		dx := targetX - p.X
		dy := targetY - p.Y
		targetDir := math.Atan2(dy, dx)

		// Calculate angle difference to target
		angleDiff := targetDir - p.Dir
		// Normalize to -PI to PI
		for angleDiff > math.Pi {
			angleDiff -= 2 * math.Pi
		}
		for angleDiff < -math.Pi {
			angleDiff += 2 * math.Pi
		}

		// If we're moving fast and need to turn significantly, slow down temporarily
		// This helps with turning since turn rate decreases exponentially with speed
		if p.LockType == "planet" && p.Speed > 6 && math.Abs(angleDiff) > math.Pi/4 {
			// Slow down to improve turning - the more we need to turn, the slower we go
			// Scale speed from 6 down to 3 based on angle (45-180 degrees)
			angleRatio := (math.Abs(angleDiff) - math.Pi/4) / (3 * math.Pi / 4) // 0 to 1
			angleRatio = math.Min(1.0, angleRatio)
			targetSpeed := 6.0 - 3.0*angleRatio // 6 to 3

			// Only reduce speed, don't increase it
			if targetSpeed < p.DesSpeed {
				p.DesSpeed = targetSpeed
			}
		}

		p.DesDir = targetDir
	} else if !validTarget {
		// Clear invalid lock
		p.LockType = "none"
		p.LockTarget = -1
	}
}

// updateTractorBeams handles tractor/pressor beam physics for all players
func (s *Server) updateTractorBeams() {
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		if p.Status != game.StatusAlive {
			continue
		}

		// Apply tractor/pressor beam physics (disabled when engines overheated, orbiting, or docked - like original)
		if (p.Tractoring >= 0 || p.Pressoring >= 0) && !p.EngineOverheat && p.Orbiting < 0 {
			var targetID int
			isPressor := false
			if p.Tractoring >= 0 {
				targetID = p.Tractoring
			} else {
				targetID = p.Pressoring
				isPressor = true
			}

			if targetID < game.MaxPlayers {
				target := s.gameState.Players[targetID]
				if target.Status == game.StatusAlive {
					dist := game.Distance(p.X, p.Y, target.X, target.Y)

					// Get ship stats for range check
					shipStats := game.ShipData[p.Ship]

					// Break beam if out of range (using ship-specific range)
					tractorRange := float64(game.TractorDist) * shipStats.TractorRange
					if dist > tractorRange {
						p.Tractoring = -1
						p.Pressoring = -1
					} else {
						// Original Netrek physics implementation from daemon.c
						targetStats := game.ShipData[target.Ship]

						// Calculate normalized direction vector (cosTheta, sinTheta in original)
						dx := target.X - p.X
						dy := target.Y - p.Y
						if dist == 0 {
							dist = 1 // prevent divide by zero
						}
						cosTheta := dx / dist
						sinTheta := dy / dist

						// Force of tractor is WARP1 * tractstr (from original code)
						// WARP1 = 20 in original Netrek
						halfforce := 20.0 * float64(shipStats.TractorStr)

						// Direction: 1 for tractor, -1 for pressor
						dir := 1.0
						if isPressor {
							dir = -1.0
						}

						// Original formula: change in position is tractor strength over mass
						// Source ship moves
						p.X += dir * cosTheta * halfforce / float64(shipStats.Mass)
						p.Y += dir * sinTheta * halfforce / float64(shipStats.Mass)

						// Target ship moves in opposite direction
						target.X -= dir * cosTheta * halfforce / float64(targetStats.Mass)
						target.Y -= dir * sinTheta * halfforce / float64(targetStats.Mass)

						// Break orbit immediately if target is orbiting (from original code)
						if target.Orbiting >= 0 {
							target.Orbiting = -1
							target.Bombing = false // Stop bombing if forced out of orbit
							target.Beaming = false // Stop beaming too
							// Send message about breaking orbit
							s.broadcast <- ServerMessage{
								Type: MsgTypeMessage,
								Data: map[string]interface{}{
									"text": fmt.Sprintf("%s was pulled out of orbit", formatPlayerName(target)),
									"type": "info",
								},
							}
						}

						// Use fuel for beam and add engine heat (from original: TRACTCOST=20, TRACTEHEAT=5)
						p.Fuel = int(math.Max(0, float64(p.Fuel-20)))
						p.ETemp += 5 // TRACTEHEAT from original
						// Cap at maximum
						if p.ETemp > 1500 {
							p.ETemp = 1500
						}
						if p.Fuel == 0 {
							p.Tractoring = -1
							p.Pressoring = -1
						}
					}
				} else {
					// Target died, release beam
					p.Tractoring = -1
					p.Pressoring = -1
				}
			}
		}
	}
}

// updateAlertLevels calculates alert levels for all players based on nearby enemies
func (s *Server) updateAlertLevels() {
	// Calculate alert level based on nearby enemy ships (from original daemon.c)
	// YRANGE = GWIDTH/7 = 100000/7 = ~14285
	// RRANGE = GWIDTH/10 = 100000/10 = 10000
	const YRANGE = 14285
	const RRANGE = 10000

	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		if p.Status != game.StatusAlive {
			continue
		}

		p.AlertLevel = "green" // Default to green

		// Check all other players for alert status
		for j := 0; j < game.MaxPlayers; j++ {
			if j == i {
				continue // Skip self
			}

			enemy := s.gameState.Players[j]

			// Skip if not alive or not at war
			if enemy.Status != game.StatusAlive {
				continue
			}

			// Check if players are at war (simplified - different teams are at war)
			if enemy.Team == p.Team {
				continue
			}

			// Calculate distance
			dx := math.Abs(p.X - enemy.X)
			dy := math.Abs(p.Y - enemy.Y)

			// Quick range check
			if dx > YRANGE || dy > YRANGE {
				continue
			}

			dist := dx*dx + dy*dy

			if dist < RRANGE*RRANGE {
				// Red alert - enemy very close
				p.AlertLevel = "red"
				break // Can't get worse than red
			} else if dist < YRANGE*YRANGE && p.AlertLevel != "red" {
				// Yellow alert - enemy moderately close
				p.AlertLevel = "yellow"
				// Don't break, keep checking for closer enemies
			}
		}
	}
}
