package game

// Torpedo physics constants
const (
	// TorpUnitFactor is the multiplier that converts ship TorpSpeed to units per tick
	// This matches the formula used in server/handlers.go: shipStats.TorpSpeed * 20
	TorpUnitFactor = 20

	// DefaultTorpSafety is the default safety margin for effective torpedo range
	// This ensures torpedoes reach targets before fuse expires, accounting for
	// target movement and aiming errors.
	DefaultTorpSafety = 0.85
)

// ShipSafetyFactor defines per-ship safety margins for torpedo firing range
// These more conservative factors help prevent torpedoes from expiring before
// reaching moving targets, especially for slower torpedo ships
var ShipSafetyFactor = map[ShipType]float64{
	ShipScout:      0.65, // Fast torps but short fuse - be conservative
	ShipDestroyer:  0.70, // Balanced torp boat
	ShipCruiser:    0.75, // Slower torps but longer fuse
	ShipBattleship: 0.75, // Slower torps but longer fuse
	ShipAssault:    0.65, // Fast torps but need to close for armies
	ShipStarbase:   0.80, // Stationary platform, can afford longer shots
}

// MaxTorpRange returns the absolute maximum distance a torpedo can fly
// before its fuse expires (no safety margin).
// Formula: (TorpSpeed * TorpUnitFactor) * TorpFuse
func MaxTorpRange(shipStats ShipStats) int {
	speedUnitsPerTick := shipStats.TorpSpeed * TorpUnitFactor
	maxRange := speedUnitsPerTick * shipStats.TorpFuse
	return maxRange
}

// EffectiveTorpRange returns MaxTorpRange multiplied by a safety margin
// to make bots fire only when a hit is reasonably possible.
// The safety margin accounts for target movement and aiming imperfection.
func EffectiveTorpRange(shipStats ShipStats, safetyMargin float64) int {
	maxRange := MaxTorpRange(shipStats)
	effectiveRange := float64(maxRange) * safetyMargin
	return int(effectiveRange)
}

// EffectiveTorpRangeDefault returns the effective torpedo range using the default safety margin.
func EffectiveTorpRangeDefault(shipStats ShipStats) int {
	return EffectiveTorpRange(shipStats, DefaultTorpSafety)
}

// EffectiveTorpRangeForShip returns the effective torpedo range using ship-specific safety margins.
// This provides more conservative ranges for bots to prevent torpedo fuse expiry.
func EffectiveTorpRangeForShip(shipType ShipType, shipStats ShipStats) int {
	safetyMargin, exists := ShipSafetyFactor[shipType]
	if !exists {
		safetyMargin = DefaultTorpSafety
	}
	return EffectiveTorpRange(shipStats, safetyMargin)
}
