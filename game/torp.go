package game

// Torpedo physics constants
const (
	// TorpUnitFactor is the multiplier that converts ship TorpSpeed to units per tick
	// This matches the formula used in server/handlers.go: shipStats.TorpSpeed * 20
	TorpUnitFactor = 20

	// DefaultTorpSafety is the default safety margin for effective torpedo range
	// This ensures torpedoes reach targets before fuse expires, accounting for
	// target movement and aiming errors
	DefaultTorpSafety = 0.85
)

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
