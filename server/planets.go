package server

import (
	"fmt"
	"log"
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// updatePlanetInteractions handles all planet-related interactions for all players
func (s *Server) updatePlanetInteractions() {
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		if p.Status != game.StatusAlive {
			continue
		}

		// Check orbit status
		if p.Orbiting >= 0 {
			s.updateOrbitingPlayer(p, i)
		}

		// Handle planet damage for non-orbiting ships near hostile planets
		// This also happens every 5 frames matching plfight()
		if p.Orbiting < 0 && s.gameState.Frame%5 == 0 {
			s.updatePlanetCombat(p, i)
		}
	}

	// Handle planet army repopulation
	s.updatePlanetArmies()
}

// updateOrbitingPlayer handles all interactions for a player currently orbiting a planet
func (s *Server) updateOrbitingPlayer(p *game.Player, playerIndex int) {
	// Orbit mechanics matching original Netrek
	if p.Orbiting >= len(s.gameState.Planets) {
		log.Printf("ERROR: Player %s orbiting invalid planet %d", p.Name, p.Orbiting)
		p.Orbiting = -1
		return
	}
	planet := s.gameState.Planets[p.Orbiting]

	// Repair and refuel at friendly planets
	if planet.Owner == p.Team && (planet.Flags&game.PlanetRepair) != 0 {
		// Repair damage
		if p.Damage > 0 {
			p.Damage = int(math.Max(0, float64(p.Damage-2)))
		}
		// Recharge shields
		shipStats := game.ShipData[p.Ship]
		if p.Shields < shipStats.MaxShields {
			p.Shields = int(math.Min(float64(shipStats.MaxShields), float64(p.Shields+2)))
		}
	}
	if planet.Owner == p.Team && (planet.Flags&game.PlanetFuel) != 0 {
		// Refuel faster at fuel planets
		shipStats := game.ShipData[p.Ship]
		p.Fuel = int(math.Min(float64(shipStats.MaxFuel), float64(p.Fuel+50)))
	}

	// Handle planet damage to orbiting hostile ships
	// This happens every 5 frames (2 times per second at 10 FPS) matching plfight()
	if s.gameState.Frame%5 == 0 {
		if planet.Owner != p.Team && planet.Owner != game.TeamNone && planet.Armies > 0 {
			// Calculate damage: armies/10 + 2
			damage := planet.Armies/10 + 2

			// Apply damage to shields first, then hull
			if p.Shields_up && p.Shields > 0 {
				p.Shields -= damage
				if p.Shields < 0 {
					p.Damage -= p.Shields // Overflow damage goes to hull
					p.Shields = 0
				}
			} else {
				p.Damage += damage
			}

			// Check if ship destroyed by planet
			if p.Damage >= game.ShipData[p.Ship].MaxDamage {
				p.Status = game.StatusExplode
				p.ExplodeTimer = 10
				p.KilledBy = -1 // No player killer
				p.WhyDead = game.KillPlanet
				p.Bombing = false
				p.Beaming = false
				p.Orbiting = -1
				// Clear lock-on when destroyed
				p.LockType = "none"
				p.LockTarget = -1
				p.Deaths++ // Increment death count

				// Send death message
				s.broadcast <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": fmt.Sprintf("%s killed by %s [planet]", formatPlayerName(p), planet.Name),
						"type": "kill",
					},
				}
			}
		}
	}

	// Handle continuous bombing
	if p.Bombing && planet.Owner != p.Team {
		if planet.Armies > 0 {
			// Original Netrek bombing mechanics:
			// plfight() is called every 0.5 seconds (2 times per second)
			// 50% chance to bomb, then:
			// 60% chance: 1 army, 20% chance: 2 armies, 20% chance: 3 armies
			// This averages 1.6 armies per second

			// Only check bombing every 5 frames (2 times per second at 10 FPS)
			if s.gameState.Frame%5 == 0 {
				// Random check (50% chance to bomb)
				if rand.Float32() < 0.5 {
					// Determine number of armies to bomb
					rnd := rand.Float32()
					var killed int
					if rnd < 0.6 {
						killed = 1
					} else if rnd < 0.8 {
						killed = 2
					} else {
						killed = 3
					}

					// Assault ships get +1 bonus (if we add assault ships later)
					// if p.Ship == game.ShipAssault {
					//     killed++
					// }

					planet.Armies = game.Max(0, planet.Armies-killed)

					// If planet has no armies left, it becomes neutral and stop bombing
					if planet.Armies == 0 {
						oldOwner := planet.Owner
						planet.Owner = game.TeamNone
						p.Bombing = false
						// Send completion message
						s.broadcast <- ServerMessage{
							Type: MsgTypeMessage,
							Data: map[string]interface{}{
								"text": fmt.Sprintf("%s destroyed all armies on %s (now independent)", formatPlayerName(p), planet.Name),
								"type": "info",
							},
						}
						// Debug log
						log.Printf("Planet %s bombed to 0 armies, owner changed from %d to %d (TeamNone=%d)",
							planet.Name, oldOwner, planet.Owner, game.TeamNone)
					}
				}
			}
		} else {
			// No armies left, stop bombing
			p.Bombing = false
		}
	}

	// Handle continuous beaming
	if p.Beaming {
		// Beam armies every 0.5 seconds (5 frames at 10 FPS)
		if s.gameState.Frame%5 == 0 {
			shipStats := game.ShipData[p.Ship]

			if p.BeamingUp {
				// Beam up mode - requires 2 kills since last death in classic Netrek
				if planet.Owner == p.Team && planet.Armies > 1 && p.Armies < shipStats.MaxArmies && p.KillsStreak >= game.ArmyKillRequirement {
					// Beam up 1 army at a time (leave at least 1 for defense)
					p.Armies++
					planet.Armies--
				} else {
					// Can't beam up anymore (no armies, full, or not enough kill streak), stop
					p.Beaming = false
					p.BeamingUp = false
				}
			} else {
				// Beam down mode
				if p.Armies > 0 && (planet.Owner == p.Team || planet.Owner == game.TeamNone) {
					// Beam down 1 army at a time
					p.Armies--
					planet.Armies++

					// If beaming down to an independent planet, conquer it
					if planet.Owner == game.TeamNone {
						oldOwner := planet.Owner
						planet.Owner = p.Team

						log.Printf("Planet %s conquered by continuous beaming, owner changed from %d to %d",
							planet.Name, oldOwner, planet.Owner)
					}
				} else {
					// Can't beam down anymore, stop
					p.Beaming = false
					p.BeamingUp = false
				}
			}
		}
	}
}

