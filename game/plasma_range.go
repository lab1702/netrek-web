package game

// MaxPlasmaRange calculates the maximum distance a plasma torpedo can travel
// before its fuse expires, given the fuse time (in ticks) and speed (units per tick)
func MaxPlasmaRange(fuseTicks int, speedUnitsPerTick float64) float64 {
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
	return MaxPlasmaRange(stats.PlasmaFuse, speedUnitsPerTick)
}

// GetPlasmaRangeTable returns a map of ship types to their maximum plasma ranges
// for debugging and documentation purposes
func GetPlasmaRangeTable() map[ShipType]float64 {
	ranges := make(map[ShipType]float64)
	for shipType := range ShipData {
		ranges[shipType] = MaxPlasmaRangeForShip(shipType)
	}
	return ranges
}

// EffectivePlasmaRange returns a conservative estimate of plasma range
// accounting for target movement and tactical considerations
func EffectivePlasmaRange(ship ShipType, safetyFactor float64) float64 {
	maxRange := MaxPlasmaRangeForShip(ship)
	if maxRange == 0.0 {
		return 0.0
	}

	// Apply safety factor (typically 0.9 or 0.95 to ensure hit before fuse expiry)
	return maxRange * safetyFactor
}
