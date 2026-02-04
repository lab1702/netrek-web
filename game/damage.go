package game

// ApplyDamageWithShields applies damage to shields first, then hull.
// Returns the total amount of damage actually applied.
// This ensures consistent damage handling across all weapon types.
func ApplyDamageWithShields(p *Player, damage int) int {
	if p == nil || damage <= 0 {
		return 0
	}

	totalApplied := 0

	// Apply damage to shields first if they're up and have capacity
	if p.Shields_up && p.Shields > 0 {
		shieldDamage := Min(damage, p.Shields)
		p.Shields -= shieldDamage
		damage -= shieldDamage
		totalApplied += shieldDamage
	}

	// Apply remaining damage to hull
	if damage > 0 {
		p.Damage += damage
		totalApplied += damage
	}

	return totalApplied
}
