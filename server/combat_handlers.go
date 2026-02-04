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
	fireData.Dir = validateDirection(fireData.Dir)

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
		Owner:  c.GetPlayerID(),
		X:      p.X,
		Y:      p.Y,
		Dir:    fireData.Dir,
		Speed:  float64(shipStats.TorpSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse, // Use ship-specific torpedo fuse
		Status: 1,                  // Moving
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
	if c.GetPlayerID() < 0 || c.GetPlayerID() >= game.MaxPlayers {
		return
	}

	var phaserData PhaserData
	if err := json.Unmarshal(data, &phaserData); err != nil {
		log.Printf("Error unmarshaling phaser data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.GetPlayerID()]
	if p == nil || p.Status != game.StatusAlive {
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

	// Calculate phaser range using original formula: PHASEDIST * phaserdamage / 100
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)

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

	// (C, D) is a point on the phaser line, relative to me
	// Using 10*PHASEDIST like original to prevent round-off errors
	C := math.Cos(course) * 10 * float64(game.PhaserDist)
	D := math.Sin(course) * 10 * float64(game.PhaserDist)

	// Initialize search parameters
	var target *game.Player
	var targetDist float64
	rangeSq := myPhaserRange*myPhaserRange + 1 // +1 to ensure we check exact range

	// Check all enemy players using the original line-to-circle algorithm
	for _, enemy := range c.server.gameState.Players {
		if enemy == nil || enemy.Status != game.StatusAlive || enemy.Team == p.Team {
			continue
		}

		// (A, B) is the position of the possible target relative to me
		A := enemy.X - p.X
		B := enemy.Y - p.Y

		// Quick bounds check
		if math.Abs(A) >= myPhaserRange || math.Abs(B) >= myPhaserRange {
			continue
		}

		// Check if within phaser range
		thisRangeSq := A*A + B*B
		if thisRangeSq >= rangeSq {
			continue
		}

		// Calculate point on phaser line nearest to target
		// s is the parameter for the point on the line closest to the target
		s := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))

		if s < 0 {
			s = 0 // Handle case where target is behind the ship
		}

		E := C * s
		F := D * s

		// Check if the closest point on the phaser line is within hit distance
		dx := E - A
		dy := F - B

		// Use ZAPPLAYERDIST for hit detection
		if dx*dx+dy*dy <= float64(game.ZAPPLAYERDIST*game.ZAPPLAYERDIST) {
			// A hit! Update if this is closer than previous target
			if target == nil || thisRangeSq < targetDist*targetDist {
				target = enemy
				targetDist = math.Sqrt(thisRangeSq)
				rangeSq = thisRangeSq // Narrow search to closer targets
			}
		}
	}

	// Check plasma torpedoes (if they exist)
	for _, plasma := range c.server.gameState.Plasmas {
		if plasma == nil || plasma.Status != 1 || plasma.Owner == c.GetPlayerID() {
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
			plasma.Status = 3 // Detonate

			log.Printf("Phaser destroyed plasma: player %d destroyed plasma from player %d", c.GetPlayerID(), plasma.Owner)

			// Send phaser visual to plasma location (non-blocking)
			select {
			case c.server.broadcast <- ServerMessage{
				Type: "phaser",
				Data: map[string]interface{}{
					"from":  c.GetPlayerID(),
					"to":    -2, // Special code for plasma hit
					"x":     plasma.X,
					"y":     plasma.Y,
					"range": myPhaserRange,
				},
			}:
			default:
			}
			return // Plasma takes priority if hit
		}
	}

	// Fire at target if found
	if target != nil {
		// Calculate damage based on distance using original formula
		damage := float64(shipStats.PhaserDamage) * (1.0 - targetDist/myPhaserRange)
		log.Printf("Phaser hit: player %d hit player %d for %.1f damage at range %.0f", c.GetPlayerID(), target.ID, damage, targetDist)

		// Apply damage to shields first, then hull
		game.ApplyDamageWithShields(target, int(damage))

		if target.Damage >= game.ShipData[target.Ship].MaxDamage {
			// Ship destroyed by phaser!
			target.Status = game.StatusExplode
			target.ExplodeTimer = game.ExplodeTimerFrames
			target.KilledBy = c.GetPlayerID()
			target.WhyDead = game.KillPhaser
			target.Bombing = false   // Stop bombing when destroyed
			target.Beaming = false   // Stop beaming when destroyed
			target.BeamingUp = false // Clear beam direction
			target.Orbiting = -1     // Break orbit when destroyed
			// Clear lock-on when destroyed
			target.LockType = "none"
			target.LockTarget = -1
			target.Deaths++ // Increment death count
			p.Kills += 1
			p.KillsStreak += 1

			// Update tournament stats
			if c.server.gameState.T_mode {
				if stats, ok := c.server.gameState.TournamentStats[c.GetPlayerID()]; ok {
					stats.Kills++
					stats.DamageDealt += int(damage)
				}
				if stats, ok := c.server.gameState.TournamentStats[target.ID]; ok {
					stats.Deaths++
					stats.DamageTaken += int(damage)
				}
			}

			// Send death message
			c.server.broadcastDeathMessage(target, p)
		}

		// Send phaser visual (non-blocking)
		select {
		case c.server.broadcast <- ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from":  c.GetPlayerID(),
				"to":    target.ID,
				"range": myPhaserRange,
			},
		}:
		default:
		}
	} else {
		// No target - phaser fires but misses
		// Send phaser visual with direction but no target (non-blocking)
		select {
		case c.server.broadcast <- ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from":  c.GetPlayerID(),
				"to":    -1,     // -1 indicates no target
				"dir":   course, // Direction the phaser was fired
				"range": myPhaserRange,
			},
		}:
		default:
		}
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
	plasmaData.Dir = validateDirection(plasmaData.Dir)

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
		Owner:  c.GetPlayerID(),
		X:      p.X,
		Y:      p.Y,
		Dir:    plasmaData.Dir,
		Speed:  float64(shipStats.PlasmaSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse, // Use original fuse value directly (already scaled for our 10 FPS)
		Status: 1,                    // Moving
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
	if c.GetPlayerID() < 0 || c.GetPlayerID() >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.GetPlayerID()]
	if p.Status != game.StatusAlive {
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
		if torp.Status != 1 || torp.Owner == c.GetPlayerID() {
			continue
		}
		// Only detonate enemy torpedoes (skip friendly)
		if torp.Owner < 0 || torp.Owner >= game.MaxPlayers {
			continue
		}
		torpOwner := c.server.gameState.Players[torp.Owner]
		if torpOwner == nil || torpOwner.Team == p.Team {
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
				select {
				case c.server.broadcast <- ServerMessage{
					Type: "message",
					Data: map[string]interface{}{
						"text": "Not enough fuel to detonate",
						"type": "error",
						"to":   c.GetPlayerID(), // Send only to this player
					},
				}:
				default:
				}
				break
			}
			// Set fuse to 1 so it will explode next frame
			torp.Fuse = 1
			detonatedCount++
			// Deduct fuel cost
			p.Fuel -= shipStats.DetCost
		}
	}

	// Send feedback message (non-blocking)
	if detonatedCount > 0 {
		select {
		case c.server.broadcast <- ServerMessage{
			Type: "message",
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s detonated %d torpedo(es)", formatPlayerName(p), detonatedCount),
				"type": "info",
			},
		}:
		default:
		}
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
	if c.GetPlayerID() < 0 || c.GetPlayerID() >= game.MaxPlayers {
		return
	}

	var tractorData struct {
		TargetID int `json:"targetId"`
	}
	if err := json.Unmarshal(data, &tractorData); err != nil {
		log.Printf("Error unmarshaling tractor data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.GetPlayerID()]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't use tractor while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for range calculation
	shipStats := game.ShipData[p.Ship]

	// Clear pressor if engaging tractor
	p.Pressoring = -1

	// Toggle tractor beam
	if p.Tractoring == tractorData.TargetID {
		// Turn off tractor
		p.Tractoring = -1
	} else {
		// Check if target is valid and in range
		if tractorData.TargetID >= 0 && tractorData.TargetID < game.MaxPlayers {
			target := c.server.gameState.Players[tractorData.TargetID]
			if target.Status == game.StatusAlive && target.ID != c.GetPlayerID() {
				// Check range (using ship-specific range)
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				tractorRange := float64(game.TractorDist) * shipStats.TractorRange
				if dist <= tractorRange {
					p.Tractoring = tractorData.TargetID
				}
			}
		}
	}
}

// handlePressor handles pressor beam engagement
func (c *Client) handlePressor(data json.RawMessage) {
	if c.GetPlayerID() < 0 || c.GetPlayerID() >= game.MaxPlayers {
		return
	}

	var pressorData struct {
		TargetID int `json:"targetId"`
	}
	if err := json.Unmarshal(data, &pressorData); err != nil {
		log.Printf("Error unmarshaling pressor data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.GetPlayerID()]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't use pressor while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for range calculation
	shipStats := game.ShipData[p.Ship]

	// Clear tractor if engaging pressor
	p.Tractoring = -1

	// Toggle pressor beam
	if p.Pressoring == pressorData.TargetID {
		// Turn off pressor
		p.Pressoring = -1
	} else {
		// Check if target is valid and in range
		if pressorData.TargetID >= 0 && pressorData.TargetID < game.MaxPlayers {
			target := c.server.gameState.Players[pressorData.TargetID]
			if target.Status == game.StatusAlive && target.ID != c.GetPlayerID() {
				// Check range (using ship-specific range)
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				tractorRange := float64(game.TractorDist) * shipStats.TractorRange
				if dist <= tractorRange {
					p.Pressoring = pressorData.TargetID
				}
			}
		}
	}
}

// handleCloak handles cloaking/uncloaking
func (c *Client) handleCloak(data json.RawMessage) {
	if c.GetPlayerID() < 0 || c.GetPlayerID() >= game.MaxPlayers {
		return
	}

	var message string

	// Lock scope: toggle cloak and build message
	func() {
		c.server.gameState.Mu.Lock()
		defer c.server.gameState.Mu.Unlock()

		p := c.server.gameState.Players[c.GetPlayerID()]
		if p.Status != game.StatusAlive {
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
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": message,
				"type": "info",
			},
		}
	}
}
