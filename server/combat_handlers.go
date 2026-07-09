package server

import (
	"encoding/json"
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"log"
	"math"
)

// handleFire processes torpedo fire commands
func (c *Client) handleFire(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	var fireData FireData
	if err := json.Unmarshal(data, &fireData); err != nil {
		log.Printf("Error unmarshaling fire data: %v", err)
		return
	}

	// Validate direction
	fireData.Dir = game.NormalizeAngle(fireData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	// Check if can fire torpedo
	if p.NumTorps >= game.MaxTorps {
		return // Too many torps out
	}

	shipStats := game.ShipData[p.Ship]

	// Check fuel (using ship-specific multiplier)
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult
	if p.Fuel < torpCost {
		return // Not enough fuel
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return // Weapons too hot
	}

	// Fire torpedo
	torp := &game.Torpedo{
		ID:     c.server.nextTorpID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireData.Dir,
		Speed:  float64(shipStats.TorpSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse, // Use ship-specific torpedo fuse
		Status: game.TorpMove,      // Moving
		Team:   p.Team,
	}

	c.server.gameState.Torps = append(c.server.gameState.Torps, torp)
	c.server.nextTorpID++
	p.NumTorps++
	p.Fuel -= torpCost
	p.WTemp += 50
}

// handlePhaser processes phaser fire commands (using original Netrek algorithm)
func (c *Client) handlePhaser(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	var phaserData PhaserData
	if err := json.Unmarshal(data, &phaserData); err != nil {
		log.Printf("Error unmarshaling phaser data: %v", err)
		return
	}

	// Validate direction
	phaserData.Dir = game.NormalizeAngle(phaserData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check fuel (using ship-specific multiplier)
	phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
	if p.Fuel < phaserCost {
		return
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Consume fuel and increase weapon temp regardless of hit
	p.Fuel -= phaserCost
	p.WTemp += 70

	myPhaserRange := game.PhaserRange(shipStats)

	// Get phaser direction (use provided direction or calculate from target)
	var course float64
	if phaserData.Target >= 0 && phaserData.Target < game.MaxPlayers {
		// Calculate direction to specific target
		targetPlayer := c.server.gameState.Players[phaserData.Target]
		if targetPlayer != nil && targetPlayer.Status == game.StatusAlive {
			course = math.Atan2(targetPlayer.Y-p.Y, targetPlayer.X-p.X)
		} else {
			return // Invalid target
		}
	} else {
		// Use the provided direction (can be any angle from -π to π)
		course = phaserData.Dir
	}

	// Find the nearest enemy ship on the phaser line; keep rangeSq so the
	// plasma scan below only considers plasmas closer than the hit ship.
	target, targetDist, rangeSq := c.server.phaserTargetInLine(p, course, myPhaserRange)

	// (C, D) is a point on the phaser line, relative to me
	// Using 10*PHASEDIST like original to prevent round-off errors
	C := math.Cos(course) * 10 * float64(game.PhaserDist)
	D := math.Sin(course) * 10 * float64(game.PhaserDist)

	// Check plasma torpedoes (if they exist)
	for _, plasma := range c.server.gameState.Plasmas {
		if plasma == nil || plasma.Status != game.TorpMove || plasma.Owner == p.ID {
			continue
		}

		// Check if plasma is enemy
		if plasma.Team == p.Team {
			continue
		}

		A := plasma.X - p.X
		B := plasma.Y - p.Y

		if math.Abs(A) >= myPhaserRange || math.Abs(B) >= myPhaserRange {
			continue
		}

		thisRangeSq := A*A + B*B
		if thisRangeSq >= rangeSq {
			continue
		}

		s := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))
		if s < 0 {
			continue
		}

		E := C * s
		F := D * s
		dx := E - A
		dy := F - B

		// Use ZAPPLASMADIST for plasma hit detection
		if dx*dx+dy*dy <= float64(game.ZAPPLASMADIST*game.ZAPPLASMADIST) {
			// Destroy the plasma - mark as exploding; updatePlasmas() will decrement NumPlasma
			plasma.Status = game.TorpDet // Detonate

			log.Printf("Phaser destroyed plasma: player %d destroyed plasma from player %d", p.ID, plasma.Owner)

			// Send phaser visual to plasma location (non-blocking)
			c.server.tryBroadcast(ServerMessage{
				Type: "phaser",
				Data: map[string]interface{}{
					"from":  p.ID,
					"to":    -2, // Special code for plasma hit
					"x":     plasma.X,
					"y":     plasma.Y,
					"range": myPhaserRange,
				},
			})
			return // Plasma takes priority if hit
		}
	}

	// Fire at target if found
	if target != nil {
		// Calculate damage based on distance using original formula
		damage := float64(shipStats.PhaserDamage) * (1.0 - targetDist/myPhaserRange)
		log.Printf("Phaser hit: player %d hit player %d for %.1f damage at range %.0f", p.ID, target.ID, damage, targetDist)

		// Apply damage to shields first, then hull (round instead of truncate)
		actualDamage := game.ApplyDamageWithShields(target, int(math.Round(damage)))

		if target.Damage >= game.ShipData[target.Ship].MaxDamage {
			c.server.killPlayer(target, p.ID, game.KillPhaser, actualDamage)
		} else if c.server.gameState.T_mode {
			// Non-lethal hit: still track damage for tournament stats, matching
			// the torpedo and plasma hit paths.
			if stats, ok := c.server.gameState.TournamentStats[p.ID]; ok {
				stats.DamageDealt += actualDamage
			}
			if stats, ok := c.server.gameState.TournamentStats[target.ID]; ok {
				stats.DamageTaken += actualDamage
			}
		}

		// Send phaser visual to all players (non-blocking).
		// Use "target" (not "to") so the broadcast router does not treat this
		// as a private message routed only to the player that was hit.
		c.server.tryBroadcast(ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from":   p.ID,
				"target": target.ID,
				"range":  myPhaserRange,
			},
		})
	} else {
		// No target - phaser fires but misses
		// Send phaser visual with direction but no target (non-blocking)
		c.server.tryBroadcast(ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from":  p.ID,
				"to":    -1,     // -1 indicates no target
				"dir":   course, // Direction the phaser was fired
				"range": myPhaserRange,
			},
		})
	}
}

