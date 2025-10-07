package server

import "log"

// Debug flags for various subsystems
var (
	DebugWeapons = false // Set to true to enable detailed weapon firing logs
)

// logWeaponDecision logs weapon firing decisions when debugging is enabled
func logWeaponDecision(weaponType, decision, reason string, shipType int, dist, maxRange float64) {
	if DebugWeapons {
		log.Printf("[WEAPON DEBUG] %s: %s - Ship:%d Dist:%.0f MaxRange:%.0f Reason:%s",
			weaponType, decision, shipType, dist, maxRange, reason)
	}
}

// logPlasmaFiring logs plasma firing attempts and decisions
func logPlasmaFiring(decision string, shipType int, dist, maxRange float64, reason string) {
	logWeaponDecision("PLASMA", decision, reason, shipType, dist, maxRange)
}