// updatePlanetCombat handles planet-to-ship combat for non-orbiting ships
func (s *Server) updatePlanetCombat(p *game.Player, playerIndex int) {
	for _, planet := range s.gameState.Planets {
		if planet == nil {
			continue
		}

		// Skip friendly or neutral planets with no armies
		if planet.Owner == p.Team || planet.Owner == game.TeamNone || planet.Armies == 0 {
			continue
		}

		// Check if within firing range
		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist <= game.PlanetFireDist {
			// Calculate damage: armies/10 + 2
			damage := planet.Armies/10 + 2

			// Apply damage to shields first, then hull
			if p.Shields_up && p.Shields > 0 {
				p.Shields -= damage
				if p.Shields < 0 {
					p.Damage -= p.Shields // Overflow damage goes to hull
					p.Shields = 0
				}
			} else {
				p.Damage += damage
			}

			// Check if ship destroyed by planet
			if p.Damage >= game.ShipData[p.Ship].MaxDamage {
				p.Status = game.StatusExplode
				p.ExplodeTimer = 10
				p.KilledBy = -1 // No player killer
				p.WhyDead = game.KillPlanet
				p.Deaths++ // Increment death count
				// Clear lock-on when destroyed
				p.LockType = "none"
				p.LockTarget = -1

				// Send death message
				s.broadcast <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": fmt.Sprintf("%s killed by %s [planet]", formatPlayerName(p), planet.Name),
						"type": "kill",
					},
				}
				break // Ship is dead, no need to check other planets
			}
		}
	}
}

// updatePlanetArmies handles planet army repopulation
func (s *Server) updatePlanetArmies() {
	// Handle planet army repopulation
	// AGRI planets generate 1 army every 5 seconds (100 frames at 20 FPS)
	// Non-AGRI planets generate 1 army every 30 seconds (600 frames at 20 FPS)
	// Only planets with owner (not neutral) can grow armies
	const maxPlanetArmies = 40

	// Check AGRI planets every 5 seconds
	if s.gameState.Frame%100 == 0 {
		for _, planet := range s.gameState.Planets {
			if planet == nil {
				continue
			}

			// Check if planet is owned and has AGRI flag
			if planet.Owner != game.TeamNone && (planet.Flags&game.PlanetAgri) != 0 {
				if planet.Armies < maxPlanetArmies {
					planet.Armies++
				}
			}
		}
	}

	// Check non-AGRI planets every 30 seconds
	if s.gameState.Frame%600 == 0 {
		for _, planet := range s.gameState.Planets {
			if planet == nil {
				continue
			}

			// Check if planet is owned and does NOT have AGRI flag
			if planet.Owner != game.TeamNone && (planet.Flags&game.PlanetAgri) == 0 {
				if planet.Armies < maxPlanetArmies {
					planet.Armies++
				}
			}
		}
	}
}
