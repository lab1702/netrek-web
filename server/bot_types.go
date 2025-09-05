package server

import (
	"github.com/lab1702/netrek-web/game"
)

// Bot AI constants
const (
	// OrbitDistance is the distance required to orbit a planet
	OrbitDistance = 2000.0

	// Planet defense constants
	PlanetDefenseDetectRadius    = 15000.0 // Range for bot to detect threats to friendly planets
	PlanetDefenseInterceptBuffer = 3000.0  // Additional range beyond bomb range to intercept threats
	PlanetBombRange              = 2000.0  // Range at which enemies can bomb planets effectively
)

// BotNames for generating random bot names
var BotNames = []string{
	"HAL-9000", "R2-D2", "C-3PO", "Data", "Bishop", "T-800",
	"Johnny-5", "WALL-E", "EVE", "Optimus", "Bender", "K-2SO",
	"BB-8", "IG-88", "HK-47", "GLaDOS", "SHODAN", "Cortana",
	"Friday", "Jarvis", "Vision", "Ultron", "Skynet", "Agent-Smith",
}

// CombatThreat tracks various combat threats
type CombatThreat struct {
	closestTorpDist float64
	closestPlasma   float64
	nearbyEnemies   int
	requiresEvasion bool
	threatLevel     int
}

// SeparationVector represents the direction and magnitude to separate from allies
type SeparationVector struct {
	x         float64
	y         float64
	magnitude float64
}

// CombatManeuver represents a tactical movement decision
type CombatManeuver struct {
	direction float64
	speed     float64
	maneuver  string
}

// PlanetDefenderInfo contains information about defenders around a planet
type PlanetDefenderInfo struct {
	Defenders         []*game.Player // All enemy players within detection radius
	DefenderCount     int            // Count of enemy ships
	ClosestDefender   *game.Player   // The closest enemy ship
	MinDefenderDist   float64        // Distance to the closest defender
	HasCarrierDefense bool           // Whether any defender is carrying armies
	DefenseScore      float64        // Calculated threat score (higher = more dangerous)
}
