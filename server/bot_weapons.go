package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// targetVelocity returns the target's effective velocity in world units per tick.
// If the target is orbiting a planet, we compute tangential orbital velocity;
// otherwise we fall back to straight-line velocity from Speed/Dir.
func (s *Server) targetVelocity(t *game.Player) Vector2D {
	if vx, vy, ok := s.OrbitalVelocity(t); ok {
		return Vector2D{X: vx, Y: vy}
	}
	return Vector2D{
		X: t.Speed * math.Cos(t.Dir) * 20,
		Y: t.Speed * math.Sin(t.Dir) * 20,
	}
}

// Weapons AI Functions
// This file contains all functions related to weapon operations:
// - Torpedo firing and targeting
// - Phaser firing and calculations
// - Plasma torpedo operations
// - Weapon spread patterns
// - Enhanced targeting algorithms

// fireBotTorpedo fires a torpedo from a bot
func (s *Server) fireBotTorpedo(p *game.Player, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check torpedo count
	if p.NumTorps >= game.MaxTorps {
		return
	}

	// Check fuel (same formula as human handler)
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult
	if p.Fuel < torpCost {
		return
	}

	// Check weapon temperature
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Use unified intercept solver
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick

	// Calculate intercept direction
	fireDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	// Add small random jitter to make bot torpedoes harder to dodge
	fireDir += randomJitterRad()

	// Create torpedo
	torp := &game.Torpedo{
		ID:     s.nextTorpID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.TorpSpeed * 20), // 20 units per tick
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: game.TorpMove, // Moving
		Team:   p.Team,        // Set team color
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	s.nextTorpID++
	p.NumTorps++
	p.Fuel -= torpCost
	p.WTemp += 50
}

// fireBotPhaser fires a phaser from a bot using the same line-to-circle hit
// detection algorithm as human phasers (combat_handlers.go handlePhaser).
func (s *Server) fireBotPhaser(p *game.Player, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check fuel (same formula as human handler)
	phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
	if p.Fuel < phaserCost {
		return
	}

	// Check weapon temperature
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Calculate phaser range using original formula: PHASEDIST * phaserdamage / 100
	myPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0

	// Bot aims directly at target
	course := math.Atan2(target.Y-p.Y, target.X-p.X)

	// Use the same line-to-circle algorithm as human phasers:
	// (C, D) is a point on the phaser line, relative to the bot
	C := math.Cos(course) * 10 * float64(game.PhaserDist)
	D := math.Sin(course) * 10 * float64(game.PhaserDist)

	var hitTarget *game.Player
	var hitDist float64
	rangeSq := myPhaserRange*myPhaserRange + 1

	for _, enemy := range s.gameState.Players {
		if enemy == nil || enemy.Status != game.StatusAlive || enemy.Team == p.Team {
			continue
		}

		A := enemy.X - p.X
		B := enemy.Y - p.Y

		if math.Abs(A) >= myPhaserRange || math.Abs(B) >= myPhaserRange {
			continue
		}

		thisRangeSq := A*A + B*B
		if thisRangeSq >= rangeSq {
			continue
		}

		// Calculate point on phaser line nearest to target
		paramS := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))
		if paramS < 0 {
			paramS = 0
		}

		E := C * paramS
		F := D * paramS

		dx := E - A
		dy := F - B

		if dx*dx+dy*dy <= float64(game.ZAPPLAYERDIST*game.ZAPPLAYERDIST) {
			if hitTarget == nil || thisRangeSq < hitDist*hitDist {
				hitTarget = enemy
				hitDist = math.Sqrt(thisRangeSq)
				rangeSq = thisRangeSq
			}
		}
	}

	// Consume fuel and increase weapon temp regardless of hit (same as human)
	p.Fuel -= phaserCost
	p.WTemp += 70

	if hitTarget == nil {
		// Miss — send phaser visual with no target
		select {
		case s.broadcast <- ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from":  p.ID,
				"to":    -1,
				"dir":   course,
				"range": myPhaserRange,
			},
		}:
		default:
		}
		return
	}

	// Calculate damage based on distance using original formula
	damage := float64(shipStats.PhaserDamage) * (1.0 - hitDist/myPhaserRange)
	game.ApplyDamageWithShields(hitTarget, int(damage))

	// Check if target destroyed
	targetStats := game.ShipData[hitTarget.Ship]
	if hitTarget.Damage >= targetStats.MaxDamage {
		hitTarget.Status = game.StatusExplode
		hitTarget.ExplodeTimer = game.ExplodeTimerFrames
		hitTarget.KilledBy = p.ID
		hitTarget.WhyDead = game.KillPhaser
		hitTarget.Bombing = false   // Stop bombing when destroyed
		hitTarget.Beaming = false   // Stop beaming when destroyed
		hitTarget.BeamingUp = false // Clear beam direction
		hitTarget.Orbiting = -1     // Break orbit when destroyed
		// Clear lock-on when destroyed
		hitTarget.LockType = "none"
		hitTarget.LockTarget = -1
		hitTarget.Deaths++ // Increment death count
		p.Kills += 1
		p.KillsStreak += 1

		// Send death message
		s.broadcastDeathMessage(hitTarget, p)
	}

	// Create phaser visual
	select {
	case s.broadcast <- ServerMessage{
		Type: "phaser",
		Data: map[string]interface{}{
			"from":  p.ID,
			"to":    hitTarget.ID,
			"range": myPhaserRange,
		},
	}:
	default:
	}
}

