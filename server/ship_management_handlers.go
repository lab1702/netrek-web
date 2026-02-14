package server

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/lab1702/netrek-web/game"
)

// handleRepair toggles repair mode
func (c *Client) handleRepair(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	if !p.Repairing && !p.RepairRequest {
		// If moving while not orbiting, set repair request and slow down
		if p.Speed > 0 && p.Orbiting < 0 {
			p.RepairRequest = true
			p.DesSpeed = 0 // Start slowing down
			// Send message about slowing to repair (non-blocking)
			select {
			case c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s is slowing to repair", formatPlayerName(p)),
					"type": "info",
				},
			}:
			default:
			}
			return
		}

		// If stopped or orbiting, enter repair mode immediately
		p.Repairing = true
		p.DesSpeed = 0       // Stop the ship
		p.Shields_up = false // Lower shields
		// Cancel any locks, beaming, bombing
		p.Bombing = false
		p.Beaming = false
		p.Tractoring = -1
		p.Pressoring = -1
	} else if p.RepairRequest {
		// Cancel repair request
		p.RepairRequest = false
		// Send message about canceling repair (non-blocking)
		select {
		case c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s canceled repair request", formatPlayerName(p)),
				"type": "info",
			},
		}:
		default:
		}
	} else {
		// Exit repair mode
		p.Repairing = false
	}
}

// handleBeam handles army beaming
func (c *Client) handleBeam(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	var beamData BeamData
	if err := json.Unmarshal(data, &beamData); err != nil {
		log.Printf("Error unmarshaling beam data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil || p.Orbiting < 0 || p.Orbiting >= game.MaxPlanets {
		return
	}

	planet := c.server.gameState.Planets[p.Orbiting]
	shipStats := game.ShipData[p.Ship]

	if beamData.Up {
		// Toggle beam up mode or turn it off if already beaming up
		if p.Beaming && p.BeamingUp {
			// Already beaming up, turn it off
			p.Beaming = false
			p.BeamingUp = false
		} else {
			// Start beaming up (only if planet has armies and is friendly)
			// Must leave at least 1 army on the planet
			// Classic Netrek requires 2 kills since last death to pick up armies
			if planet.Owner == p.Team && planet.Armies > 1 && p.Armies < shipStats.MaxArmies {
				if p.KillsStreak >= game.ArmyKillRequirement {
					p.Beaming = true
					p.BeamingUp = true
				} else {
					// Send message about needing kills
					errorMsg := ServerMessage{
						Type: MsgTypeMessage,
						Data: map[string]interface{}{
							"text": fmt.Sprintf("You need %.0f more kills since last death to pick up armies", game.ArmyKillRequirement-p.KillsStreak),
							"type": "error",
						},
					}
					select {
					case c.send <- errorMsg:
					default:
						// Client's send channel is full, skip
					}
				}
			}
		}
	} else {
		// Toggle beam down mode or turn it off if already beaming down
		if p.Beaming && !p.BeamingUp {
			// Already beaming down, turn it off
			p.Beaming = false
			p.BeamingUp = false
		} else {
			// Start beaming down (only if we have armies and planet is friendly or independent)
			if p.Armies > 0 && (planet.Owner == p.Team || planet.Owner == game.TeamNone) {
				p.Beaming = true
				p.BeamingUp = false
			}
		}
	}
}

// handleBomb handles planet bombing
func (c *Client) handleBomb(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil || p.Orbiting < 0 || p.Orbiting >= game.MaxPlanets {
		return
	}

	planet := c.server.gameState.Planets[p.Orbiting]

	// Can only bomb enemy or independent planets
	if planet.Owner != p.Team {
		// Don't start bombing if planet has no armies
		if !p.Bombing && planet.Armies == 0 {
			return
		}
		// Toggle bombing state
		p.Bombing = !p.Bombing
		if p.Bombing && planet.Armies > 0 {
			// Send message about starting bombing (non-blocking)
			select {
			case c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s is bombing %s", formatPlayerName(p), planet.Name),
					"type": "info",
				},
			}:
			default:
			}
		} else if !p.Bombing {
			// Send message about stopping bombing (non-blocking)
			select {
			case c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s stopped bombing %s", formatPlayerName(p), planet.Name),
					"type": "info",
				},
			}:
			default:
			}
		}
	}
}
