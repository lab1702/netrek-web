package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

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
		Status: 1,                    // Moving
		Team:   p.Team,               // Set team color
	}

	s.gameState.Plasmas = append(s.gameState.Plasmas, plasma)
	p.NumPlasma++
	p.Fuel -= shipStats.PlasmaDamage * shipStats.PlasmaFuelMult
}

// fireBotTorpedoWithLead fires torpedo with advanced leading
func (s *Server) fireBotTorpedoWithLead(p, target *game.Player) {
	// Can't fire while cloaked or repairing (same rules as human players)
	if p.Cloaked || p.Repairing {
		return
	}

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
	vs := math.Hypot(vxs, vys)

	// Solve intercept equation
	var t float64
	if vs > 0 {
		// Quadratic solution for intercept time
		a := vs*vs - torpSpeed*torpSpeed
		b := 2 * l * dp
		c := l * l

		if a == 0 {
			// Linear case
			if b != 0 {
				t = -c / b
			}
		} else {
			// Quadratic case
			discriminant := b*b - 4*a*c
			if discriminant >= 0 {
				t1 := (-b + math.Sqrt(discriminant)) / (2 * a)
				t2 := (-b - math.Sqrt(discriminant)) / (2 * a)
				// Choose positive, smaller time
				if t1 > 0 && t2 > 0 {
					t = math.Min(t1, t2)
				} else if t1 > 0 {
					t = t1
				} else if t2 > 0 {
					t = t2
				}
			}
		}
	}

	// Calculate firing direction
	var fireDir float64
	if t > 0 {
		// Fire at intercept point
		interceptX := target.X + vxs*t/20
		interceptY := target.Y + vys*t/20
		fireDir = math.Atan2(interceptY-p.Y, interceptX-p.X)
	} else {
		// Direct shot if no intercept solution
		fireDir = math.Atan2(target.Y-p.Y, target.X-p.X)
	}

	// Add small random jitter
	fireDir += randomJitterRad()

	// Create torpedo
	torp := &game.Torpedo{
		ID:     len(s.gameState.Torps),
		Owner:  p.ID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireDir,
		Speed:  torpSpeed,
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
	baseDir := s.calculateEnhancedInterceptCourse(p, target)
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

	fireDir := s.calculateEnhancedInterceptCourse(p, target)
	// Add small random jitter to make bot torpedoes harder to dodge
	fireDir += randomJitterRad()
	shipStats := game.ShipData[p.Ship]

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
	effectiveTorpRange := float64(game.EffectiveTorpRangeDefault(shipStats))
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

	// Enhanced plasma usage for ships that have it
	if shipStats.HasPlasma && p.NumPlasma < 1 && enemyDist < 7000 && enemyDist > 2000 && p.Fuel > 3000 {
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
	// Use actual torpedo physics range instead of hardcoded constant
	effectiveTorpRange := float64(game.EffectiveTorpRangeDefault(shipStats))
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
}
