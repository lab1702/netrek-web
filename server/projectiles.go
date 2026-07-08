package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// updateProjectiles handles all torpedo and plasma movement and collision detection
func (s *Server) updateProjectiles() {
	// Index all alive players into the spatial grid for efficient collision detection
	// (if grid is nil, collision detection falls back to O(n*m) iteration)
	if s.playerGrid != nil {
		s.playerGrid.IndexPlayers(s.gameState.Players[:])
	}

	s.updateTorpedoes()
	s.updatePlasmas()
}

// updateTorpedoes handles torpedo movement, collision detection, and cleanup
func (s *Server) updateTorpedoes() {
	s.gameState.Torps = s.updateProjectileList(s.gameState.Torps, game.ExplosionDist, game.KillTorp,
		func(owner *game.Player) {
			if owner.NumTorps > 0 {
				owner.NumTorps--
			}
		})
}

// updatePlasmas handles plasma movement, collision detection, and cleanup
func (s *Server) updatePlasmas() {
	s.gameState.Plasmas = s.updateProjectileList(s.gameState.Plasmas, game.PlasmaExplosionDist, game.KillPlasma,
		func(owner *game.Player) {
			if owner.NumPlasma > 0 {
				owner.NumPlasma--
			}
		})
}

// updateProjectileList moves projectiles, expires fuses, and detects hits,
// filtering the list in place to avoid slice allocation every frame.
// Torpedoes and plasmas share the same struct and lifecycle; they differ only
// in explosion distance, kill reason, and which per-player counter to
// decrement (decCount floors at 0 to handle post-death expiry).
func (s *Server) updateProjectileList(list []*game.Torpedo, explDist float64, killType int, decCount func(*game.Player)) []*game.Torpedo {
	writeIdx := 0
	for _, t := range list {
		decOwner := func() {
			if t.Owner >= 0 && t.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[t.Owner]; owner != nil {
					decCount(owner)
				}
			}
		}

		// If projectile is already exploding, remove it this frame
		if t.Status == game.TorpDet {
			decOwner()
			continue
		}

		// Move projectile before decrementing fuse so projectiles travel
		// the full number of ticks their fuse allows (fixes off-by-one
		// where fuse was decremented before movement, causing projectiles
		// to travel one tick short of their configured range).
		t.X += t.Speed * math.Cos(t.Dir)
		t.Y += t.Speed * math.Sin(t.Dir)

		// Decrement fuse every tick (now running at 10 ticks/sec)
		t.Fuse--
		if t.Fuse <= 0 {
			// Projectile expired
			decOwner()
			continue
		}

		// Check if projectile went out of bounds - remove it
		if t.X < 0 || t.X > game.GalaxyWidth || t.Y < 0 || t.Y > game.GalaxyHeight {
			decOwner()
			continue
		}

		// Check for hits using spatial grid for O(1) average lookup (falls back to O(n) if grid unavailable)
		var nearbyPlayers []int
		if s.playerGrid != nil {
			nearbyPlayers = s.playerGrid.GetNearby(t.X, t.Y)
		} else {
			// Fallback: check all players (O(n) per projectile)
			for i := 0; i < game.MaxPlayers; i++ {
				if s.gameState.Players[i].Status == game.StatusAlive {
					nearbyPlayers = append(nearbyPlayers, i)
				}
			}
		}
		for _, i := range nearbyPlayers {
			p := s.gameState.Players[i]
			// Skip if not alive or self-damage
			if p.Status != game.StatusAlive || p.ID == t.Owner {
				continue
			}
			// Prevent friendly fire - check if target is on same team as projectile owner
			if t.Owner >= 0 && t.Owner < game.MaxPlayers {
				owner := s.gameState.Players[t.Owner]
				if owner != nil && p.Team == owner.Team {
					continue
				}
			}

			if game.Distance(t.X, t.Y, p.X, p.Y) <= explDist {
				// Hit! Mark as exploding - it will be removed next frame
				s.handleProjectileHit(t, p, killType)
				t.Status = game.TorpDet
				break
			}
		}

		// Keep projectile in list (even if exploding, so it shows for one frame)
		list[writeIdx] = t
		writeIdx++
	}
	return list[:writeIdx]
}

// handleProjectileHit processes a torpedo or plasma hit on a player
func (s *Server) handleProjectileHit(t *game.Torpedo, target *game.Player, killType int) {
	actualDamage := game.ApplyDamageWithShields(target, t.Damage)
	if target.Damage >= game.ShipData[target.Ship].MaxDamage {
		s.killPlayer(target, t.Owner, killType, actualDamage)
	} else if s.gameState.T_mode {
		// Non-lethal hit: still track damage for tournament stats
		if stats, ok := s.gameState.TournamentStats[t.Owner]; ok {
			stats.DamageDealt += actualDamage
		}
		if stats, ok := s.gameState.TournamentStats[target.ID]; ok {
			stats.DamageTaken += actualDamage
		}
	}
}