// fireBotPhaserAtPlasma fires a phaser at an incoming plasma torpedo to destroy it
func (s *Server) fireBotPhaserAtPlasma(p *game.Player, plasma *game.Plasma) bool {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return false
	}

	shipStats := game.ShipData[p.Ship]
	dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)

	// Calculate phaser range using original formula
	myPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0

	if dist > myPhaserRange {
		return false
	}

	// Check fuel cost
	phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
	if p.Fuel < phaserCost {
		return false
	}

	// Check weapon temperature
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return false
	}

	// Calculate phaser direction to plasma
	phaserDir := math.Atan2(plasma.Y-p.Y, plasma.X-p.X)

	// Check if phaser would hit the plasma using ZAPPLASMADIST
	// This mirrors the logic in combat_handlers.go
	C := 10.0 * float64(game.PhaserDist) * math.Cos(phaserDir)
	D := 10.0 * float64(game.PhaserDist) * math.Sin(phaserDir)

	A := plasma.X - p.X
	B := plasma.Y - p.Y

	s_param := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))
	if s_param < 0 {
		return false
	}

	E := C * s_param
	F := D * s_param
	dx := E - A
	dy := F - B

	// Use ZAPPLASMADIST for plasma hit detection
	if dx*dx+dy*dy > float64(game.ZAPPLASMADIST*game.ZAPPLASMADIST) {
		return false
	}

	// Destroy the plasma - mark as exploding; updatePlasmas() will decrement NumPlasma
	plasma.Status = game.TorpDet // Detonate

	// Send phaser visual to plasma location
	select {
	case s.broadcast <- ServerMessage{
		Type: "phaser",
		Data: map[string]interface{}{
			"from":  p.ID,
			"to":    -2, // Special code for plasma hit
			"x":     plasma.X,
			"y":     plasma.Y,
			"range": myPhaserRange,
		},
	}:
	default:
	}

	p.Fuel -= phaserCost

	return true
}

// tryPhaserNearbyPlasma checks for enemy plasma in range and attempts to phaser it
// Returns true if a plasma was phasered
func (s *Server) tryPhaserNearbyPlasma(p *game.Player) bool {
	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return false
	}

	shipStats := game.ShipData[p.Ship]
	myPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0

	// Check fuel and temperature
	phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
	if p.Fuel < phaserCost || p.WTemp > shipStats.MaxWpnTemp-100 {
		return false
	}

	// Find the closest threatening plasma within phaser range
	var closestPlasma *game.Plasma
	closestDist := myPhaserRange

	for _, plasma := range s.gameState.Plasmas {
		// Skip our own plasma or non-active plasma
		if plasma.Owner == p.ID || plasma.Status != game.TorpMove {
			continue
		}

		// Skip friendly plasma
		if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
			if owner := s.gameState.Players[plasma.Owner]; owner != nil && owner.Team == p.Team {
				continue
			}
		}

		dist := game.Distance(p.X, p.Y, plasma.X, plasma.Y)
		if dist < closestDist {
			closestPlasma = plasma
			closestDist = dist
		}
	}

	if closestPlasma != nil {
		return s.fireBotPhaserAtPlasma(p, closestPlasma)
	}

	return false
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

	// Check fuel (using ship-specific multiplier, same as human handler)
	plasmaCost := shipStats.PlasmaDamage * shipStats.PlasmaFuelMult
	if p.Fuel < plasmaCost {
		return
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Pre-fire sanity check: don't fire beyond plasma maximum range
	dist := game.Distance(p.X, p.Y, target.X, target.Y)
	maxPlasmaRange := game.MaxPlasmaRangeForShip(p.Ship)
	if dist > maxPlasmaRange {
		// Don't fire - plasma would expire before reaching target
		logPlasmaFiring("SKIPPED", int(p.Ship), dist, maxPlasmaRange, "target beyond max range")
		return
	}

	// Use unified intercept solver for plasma
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.PlasmaSpeed * 20) // Convert to units/tick
	fireDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	// Create plasma
	plasma := &game.Plasma{
		ID:     s.nextPlasmaID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.PlasmaSpeed * 20), // 20 units per tick
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse, // Use original fuse value directly
		Status: game.TorpMove,        // Moving
		Team:   p.Team,               // Set team color
	}

	s.gameState.Plasmas = append(s.gameState.Plasmas, plasma)
	s.nextPlasmaID++
	p.NumPlasma++
	p.Fuel -= plasmaCost
	p.WTemp += 100 // Plasma heats weapons (matching human handler)

	// Log successful plasma firing
	logPlasmaFiring("FIRED", int(p.Ship), dist, maxPlasmaRange, "within range")
}

