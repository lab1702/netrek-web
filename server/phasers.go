package server

import (
	"log"
	"math"

	"github.com/lab1702/netrek-web/game"
)

// resolvePhaser applies the nearest ship/plasma collision, damage and visuals
// for human and bot shots. The caller holds gameState.Mu and pays firing costs.
func (s *Server) resolvePhaser(p *game.Player, course float64) {
	shipStats := game.ShipData[p.Ship]
	myPhaserRange := game.PhaserRange(shipStats)
	// Find the nearest enemy ship on the phaser line; keep rangeSq so the
	// plasma scan below only considers plasmas closer than the hit ship.
	target, targetDist, rangeSq := s.phaserTargetInLine(p, course, myPhaserRange)

	// (C, D) is a point on the phaser line, relative to me
	// Using 10*PHASEDIST like original to prevent round-off errors
	C := math.Cos(course) * 10 * float64(game.PhaserDist)
	D := math.Sin(course) * 10 * float64(game.PhaserDist)

	var hitPlasma *game.Plasma
	// Check plasma torpedoes (if they exist)
	for _, plasma := range s.gameState.Plasmas {
		if plasma == nil || plasma.Status != game.TorpMove || plasma.OwnedBy(p) {
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

		projection := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))
		if projection < 0 {
			continue
		}

		E := C * projection
		F := D * projection
		dx := E - A
		dy := F - B

		// Use ZAPPLASMADIST for plasma hit detection
		if dx*dx+dy*dy <= float64(game.ZAPPLASMADIST*game.ZAPPLASMADIST) {
			hitPlasma = plasma
			rangeSq = thisRangeSq
		}
	}
	if hitPlasma != nil {
		hitPlasma.Status = game.TorpDet
		s.tryBroadcast(ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from": p.ID, "to": -2,
				"x": hitPlasma.X, "y": hitPlasma.Y, "range": myPhaserRange,
			},
		})
		return
	}

	// Fire at target if found
	if target != nil {
		// Calculate damage based on distance using original formula
		damage := float64(shipStats.PhaserDamage) * (1.0 - targetDist/myPhaserRange)
		log.Printf("Phaser hit: player %d hit player %d for %.1f damage at range %.0f", p.ID, target.ID, damage, targetDist)

		// Apply damage to shields first, then hull (round instead of truncate)
		actualDamage := game.ApplyDamageWithShields(target, int(math.Round(damage)))

		if target.Damage >= game.ShipData[target.Ship].MaxDamage {
			s.killPlayer(target, p.ID, game.KillPhaser, actualDamage)
		} else if s.gameState.T_mode {
			// Non-lethal hit: still track damage for tournament stats, matching
			// the torpedo and plasma hit paths.
			if stats, ok := s.gameState.TournamentStats[p.ID]; ok {
				stats.DamageDealt += actualDamage
			}
			if stats, ok := s.gameState.TournamentStats[target.ID]; ok {
				stats.DamageTaken += actualDamage
			}
		}

		// Send phaser visual to all players (non-blocking).
		// Use "target" (not "to") so the broadcast router does not treat this
		// as a private message routed only to the player that was hit.
		s.tryBroadcast(ServerMessage{
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
		s.tryBroadcast(ServerMessage{
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
