package server

import (
	"fmt"
	"math"
	"math/rand"

	"github.com/lab1702/netrek-web/game"
)

// updateShipSystems handles all ship system updates for all players
func (s *Server) updateShipSystems() {
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		if p.Status != game.StatusAlive {
			continue
		}

		s.updatePlayerSystems(p, i)
	}
}

// updatePlayerSystems handles fuel, heat, repair, and other systems for a single player
func (s *Server) updatePlayerSystems(p *game.Player, playerIndex int) {
	// Check if ship has slowed down to 0 for repair request
	if p.RepairRequest && p.Speed == 0 && p.Orbiting < 0 {
		// Transition from repair request to actual repair
		p.RepairRequest = false
		p.Repairing = true
		// Send message about starting repairs
		s.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s is repairing damage", formatPlayerName(p)),
				"type": "info",
			},
		}
	}

	// Update fuel and engine temperature
	fuelUsage := 0
	if p.Orbiting < 0 { // Only use fuel when not orbiting
		fuelUsage = int(p.Speed) * 2
		if p.Cloaked {
			// Use ship-specific cloak cost
			shipStats := game.ShipData[p.Ship]
			fuelUsage += shipStats.CloakCost
		}
		// Charge for shields (from original Netrek daemon.c)
		if p.Shields_up {
			switch p.Ship {
			case game.ShipScout:
				fuelUsage += 2
			case game.ShipDestroyer, game.ShipCruiser, game.ShipBattleship, game.ShipAssault:
				fuelUsage += 3
			case game.ShipStarbase:
				fuelUsage += 6
			case game.ShipGalaxy:
				fuelUsage += 3 // Using same as cruiser
			}
		}

		// Engine temperature increases with speed (from original daemon.c)
		// p_etemp += j->p_speed
		p.ETemp += int(p.Speed)
	}
	p.Fuel = int(math.Max(0, float64(p.Fuel-fuelUsage)))

	// Cap ETemp at a reasonable maximum (150% of overheat threshold)
	const maxETempCap = 1500
	if p.ETemp > maxETempCap {
		p.ETemp = maxETempCap
	}

	// Recharge fuel
	shipStats := game.ShipData[p.Ship]
	if p.Orbiting < 0 {
		// Normal fuel recharge when not orbiting
		p.Fuel = int(math.Min(float64(shipStats.MaxFuel), float64(p.Fuel+10)))
	}

	// Cool weapons and engines using ship-specific rates
	if p.WTemp > 0 {
		p.WTemp -= shipStats.WpnCool
		if p.WTemp < 0 {
			p.WTemp = 0
		}
	}
	if p.ETemp > 0 {
		p.ETemp -= shipStats.EngCool
		// Ensure it never goes below 0
		if p.ETemp < 0 {
			p.ETemp = 0
		}
	}

	// Handle engine overheat (from original daemon.c)
	// Use ship-specific max engine temp
	maxETemp := shipStats.MaxEngTemp

	if p.EngineOverheat {
		// Count down overheat timer
		p.OverheatTimer--
		if p.OverheatTimer <= 0 {
			p.EngineOverheat = false
			// Engine cooling no longer generates a message
		}
	} else if p.ETemp > maxETemp {
		// Check for overheat - chance increases with temperature
		// At 1000: 1/40 chance, at 1500: 1/8 chance
		overheatChance := 40
		if p.ETemp > 1200 {
			overheatChance = 20
		}
		if p.ETemp > 1400 {
			overheatChance = 8
		}

		if rand.Intn(overheatChance) == 0 {
			p.EngineOverheat = true
			// Random duration between 100-250 frames (10-25 seconds at 10 FPS)
			p.OverheatTimer = rand.Intn(150) + 100
			p.DesSpeed = 0 // Stop the ship
			// Disable tractor/pressor beams
			p.Tractoring = -1
			p.Pressoring = -1
			// Engine overheating no longer generates a message
		}
	}

	// Handle repair mode
	// Fuel recharge (always happens, faster when at a fuel planet)
	// shipStats already declared above
	if p.Fuel < shipStats.MaxFuel {
		rechargeRate := shipStats.FuelRecharge
		// Check if orbiting a fuel planet
		if p.Orbiting >= 0 {
			planet := s.gameState.Planets[p.Orbiting]
			if planet.Owner == p.Team && (planet.Flags&game.PlanetFuel) != 0 {
				rechargeRate *= 2 // Double rate at fuel planets
			}
		}
		// Apply recharge every 10 ticks to match original scale
		if s.gameState.TickCount%10 == 0 {
			p.Fuel = game.Min(p.Fuel+rechargeRate, shipStats.MaxFuel)
		}
	}

	if p.Repairing {
		// Repair only works when stopped or orbiting
		if p.Speed == 0 || p.Orbiting >= 0 {
			// Track repair progress with accumulator for fractional repairs
			p.RepairCounter++

			// Use ship-specific repair rate, scale down for reasonable gameplay
			// RepairRate values are 80-140, we'll divide by 8 for intervals of 10-17 ticks
			// This means repairing every 1-1.7 seconds
			repairInterval := shipStats.RepairRate / 8
			if repairInterval < 5 {
				repairInterval = 5 // Minimum interval (0.5 seconds)
			}

			// Check if at repair planet (halve the interval for double speed)
			if p.Orbiting >= 0 {
				planet := s.gameState.Planets[p.Orbiting]
				if planet.Owner == p.Team && (planet.Flags&game.PlanetRepair) != 0 {
					repairInterval = repairInterval / 2
					if repairInterval < 3 {
						repairInterval = 3
					}
				}
			}

			// Apply repairs when counter reaches interval
			if p.RepairCounter >= repairInterval {
				p.RepairCounter = 0

				// Repair shields by 3 points (even with shields up)
				if p.Shields < shipStats.MaxShields {
					p.Shields = game.Min(p.Shields+3, shipStats.MaxShields)
				}

				// Repair hull damage by 2 points (only with shields down)
				if !p.Shields_up && p.Damage > 0 {
					p.Damage = game.Max(p.Damage-2, 0)
				}
			}

			// Add small fuel consumption for repairs
			p.Fuel = game.Max(p.Fuel-1, 0)
		} else {
			// Cancel repair mode and repair request if moving while not orbiting
			p.Repairing = false
			p.RepairRequest = false
			p.RepairCounter = 0
		}
	}

	// Cloak fuel consumption
	if p.Cloaked {
		// Cloak uses 10 fuel per frame
		p.Fuel = int(math.Max(0, float64(p.Fuel-10)))
		if p.Fuel == 0 {
			// Out of fuel, decloak
			p.Cloaked = false
		}
	}
}
