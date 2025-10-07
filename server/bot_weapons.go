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

	// Apply damage to shields first, then hull
	game.ApplyDamageWithShields(target, int(damage))

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
		ID:     len(s.gameState.Torps),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  projSpeed,
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse,
		Status: 1,
		Team:   p.Team,
	}

	s.gameState.Torps = append(s.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= shipStats.TorpDamage * shipStats.TorpFuelMult
}

// fireTorpedoSpread fires multiple torpedoes in a spread pattern
func (s *Server) fireTorpedoSpread(p, target *game.Player, count int) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

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

		// Calculate spread direction
		offset := float64(i-count/2) * spreadAngle
		fireDir := baseDir + offset
		// Add small random jitter to make each torpedo harder to dodge
		fireDir += randomJitterRad()

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

	shipStats := game.ShipData[p.Ship]

	// Use unified intercept solver
	shooterPos := Point2D{X: p.X, Y: p.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}
	targetVel := s.targetVelocity(target)
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick
	fireDir, _ := InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

	// Add small random jitter to make bot torpedoes harder to dodge
	fireDir += randomJitterRad()

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

// planetDefenseWeaponLogic implements aggressive weapon usage for planet defense
func (s *Server) planetDefenseWeaponLogic(p *game.Player, enemy *game.Player, enemyDist float64) {
	shipStats := game.ShipData[p.Ship]

	// Weapon usage for planet defense - no facing restrictions needed

	// Aggressive torpedo usage - wider criteria than normal combat
	// Torpedoes can be fired in any direction regardless of ship facing
	// Use velocity-adjusted range to prevent fuse expiry on fast targets
	effectiveTorpRange := s.getVelocityAdjustedTorpRange(p, enemy)
	if enemyDist < effectiveTorpRange && p.NumTorps < game.MaxTorps-1 && p.Fuel > 1500 && p.WTemp < 85 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 4 // Faster firing rate for planet defense
		return
	}

	// Opportunistic phaser usage - prioritize planet protection over fuel conservation
	// Phasers can be fired in any direction regardless of ship facing
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)
	if enemyDist < myPhaserRange && p.Fuel > 1000 && p.WTemp < 75 {
		// Fire phasers more liberally when defending planets
		s.fireBotPhaser(p, enemy)
		p.BotCooldown = 8
		return
	}

	// Enhanced plasma usage for ships that have it - use actual plasma range
	maxPlasmaRange := game.MaxPlasmaRangeForShip(p.Ship)
	plasmaDefenseRange := game.EffectivePlasmaRange(p.Ship, 0.90) // 90% of max plasma range
	plasmaMinRange := maxPlasmaRange * 0.25                       // 25% of max plasma range
	if shipStats.HasPlasma && p.NumPlasma < 1 && enemyDist < plasmaDefenseRange && enemyDist > plasmaMinRange && p.Fuel > 3000 {
		s.fireBotPlasma(p, enemy)
		p.BotCooldown = 15
		return
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
	if enemyDist < effectiveTorpRange && p.NumTorps < game.MaxTorps-1 && p.Fuel > 2500 && p.WTemp < 650 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 6 // Faster than normal starbase firing
		return
	}

	// Aggressive phaser usage for planet defense
	// Phasers can be fired in any direction regardless of ship facing
	if enemyDist < game.StarbasePhaserRange && p.Fuel > 1500 && p.WTemp < 700 {
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
	if enemyDist < game.StarbaseTorpRange && p.NumTorps < game.MaxTorps && p.Fuel > 2000 && p.WTemp < 800 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 10
		return
	}
}

// considerTorpedoDetonation checks if bot should detonate torpedoes for area denial
func (s *Server) considerTorpedoDetonation(p *game.Player) bool {
	// Check if we have torpedoes in flight
	if p.NumTorps == 0 {
		return false
	}

	// Look for scenarios where detonation would be beneficial
	for _, torp := range s.gameState.Torps {
		if torp.Owner != p.ID || torp.Status != 1 {
			continue
		}

		// Check for enemies near our torpedoes
		for _, enemy := range s.gameState.Players {
			if enemy.Status != game.StatusAlive || enemy.Team == p.Team {
				continue
			}

			dist := game.Distance(torp.X, torp.Y, enemy.X, enemy.Y)

			// Detonate if enemy is in blast radius but torpedo won't hit directly
			if dist < 2500 && dist > 800 {
				// Check if torpedo is not heading directly at enemy
				dx := enemy.X - torp.X
				dy := enemy.Y - torp.Y
				angleToEnemy := math.Atan2(dy, dx)
				angleDiff := math.Abs(angleToEnemy - torp.Dir)
				if angleDiff > math.Pi {
					angleDiff = 2*math.Pi - angleDiff
				}

				// Detonate if torpedo is passing by the enemy
				if angleDiff > math.Pi/6 {
					return true
				}
			}

			// Detonate if multiple enemies are clustered
			if dist < 3000 {
				nearbyCount := 0
				for _, other := range s.gameState.Players {
					if other.Status == game.StatusAlive && other.Team == enemy.Team && other.ID != enemy.ID {
						if game.Distance(torp.X, torp.Y, other.X, other.Y) < 3500 {
							nearbyCount++
						}
					}
				}
				if nearbyCount >= 1 {
					return true // Detonate for area damage on clustered enemies
				}
			}
		}
	}

	return false
}
