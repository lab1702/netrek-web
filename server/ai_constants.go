package server

// AI Constants for Bot Behavior
// These constants control various aspects of bot AI behavior, particularly
// shield management and threat assessment. Centralizing these values makes
// the AI more maintainable and allows for easier tuning and testing.

const (
	// Shield Fuel Thresholds
	// These control when bots will activate shields based on available fuel
	FuelCritical = 400  // Emergency threshold - only shield for immediate threats
	FuelLow      = 600  // Low fuel - shield for close threats
	FuelModerate = 1400 // Moderate fuel - shield for medium-range threats
	FuelGood     = 2000 // Good fuel reserves - shield more liberally

	// Threat Assessment Constants
	ThreatLevelImmediate = 6 // Threat level that triggers immediate shielding
	ThreatLevelHigh      = 4 // High threat level
	ThreatLevelMedium    = 3 // Medium threat level

	// Distance Thresholds for Threat Detection
	TorpedoVeryClose = 2000.0 // Torpedoes within this range are always dangerous
	TorpedoClose     = 3000.0 // Range for torpedo threat assessment
	EnemyVeryClose   = 1800.0 // Enemies within this range are immediate threats
	EnemyClose       = 2500.0 // Range for enemy threat assessment
	PlasmaClose      = 2000.0 // Range for plasma threat assessment
	PlasmaFar        = 4000.0 // Extended range for plasma detection

	// Shield Decision Weights
	// These control how threat levels translate to shield decisions
	ImmediateThreatBonus = 6 // Additional threat level for immediate threats
	TorpedoThreatBonus   = 4 // Bonus threat level for threatening torpedoes
	CloseEnemyBonus      = 3 // Bonus for very close enemies
	PlasmaThreatBonus    = 5 // Bonus for plasma threats

	// Special Situation Thresholds
	ArmyCarryingRange  = 3500.0 // Shield range when carrying armies
	DefenseShieldRange = 3000.0 // Shield range during planet defense
	PhaserRangeFactor  = 0.8    // Shield when within 80% of enemy phaser range

	// Sentinel Values
	MaxSearchDistance = 999999.0  // Sentinel for "no target found" in nearest-object searches
	WorstScore       = -999999.0 // Sentinel for "no candidate scored" in best-candidate searches
)
