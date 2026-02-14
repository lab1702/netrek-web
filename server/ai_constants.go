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
	ArmyCarryingRange    = 3500.0  // Shield range when carrying armies
	DefenseShieldRange   = 3000.0  // Shield range during planet defense
	PhaserRangeFactor    = 0.8     // Shield when within 80% of enemy phaser range
	RepairSafetyDistance = 12000.0 // Minimum enemy distance to drop shields for repair (must exceed phaser range + buffer)

	// Team Coordination Thresholds
	BroadcastTargetMinValue = 15000.0 // Minimum target score to broadcast to allies
	BroadcastTargetRange    = 15000.0 // Maximum distance to broadcast target suggestions

	// Ally Separation Thresholds
	// These control how bots maintain distance from teammates
	SepMinSafeDistance  = 4000.0 // Maximum range to consider allies for separation
	SepIdealDistance    = 2500.0 // Ideal spacing between bots
	SepCriticalDistance = 1200.0 // Emergency separation distance

	// Ally Separation Strength Multipliers
	SepCriticalStrength    = 5.0 // Repulsion strength at critical distance
	SepIdealStrength       = 2.0 // Repulsion strength within ideal distance
	SepModerateStrength    = 0.8 // Repulsion strength beyond ideal distance
	SepSameTargetMult      = 1.8 // Extra repulsion when targeting same enemy
	SepDamagedAllyHighMult = 2.0 // Multiplier for heavily damaged allies (>50%)
	SepDamagedAllyLowMult  = 1.5 // Multiplier for moderately damaged allies (>30%)
	SepClusterMult         = 1.3 // Multiplier when 2+ allies are nearby
	SepMagnitudeCap        = 3.0 // Maximum magnitude scale to prevent erratic scattering

	// Target Scoring Weights (used by calculateTargetScore)
	TargetDistanceFactor   = 20000.0 // Numerator for distance-based scoring (score = factor/dist)
	TargetCriticalDmgBonus = 8000.0  // Bonus for nearly dead targets (>80% damage)
	TargetHighDmgMult      = 5000.0  // Multiplier for heavily damaged targets (>50%)
	TargetLowDmgMult       = 3000.0  // Multiplier for lightly damaged targets
	TargetCarrierBonus     = 10000.0 // Base bonus for army carriers
	TargetCarrierPerArmy   = 1500.0  // Additional bonus per army carried
	TargetSpeedBonus       = 300.0   // Bonus per warp of speed advantage
	TargetCloakedPenalty   = 6000.0  // Penalty for cloaked targets beyond close range
	TargetDecloakBonus     = 2000.0  // Bonus for cloaked targets within close range
	TargetIsolatedBonus    = 2000.0  // Bonus for targets with no nearby allies
	TargetPersistenceBonus = 3000.0  // Bonus for keeping current target (prevents thrashing)
	TargetCloakDetectRange = 2000.0  // Range within which cloaked ships are worth attacking
	IsolationRange         = 5000.0  // Range to check for nearby allies when determining isolation

	// Planet Strategy Thresholds
	CorePlanetRadius = 25000.0 // Distance from team home to consider a planet "core"

	// Sentinel Values
	MaxSearchDistance = 999999.0  // Sentinel for "no target found" in nearest-object searches
	WorstScore        = -999999.0 // Sentinel for "no candidate scored" in best-candidate searches
)
