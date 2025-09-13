package server

import (
	"encoding/json"
	"fmt"
	"log"
	"math"

	"github.com/lab1702/netrek-web/game"
)

// handleMove processes movement commands
func (c *Client) handleMove(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var moveData MoveData
	if err := json.Unmarshal(data, &moveData); err != nil {
		return
	}

	// Validate direction
	moveData.Dir = validateDirection(moveData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Break orbit if setting new course
	if p.Orbiting >= 0 {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when leaving orbit
		// Send message about breaking orbit
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s has left orbit", formatPlayerName(p)),
				"type": "info",
			},
		}
	}

	// Set desired direction and speed
	p.DesDir = game.NormalizeAngle(moveData.Dir)

	// Clear lock when manually setting course
	p.LockType = "none"
	p.LockTarget = -1

	// Cancel repair mode and repair request when setting speed > 0 (unless orbiting)
	if moveData.Speed > 0 && p.Orbiting < 0 {
		if p.RepairRequest {
			p.RepairRequest = false
			// Send message about canceling repair request
			c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s canceled repair request", formatPlayerName(p)),
					"type": "info",
				},
			}
		}
		p.Repairing = false
	}

	// Calculate max speed based on damage
	shipStats := game.ShipData[p.Ship]
	maxSpeed := float64(shipStats.MaxSpeed)
	if p.Damage > 0 {
		// Formula from original Netrek: maxspeed = (max + 2) - (max + 1) * (damage / maxdamage)
		damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
		maxSpeed = float64(shipStats.MaxSpeed+2) - float64(shipStats.MaxSpeed+1)*damageRatio
		maxSpeed = math.Max(1, maxSpeed) // Minimum speed of 1
	}

	// Clamp speed to damage-adjusted maximum
	p.DesSpeed = math.Max(0, math.Min(moveData.Speed, maxSpeed))
}

// handleLock handles lock-on to players or planets
func (c *Client) handleLock(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var lockData struct {
		Type   string `json:"type"`   // "player" or "planet"
		Target int    `json:"target"` // Target ID
	}
	if err := json.Unmarshal(data, &lockData); err != nil {
		return
	}

	// Validate lock target type - only allow "planet" or "none"
	if lockData.Type != "planet" && lockData.Type != "none" {
		return
	}

	// Validate target ID based on type
	if lockData.Type == "planet" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlanets {
			return
		}
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Break orbit when locking onto a new target (unless locking the planet we're orbiting)
	if p.Orbiting >= 0 && (lockData.Type != "planet" || lockData.Target != p.Orbiting) {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when leaving orbit
		p.Beaming = false // Stop beaming when leaving orbit
		// Send message about breaking orbit
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s has left orbit", formatPlayerName(p)),
				"type": "info",
			},
		}
	}

	// Validate target and set course
	if lockData.Type == "planet" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlanets {
			return
		}
		planet := c.server.gameState.Planets[lockData.Target]
		p.LockType = "planet"
		p.LockTarget = lockData.Target

		// Set desired course toward planet (ship will turn at normal rate)
		dx := planet.X - p.X
		dy := planet.Y - p.Y
		p.DesDir = math.Atan2(dy, dx)
	} else if lockData.Type == "none" {
		// Clear lock
		p.LockType = "none"
		p.LockTarget = -1
	}
}

// handleOrbit toggles orbit around nearest planet
func (c *Client) handleOrbit(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// If already orbiting, break orbit
	if p.Orbiting >= 0 {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when breaking orbit
		p.Beaming = false // Stop beaming when breaking orbit
		return
	}

	// Check if going slow enough to orbit (max warp 2)
	if p.Speed > float64(game.ORBSPEED) {
		// Too fast to orbit - silently fail like original
		return
	}

	// Find nearest planet within orbit entry distance
	closestPlanet := -1
	closestDist := float64(game.EntOrbitDist)

	for i := 0; i < game.MaxPlanets; i++ {
		planet := c.server.gameState.Planets[i]
		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist <= closestDist {
			closestDist = dist
			closestPlanet = i
		}
	}

	if closestPlanet >= 0 {
		p.Orbiting = closestPlanet
		p.Speed = 0
		p.DesSpeed = 0

		// Clear lock when entering orbit
		p.LockType = "none"
		p.LockTarget = -1

		// Clear tractor/pressor beams when entering orbit (from original)
		p.Tractoring = -1
		p.Pressoring = -1

		// Calculate initial orbit position at correct radius
		planet := c.server.gameState.Planets[closestPlanet]
		angle := math.Atan2(p.Y-planet.Y, p.X-planet.X)
		p.X = planet.X + float64(game.OrbitDist)*math.Cos(angle)
		p.Y = planet.Y + float64(game.OrbitDist)*math.Sin(angle)

		// Set direction tangent to orbit (perpendicular to radius)
		// In original: dir + 64 where 256 units = 2*PI, so 64 = PI/2
		p.Dir = angle + math.Pi/2
		p.DesDir = p.Dir

		// Update planet info - team now has scouted this planet
		oldInfo := planet.Info
		planet.Info |= p.Team
		log.Printf("Player %s (team %d) orbited planet %s. Info updated from %d to %d",
			p.Name, p.Team, planet.Name, oldInfo, planet.Info)
	}
}