// handlePlasma processes plasma torpedo fire commands
func (c *Client) handlePlasma(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	var plasmaData PlasmaData
	if err := json.Unmarshal(data, &plasmaData); err != nil {
		log.Printf("Error unmarshaling plasma data: %v", err)
		return
	}

	// Validate direction
	plasmaData.Dir = game.NormalizeAngle(plasmaData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	// Check if ship has plasma capability
	shipStats := game.ShipData[p.Ship]
	if !shipStats.HasPlasma {
		return // Ship can't fire plasma
	}

	// Check if can fire plasma (only 1 at a time)
	if p.NumPlasma >= game.MaxPlasma {
		return // Already have plasma out
	}

	// Check fuel (using ship-specific multiplier)
	plasmaCost := shipStats.PlasmaDamage * shipStats.PlasmaFuelMult
	if p.Fuel < plasmaCost {
		return // Not enough fuel
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return // Weapons too hot
	}

	// Fire plasma torpedo
	plasma := &game.Plasma{
		ID:     c.server.nextPlasmaID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    plasmaData.Dir,
		Speed:  float64(shipStats.PlasmaSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse, // Use original fuse value directly (already scaled for our 10 FPS)
		Status: game.TorpMove,        // Moving
		Team:   p.Team,
	}

	c.server.gameState.Plasmas = append(c.server.gameState.Plasmas, plasma)
	c.server.nextPlasmaID++
	p.NumPlasma++
	p.Fuel -= plasmaCost
	p.WTemp += 100 // Plasma heats weapons more
}

// handleDetonate handles detonating own torpedoes
func (c *Client) handleDetonate(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Can't detonate while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for detonate cost
	shipStats := game.ShipData[p.Ship]

	// Find and detonate enemy torpedoes near this player
	detonatedCount := 0
	for _, torp := range c.server.gameState.Torps {
		if torp.Status != game.TorpMove || torp.Owner == p.ID {
			continue
		}
		// Only detonate enemy torpedoes using torp.Team directly
		if torp.Team == p.Team {
			continue
		}
		// Check if torpedo is within detonate range
		dist := game.Distance(p.X, p.Y, torp.X, torp.Y)
		if dist > float64(game.PhaserDist) {
			continue
		}
		{
			// Check if we have enough fuel
			if p.Fuel < shipStats.DetCost {
				// Not enough fuel to detonate (non-blocking)
				c.server.tryBroadcast(ServerMessage{
					Type: "message",
					Data: map[string]interface{}{
						"text": "Not enough fuel to detonate",
						"type": "error",
						"to":   p.ID, // Send only to this player
					},
				})
				break
			}
			// Mark the torpedo as detonating so updateTorpedoes removes it in
			// place next frame. Setting Fuse=1 instead would let the torp move
			// a full step and run its collision check one more tick, so a
			// "neutralized" torp could still strike a target before vanishing.
			torp.Status = game.TorpDet
			detonatedCount++
			// Deduct fuel cost
			p.Fuel -= shipStats.DetCost
		}
	}

	// Send feedback message (non-blocking)
	if detonatedCount > 0 {
		c.server.tryBroadcast(ServerMessage{
			Type: "message",
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s detonated %d torpedo(es)", formatPlayerName(p), detonatedCount),
				"type": "info",
			},
		})
	}
}

// handleShields toggles shields
func (c *Client) handleShields(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Toggle shields
	p.Shields_up = !p.Shields_up

	// Raising shields cancels repair mode and repair request
	if p.Shields_up {
		if p.RepairRequest {
			p.RepairRequest = false
		}
		if p.Repairing {
			p.Repairing = false
		}
	}
}

// handleTractor handles tractor beam engagement
func (c *Client) handleTractor(data json.RawMessage) {
	c.handleBeamEngage(data, false)
}

// handlePressor handles pressor beam engagement
func (c *Client) handlePressor(data json.RawMessage) {
	c.handleBeamEngage(data, true)
}

// handleBeamEngage toggles a tractor or pressor beam; the two beams share
// identical rules and range, differing only in which field they set (and
// engaging one clears the other).
func (c *Client) handleBeamEngage(data json.RawMessage, pressor bool) {
	if !c.validPlayerID() {
		return
	}

	var beamData struct {
		TargetID int `json:"targetId"`
	}
	if err := json.Unmarshal(data, &beamData); err != nil {
		log.Printf("Error unmarshaling tractor/pressor data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.getAlivePlayer()
	if p == nil {
		return
	}

	// Can't use tractor/pressor while cloaked
	if p.Cloaked {
		return
	}

	beam, other := &p.Tractoring, &p.Pressoring
	if pressor {
		beam, other = other, beam
	}

	// Engaging one beam clears the other
	*other = -1

	// Toggle beam
	if *beam == beamData.TargetID {
		// Turn off beam
		*beam = -1
	} else {
		// Check if target is valid and in range
		if beamData.TargetID >= 0 && beamData.TargetID < game.MaxPlayers {
			target := c.server.gameState.Players[beamData.TargetID]
			if target != nil && target.Status == game.StatusAlive && target.ID != p.ID {
				// Check range (using ship-specific range)
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				tractorRange := float64(game.TractorDist) * game.ShipData[p.Ship].TractorRange
				if dist <= tractorRange {
					*beam = beamData.TargetID
				}
			}
		}
	}
}

// handleCloak handles cloaking/uncloaking
func (c *Client) handleCloak(data json.RawMessage) {
	if !c.validPlayerID() {
		return
	}

	var message string

	// Lock scope: toggle cloak and build message
	func() {
		c.server.gameState.Mu.Lock()
		defer c.server.gameState.Mu.Unlock()

		p := c.getAlivePlayer()
		if p == nil {
			return
		}

		// Toggle cloak
		p.Cloaked = !p.Cloaked

		// Build message while holding lock
		if p.Cloaked {
			message = fmt.Sprintf("%s engaged cloaking device", formatPlayerName(p))
		} else {
			message = fmt.Sprintf("%s disengaged cloaking device", formatPlayerName(p))
		}
	}()

	// Send cloak status message to all clients (after releasing lock to avoid deadlock)
	if message != "" {
		c.server.broadcastInfo(message)
	}
}
