package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
	"github.com/lab1702/netrek-web/server/aimcalc"
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

	// Use unified intercept solver
	shooterPos := aimcalc.Point2D{X: p.X, Y: p.Y}
	targetPos := aimcalc.Point2D{X: target.X, Y: target.Y}
	targetVel := aimcalc.Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20, // Convert to units/tick
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick

	// Calculate intercept direction
	fireDir, _ := aimcalc.InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)
	
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

	// Use unified intercept solver for plasma
	shooterPos := aimcalc.Point2D{X: p.X, Y: p.Y}
	targetPos := aimcalc.Point2D{X: target.X, Y: target.Y}
	targetVel := aimcalc.Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20, // Convert to units/tick
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}
	projSpeed := float64(shipStats.PlasmaSpeed * 20) // Convert to units/tick
	fireDir, _ := aimcalc.InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)

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

	// Use unified intercept solver
	shooterPos := aimcalc.Point2D{X: p.X, Y: p.Y}
	targetPos := aimcalc.Point2D{X: target.X, Y: target.Y}
	targetVel := aimcalc.Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20, // Convert to units/tick
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick

	// Calculate intercept direction
	fireDir, _ := aimcalc.InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)
	
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
	shooterPos := aimcalc.Point2D{X: p.X, Y: p.Y}
	targetPos := aimcalc.Point2D{X: target.X, Y: target.Y}
	targetVel := aimcalc.Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20, // Convert to units/tick
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick
	baseDir, _ := aimcalc.InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)
	
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
	shooterPos := aimcalc.Point2D{X: p.X, Y: p.Y}
	targetPos := aimcalc.Point2D{X: target.X, Y: target.Y}
	targetVel := aimcalc.Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20, // Convert to units/tick
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}
	projSpeed := float64(shipStats.TorpSpeed * 20) // Convert to units/tick
	fireDir, _ := aimcalc.InterceptDirectionSimple(shooterPos, targetPos, targetVel, projSpeed)
	
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

	// Unconditional close-range torpedo fallback - fires even when other conditions prevent it
	if enemyDist < game.StarbaseTorpRange && p.NumTorps < game.MaxTorps && p.Fuel > 2000 && p.WTemp < 800 {
		s.fireBotTorpedoWithLead(p, enemy)
		p.BotCooldown = 10
		return
	}
}
