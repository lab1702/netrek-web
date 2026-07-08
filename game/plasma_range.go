package game

// maxPlasmaRange calculates the maximum distance a plasma torpedo can travel
// before its fuse expires, given the fuse time (in ticks) and speed (units per tick)
func maxPlasmaRange(fuseTicks int, speedUnitsPerTick float64) float64 {
	if fuseTicks < 0 || speedUnitsPerTick < 0 {
		return 0.0
	}
	return float64(fuseTicks) * speedUnitsPerTick
}

// MaxPlasmaRangeForShip calculates the maximum plasma range for a specific ship type
func MaxPlasmaRangeForShip(ship ShipType) float64 {
	stats := ShipData[ship]
	if !stats.HasPlasma {
		return 0.0
	}

	// Speed is converted to units per tick: PlasmaSpeed * 20
	speedUnitsPerTick := float64(stats.PlasmaSpeed * 20)
	return maxPlasmaRange(stats.PlasmaFuse, speedUnitsPerTick)
}

// EffectivePlasmaRange returns a conservative estimate of plasma range
// accounting for target movement and tactical considerations
func EffectivePlasmaRange(ship ShipType, safetyFactor float64) float64 {
	maxRange := MaxPlasmaRangeForShip(ship)
	if maxRange == 0.0 {
		return 0.0
	}

	// Clamp safety factor to valid range
	if safetyFactor < 0 {
		safetyFactor = 0
	} else if safetyFactor > 1.0 {
		safetyFactor = 1.0
	}

	// Apply safety factor (typically 0.9 or 0.95 to ensure hit before fuse expiry)
	return maxRange * safetyFactor
}
