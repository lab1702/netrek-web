package game

import (
	"math/rand"
)

// InitPlanets initializes the 40 planets with their positions and teams
// Data exactly matches the original Netrek game
func InitPlanets(gs *GameState) {
	planetData := []struct {
		name  string
		label string
		x, y  float64
		team  int
		flags int
	}{
		// Federation planets (0-9)
		{"Earth", "EAR", 20000, 80000, TeamFed, PlanetHome | PlanetCore | PlanetRepair | PlanetFuel | PlanetAgri},
		{"Rigel", "RIG", 10000, 60000, TeamFed, 0},
		{"Canopus", "CAN", 25000, 60000, TeamFed, 0},
		{"Beta Crucis", "BET", 44000, 81000, TeamFed, 0},
		{"Organia", "ORG", 39000, 55000, TeamFed, 0},
		{"Deneb", "DEN", 30000, 90000, TeamFed, PlanetCore},
		{"Ceti Alpha V", "CET", 45000, 66000, TeamFed, 0},
		{"Altair", "ALT", 11000, 75000, TeamFed, PlanetCore},
		{"Vega", "VEG", 8000, 93000, TeamFed, PlanetCore},
		{"Alpha Centauri", "ALP", 32000, 74000, TeamFed, PlanetCore},

		// Romulan planets (10-19)
		{"Romulus", "ROM", 20000, 20000, TeamRom, PlanetHome | PlanetCore | PlanetRepair | PlanetFuel | PlanetAgri},
		{"Eridani", "ERI", 45000, 7000, TeamRom, 0},
		{"Aldeberan", "ALD", 4000, 12000, TeamRom, PlanetCore},
		{"Regulus", "REG", 42000, 44000, TeamRom, 0},
		{"Capella", "CAP", 13000, 45000, TeamRom, 0},
		{"Tauri", "TAU", 28000, 8000, TeamRom, PlanetCore},
		{"Draconis", "DRA", 28000, 23000, TeamRom, PlanetCore},
		{"Sirius", "SIR", 40000, 25000, TeamRom, 0},
		{"Indi", "IND", 25000, 44000, TeamRom, 0},
		{"Hydrae", "HYD", 8000, 29000, TeamRom, PlanetCore},

		// Klingon planets (20-29)
		{"Klingus", "KLI", 80000, 20000, TeamKli, PlanetHome | PlanetCore | PlanetRepair | PlanetFuel | PlanetAgri},
		{"Pliedes V", "PLI", 70000, 40000, TeamKli, 0},
		{"Andromeda", "AND", 60000, 10000, TeamKli, 0},
		{"Lalande", "LAL", 56400, 38200, TeamKli, 0},
		{"Praxis", "PRA", 91120, 9320, TeamKli, PlanetCore},
		{"Lyrae", "LYR", 89960, 31760, TeamKli, PlanetCore},
		{"Scorpii", "SCO", 70720, 26320, TeamKli, PlanetCore},
		{"Mira", "MIR", 83600, 45400, TeamKli, 0},
		{"Cygni", "CYG", 54600, 22600, TeamKli, 0},
		{"Achernar", "ACH", 73080, 6640, TeamKli, PlanetCore},

		// Orion planets (30-39)
		{"Orion", "ORI", 80000, 80000, TeamOri, PlanetHome | PlanetCore | PlanetRepair | PlanetFuel | PlanetAgri},
		{"Cassiopeia", "CAS", 91200, 56600, TeamOri, 0},
		{"El Nath", "EL", 70800, 54200, TeamOri, 0},
		{"Spica", "SPI", 57400, 62600, TeamOri, 0},
		{"Procyon", "PRO", 72720, 70880, TeamOri, PlanetCore},
		{"Polaris", "POL", 61400, 77000, TeamOri, 0},
		{"Arcturus", "ARC", 55600, 89000, TeamOri, 0},
		{"Ursae Majoris", "URS", 91000, 94000, TeamOri, PlanetCore},
		{"Herculis", "HER", 70000, 93000, TeamOri, PlanetCore},
		{"Antares", "ANT", 86920, 68920, TeamOri, PlanetCore},
	}

	for i := 0; i < len(planetData) && i < MaxPlanets; i++ {
		gs.Planets[i] = &Planet{
			ID:     i,
			Name:   planetData[i].name,
			Label:  planetData[i].label,
			X:      planetData[i].x,
			Y:      planetData[i].y,
			Owner:  planetData[i].team,
			Armies: 17, // Default starting armies
			Flags:  planetData[i].flags,
			Info:   0xF, // All teams have info at start
		}
	}
}