// fireBotTorpedoWithLead fires torpedo with advanced leading
func (s *Server) fireBotTorpedoWithLead(p, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check torpedo count
	if p.NumTorps >= game.MaxTorps {
		return
	}

	// Check fuel (same formula as human handler)
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult
	if p.Fuel < torpCost {
		return
	}

	// Check weapon temperature
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Use unified intercept solver
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick

	// Calculate intercept direction
	fireDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	// Add small random jitter
	fireDir += randomJitterRad()

	// Create torpedo
	torp := &game.Torpedo{
		ID:     s.nextTorpID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  projSpeed,
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: game.TorpMove,
		Team:   p.Team,
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	s.nextTorpID++
	p.NumTorps++
	p.Fuel -= torpCost
	p.WTemp += 50
}

// fireTorpedoSpread fires multiple torpedoes in a spread pattern
func (s *Server) fireTorpedoSpread(p, target *game.Player, count int) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult

	// Use unified intercept solver for base direction
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick
	baseDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	spreadAngle := math.Pi / 16 // Spread angle between torpedoes

	for i := 0; i < count; i++ {
		if p.NumTorps >= game.MaxTorps {
			break
		}
		// Check fuel for each torpedo
		if p.Fuel < torpCost {
			break
		}
		// Check weapon temperature
		if p.WTemp > shipStats.MaxWpnTemp-100 {
			break
		}

		// Calculate spread direction
		offset := float64(i-count/2) * spreadAngle
		fireDir := baseDir + offset
		// Add small random jitter to make each torpedo harder to dodge
		fireDir += randomJitterRad()

		// Create torpedo
		torp := &game.Torpedo{
			ID:     s.nextTorpID,
			Owner:  p.ID,
			X:      p.X,
			Y:      p.Y,
			Dir:    fireDir,
			Speed:  float64(shipStats.TorpSpeed * 20),
			Damage: shipStats.TorpDamage,
			Fuse:   shipStats.TorpFuse,
			Status: game.TorpMove,
			Team:   p.Team,
		}

		s.gameState.Torps = append(s.gameState.Torps, torp)
		s.nextTorpID++
		p.NumTorps++
		p.Fuel -= torpCost
		p.WTemp += 50
	}
}

// fireEnhancedTorpedo fires a torpedo with enhanced prediction
func (s *Server) fireEnhancedTorpedo(p, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check torpedo count
	if p.NumTorps >= game.MaxTorps {
		return
	}

	// Check fuel (same formula as human handler)
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult
	if p.Fuel < torpCost {
		return
	}

	// Check weapon temperature
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Use unified intercept solver
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick
	fireDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	// Add small random jitter to make bot torpedoes harder to dodge
	fireDir += randomJitterRad()

	torp := &game.Torpedo{
		ID:     s.nextTorpID,
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  float64(shipStats.TorpSpeed * 20),
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: game.TorpMove,
		Team:   p.Team,
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	s.nextTorpID++
	p.NumTorps++
	p.Fuel -= torpCost
	p.WTemp += 50
}

// planetDefenseWeaponLogic implements aggressive weapon usage for planet defense
func (s *Server) planetDefenseWeaponLogic(p *game.Player, enemy *game.Player, enemyDist float64) {
	shipStats := game.ShipData[p.Ship]
	firedWeapon := false

	// Weapon usage for planet defense - no facing restrictions needed

	// Aggressive torpedo usage - wider criteria than normal combat
	// Torpedoes can be fired in any direction regardless of ship facing
	// Use velocity-adjusted range to prevent fuse expiry on fast targets
	effectiveTorpRange := s.getVelocityAdjustedTorpRange(p, enemy)
	if enemyDist < effectiveTorpRange && p.NumTorps < game.MaxTorps-1 && p.Fuel > 1500 && p.WTemp < shipStats.MaxWpnTemp-100 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 4 // Faster firing rate for planet defense
		firedWeapon = true
	}

	// Opportunistic phaser usage - prioritize planet protection over fuel conservation
	// Phasers can be fired in any direction regardless of ship facing
	myPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0
	if !firedWeapon && enemyDist < myPhaserRange && p.Fuel > 1000 && p.WTemp < shipStats.MaxWpnTemp-100 {
		// Fire phasers more liberally when defending planets
		s.fireBotPhaser(p, enemy)
		p.BotCooldown = 8
		firedWeapon = true
	}

	// Enhanced plasma usage for ships that have it - use actual plasma range
	maxPlasmaRange := game.MaxPlasmaRangeForShip(p.Ship)
	plasmaDefenseRange := game.EffectivePlasmaRange(p.Ship, 0.90) // 90% of max plasma range
	plasmaMinRange := maxPlasmaRange * 0.25                       // 25% of max plasma range
	if !firedWeapon && shipStats.HasPlasma && p.NumPlasma < 1 && enemyDist < plasmaDefenseRange && enemyDist > plasmaMinRange && p.Fuel > 3000 {
		s.fireBotPlasma(p, enemy)
		p.BotCooldown = 15
	}
}

