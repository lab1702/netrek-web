package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// updateProjectiles handles all torpedo and plasma movement and collision detection
func (s *Server) updateProjectiles() {
	s.updateTorpedoes()
	s.updatePlasmas()
}

// updateTorpedoes handles torpedo movement, collision detection, and cleanup
func (s *Server) updateTorpedoes() {
	newTorps := make([]*game.Torpedo, 0)
	for _, torp := range s.gameState.Torps {
		// If torpedo is already exploding, remove it this frame
		if torp.Status == 3 {
			// Decrement owner's torpedo count
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Decrement fuse every tick (now running at 10 ticks/sec)
		torp.Fuse--
		if torp.Fuse <= 0 {
			// Torpedo exploded
			// Decrement owner's torpedo count
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Move torpedo
		torp.X += torp.Speed * math.Cos(torp.Dir)
		torp.Y += torp.Speed * math.Sin(torp.Dir)

		// Check if torpedo went out of bounds
		if torp.X < 0 || torp.X > game.GalaxyWidth || torp.Y < 0 || torp.Y > game.GalaxyHeight {
			// Torpedo hit galaxy edge - remove it
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Check for hits
		for i := 0; i < game.MaxPlayers; i++ {
			p := s.gameState.Players[i]
			// Skip if not alive, self-damage, or friendly fire
			if p.Status != game.StatusAlive || p.ID == torp.Owner {
				continue
			}
			// Prevent friendly fire - check if target is on same team as torpedo owner
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				owner := s.gameState.Players[torp.Owner]
				if owner != nil && p.Team == owner.Team {
					continue
				}
			}

			if game.Distance(torp.X, torp.Y, p.X, p.Y) < game.ExplosionDist {
				// Hit!
				s.handleTorpedoHit(torp, p, i)
				// Mark torpedo as exploding - it will be removed next frame
				torp.Status = 3
				break
			}
		}

		// Keep torpedo in list (even if exploding, so it shows for one frame)
		newTorps = append(newTorps, torp)
	}
	s.gameState.Torps = newTorps
}

// updatePlasmas handles plasma movement, collision detection, and cleanup
func (s *Server) updatePlasmas() {
	newPlasmas := make([]*game.Plasma, 0)
	for _, plasma := range s.gameState.Plasmas {
		// Decrement fuse every tick (now running at 10 ticks/sec)
		plasma.Fuse--
		if plasma.Fuse <= 0 {
			// Plasma dissipated
			// Decrement owner's plasma count
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				s.gameState.Players[plasma.Owner].NumPlasma--
			}
			continue
		}

		// Move plasma
		plasma.X += plasma.Speed * math.Cos(plasma.Dir)
		plasma.Y += plasma.Speed * math.Sin(plasma.Dir)

		// Check if plasma went out of bounds
		if plasma.X < 0 || plasma.X > game.GalaxyWidth || plasma.Y < 0 || plasma.Y > game.GalaxyHeight {
			// Plasma hit galaxy edge - remove it
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[plasma.Owner]; owner != nil {
					owner.NumPlasma--
				}
			}
			continue
		}

		// Check for hits
		hit := false
		explosionRadius := 1500.0 // Plasma has larger explosion radius
		for i := 0; i < game.MaxPlayers; i++ {
			p := s.gameState.Players[i]
			// Skip if not alive, self-damage, or friendly fire
			if p.Status != game.StatusAlive || p.ID == plasma.Owner {
				continue
			}
			// Prevent friendly fire - check if target is on same team as plasma owner
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				owner := s.gameState.Players[plasma.Owner]
				if owner != nil && p.Team == owner.Team {
					continue
				}
			}

			if game.Distance(plasma.X, plasma.Y, p.X, p.Y) < explosionRadius {
				// Hit!
				s.handlePlasmaHit(plasma, p, i)
				hit = true
				break
			}
		}

		if hit {
			// Decrement owner's plasma count
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				s.gameState.Players[plasma.Owner].NumPlasma--
			}
		} else {
			newPlasmas = append(newPlasmas, plasma)
		}
	}
	s.gameState.Plasmas = newPlasmas
}

// handleTorpedoHit processes a torpedo hit on a player
func (s *Server) handleTorpedoHit(torp *game.Torpedo, target *game.Player, targetIndex int) {
	// Apply damage to shields first, then hull
	game.ApplyDamageWithShields(target, torp.Damage)
	if target.Damage >= game.ShipData[target.Ship].MaxDamage {
		// Ship destroyed!
		target.Status = game.StatusExplode
		target.ExplodeTimer = 10 // 10 frames of explosion animation
		target.KilledBy = torp.Owner
		target.WhyDead = game.KillTorp
		target.Bombing = false // Stop bombing when destroyed
		target.Beaming = false // Stop beaming when destroyed
		target.Orbiting = -1   // Break orbit when destroyed
		// Clear lock-on when destroyed
		target.LockType = "none"
		target.LockTarget = -1
		target.Deaths++ // Increment death count
		s.gameState.Players[torp.Owner].Kills += 1
		s.gameState.Players[torp.Owner].KillsStreak += 1

		// Update tournament stats
		if s.gameState.T_mode {
			if stats, ok := s.gameState.TournamentStats[torp.Owner]; ok {
				stats.Kills++
				stats.DamageDealt += torp.Damage
			}
			if stats, ok := s.gameState.TournamentStats[targetIndex]; ok {
				stats.Deaths++
				stats.DamageTaken += torp.Damage
			}
		}

		// Send death message
		s.broadcastDeathMessage(target, s.gameState.Players[torp.Owner])
	}
}

// handlePlasmaHit processes a plasma hit on a player
func (s *Server) handlePlasmaHit(plasma *game.Plasma, target *game.Player, targetIndex int) {
	// Apply damage to shields first, then hull
	game.ApplyDamageWithShields(target, plasma.Damage)
	if target.Damage >= game.ShipData[target.Ship].MaxDamage {
		// Ship destroyed by plasma!
		target.Status = game.StatusExplode
		target.ExplodeTimer = 10
		target.KilledBy = plasma.Owner
		target.WhyDead = game.KillTorp // Using torp constant for now
		target.Bombing = false         // Stop bombing when destroyed
		target.Beaming = false         // Stop beaming when destroyed
		target.Orbiting = -1           // Break orbit when destroyed
		// Clear lock-on when destroyed
		target.LockType = "none"
		target.LockTarget = -1
		target.Deaths++ // Increment death count
		s.gameState.Players[plasma.Owner].Kills += 1
		s.gameState.Players[plasma.Owner].KillsStreak += 1

		// Update tournament stats
		if s.gameState.T_mode {
			if stats, ok := s.gameState.TournamentStats[plasma.Owner]; ok {
				stats.Kills++
				stats.DamageDealt += plasma.Damage
			}
			if stats, ok := s.gameState.TournamentStats[targetIndex]; ok {
				stats.Deaths++
				stats.DamageTaken += plasma.Damage
			}
		}

		// Send death message
		s.broadcastDeathMessage(target, s.gameState.Players[plasma.Owner])
	}
}