// InitINLPlanetFlags sets up planet flags for INL (International Netrek League) mode
// This distributes AGRI, FUEL, and REPAIR flags strategically across the galaxy
func InitINLPlanetFlags(gs *GameState) {
	// Define core planets (4 planets near each homeworld)
	corePlanets := [4][4]int{
		{7, 9, 5, 8},     // Fed core: Altair, Alpha Centauri, Deneb, Vega
		{12, 19, 15, 16}, // Rom core: Aldeberan, Hydrae, Tauri, Draconis
		{24, 29, 25, 26}, // Kli core: Praxis, Achernar, Lyrae, Scorpii
		{34, 39, 38, 37}, // Ori core: Procyon, Antares, Herculis, Ursae Majoris
	}

	// Define front planets (5 planets on the front lines)
	frontPlanets := [4][5]int{
		{1, 2, 4, 6, 3},      // Fed front: Rigel, Canopus, Organia, Ceti Alpha V, Beta Crucis
		{14, 18, 13, 17, 11}, // Rom front: Capella, Indi, Regulus, Sirius, Eridani
		{22, 28, 23, 21, 27}, // Kli front: Andromeda, Cygni, Lalande, Pliedes V, Mira
		{31, 32, 33, 35, 36}, // Ori front: Cassiopeia, El Nath, Spica, Polaris, Arcturus
	}

	// Clear all special flags except for homeworlds
	for i := 0; i < MaxPlanets; i++ {
		if gs.Planets[i] != nil {
			// Keep home planet flags intact
			if (gs.Planets[i].Flags & PlanetHome) == 0 {
				gs.Planets[i].Flags &= ^(PlanetRepair | PlanetFuel | PlanetAgri)
			}
		}
	}

	// Distribute flags for each team
	for team := 0; team < 4; team++ {
		// Place one AGRI in core planets
		coreAgri := rand.Intn(4)
		gs.Planets[corePlanets[team][coreAgri]].Flags |= PlanetAgri

		// Place one AGRI in front planets
		which := rand.Intn(2)
		var frontAgri int

		if which == 1 {
			// Place AGRI in first two front planets
			frontAgri = rand.Intn(2)
			gs.Planets[frontPlanets[team][frontAgri]].Flags |= PlanetAgri

			// Give FUEL to planet next to AGRI
			gs.Planets[frontPlanets[team][1-frontAgri]].Flags |= PlanetFuel

			// Place one REPAIR on the other front (planets 2-4)
			repairIdx := 2 + rand.Intn(3)
			gs.Planets[frontPlanets[team][repairIdx]].Flags |= PlanetRepair

			// Place 2 more FUEL on the other front
			fuelCount := 0
			for fuelCount < 2 {
				idx := 2 + rand.Intn(3)
				if (gs.Planets[frontPlanets[team][idx]].Flags & PlanetFuel) == 0 {
					gs.Planets[frontPlanets[team][idx]].Flags |= PlanetFuel
					fuelCount++
				}
			}
		} else {
			// Place AGRI in last two front planets
			frontAgri = 3 + rand.Intn(2)
			gs.Planets[frontPlanets[team][frontAgri]].Flags |= PlanetAgri

			// Give FUEL to planet next to AGRI
			otherIdx := 3
			if frontAgri == 3 {
				otherIdx = 4
			}
			gs.Planets[frontPlanets[team][otherIdx]].Flags |= PlanetFuel

			// Place one REPAIR on the other front (planets 0-2)
			repairIdx := rand.Intn(3)
			gs.Planets[frontPlanets[team][repairIdx]].Flags |= PlanetRepair

			// Place 2 more FUEL on the other front
			fuelCount := 0
			for fuelCount < 2 {
				idx := rand.Intn(3)
				if (gs.Planets[frontPlanets[team][idx]].Flags & PlanetFuel) == 0 {
					gs.Planets[frontPlanets[team][idx]].Flags |= PlanetFuel
					fuelCount++
				}
			}
		}

		// Place one more REPAIR in the core
		// (home + 1 front + 1 core = 3 Repair total)
		coreRepair := rand.Intn(4)
		gs.Planets[corePlanets[team][coreRepair]].Flags |= PlanetRepair

		// Place 2 more FUEL in the core
		// (home + 3 front + 2 core = 6 Fuel total, but home already has 1, so 5 more needed)
		coreFuelCount := 0
		for coreFuelCount < 2 {
			idx := rand.Intn(4)
			if (gs.Planets[corePlanets[team][idx]].Flags & PlanetFuel) == 0 {
				gs.Planets[corePlanets[team][idx]].Flags |= PlanetFuel
				coreFuelCount++
			}
		}
	}
}