// starbaseDefenseWeaponLogic implements weapon usage for starbase planet defense
func (s *Server) starbaseDefenseWeaponLogic(p *game.Player, enemy *game.Player, enemyDist float64) {
	shipStats := game.ShipData[p.Ship]

	// Starbase weapon usage for planet defense - no facing restrictions needed

	// More aggressive torpedo usage than normal starbase combat
	// Torpedoes can be fired in any direction regardless of ship facing
	// Use velocity-adjusted range to prevent fuse expiry
	effectiveTorpRange := s.getVelocityAdjustedTorpRange(p, enemy)
	if enemyDist < effectiveTorpRange && p.NumTorps < game.MaxTorps-1 && p.Fuel > 2500 && p.WTemp < shipStats.MaxWpnTemp/2 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 6 // Faster than normal starbase firing
		return
	}

	// Aggressive phaser usage for planet defense
	// Phasers can be fired in any direction regardless of ship facing
	// Use the canonical phaser range formula for consistency
	sbPhaserRange := float64(game.PhaserDist) * float64(shipStats.PhaserDamage) / 100.0
	if enemyDist < sbPhaserRange && p.Fuel > 1500 && p.WTemp < shipStats.MaxWpnTemp-100 {
		s.fireBotPhaser(p, enemy)
		p.BotCooldown = 8
		return
	}

	// Wider plasma usage window for area denial
	if shipStats.HasPlasma && p.NumPlasma < 1 && enemyDist < game.StarbasePlasmaMaxRange && enemyDist > 1500 && p.Fuel > 3500 {
		s.fireBotPlasma(p, enemy)
		p.BotCooldown = 18
		return
	}

	// Unconditional close-range torpedo fallback - fires even when other conditions prevent it
	if enemyDist < game.StarbaseTorpRange && p.NumTorps < game.MaxTorps && p.Fuel > 2000 && p.WTemp < shipStats.MaxWpnTemp-100 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 10
		return
	}
}

// detonatePassingTorpedoes checks each torpedo individually and only detonates
// torpedoes that are passing by enemies (not heading for direct hits).
// This avoids the previous bug where ALL torpedoes were detonated when one triggered.
func (s *Server) detonatePassingTorpedoes(p *game.Player) {
	if p.NumTorps == 0 {
		return
	}

	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID || torp.Status != game.TorpMove {
			continue
		}

		// Check if this specific torpedo should be detonated
		for _, enemy := range s.gameState.Players {
			if enemy.Status != game.StatusAlive || enemy.Team == p.Team {
				continue
			}

			dist := game.Distance(torp.X, torp.Y, enemy.X, enemy.Y)

			// Detonate if enemy is in blast radius but torpedo won't hit directly
			if dist < 2500 && dist > 800 {
				dx := enemy.X - torp.X
				dy := enemy.Y - torp.Y
				angleToEnemy := math.Atan2(dy, dx)
				angleDiff := math.Abs(angleToEnemy - torp.Dir)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}

				// Only detonate if torpedo is clearly passing by (not heading at) the enemy
				if angleDiff > math.Pi/4 {
					torp.Fuse = 1 // Detonate this specific torpedo
					break         // Move to next torpedo
				}
			}

			// Detonate if 3+ enemies are clustered near this torpedo
			if dist < 3000 {
				nearbyCount := 0
				for _, other := range s.gameState.Players {
					if other.Status == game.StatusAlive && other.Team != p.Team {
						if game.Distance(torp.X, torp.Y, other.X, other.Y) < 3000 {
							nearbyCount++
						}
					}
				}
				if nearbyCount >= 3 {
					torp.Fuse = 1 // Detonate for area damage on clustered enemies
					break         // Move to next torpedo
				}
			}
		}
	}
}
