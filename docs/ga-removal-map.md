# Galaxy (GA) Ship Class Removal Reference Map

This document tracks all references to the Galaxy ship class that need to be removed or updated.

## Core Type Definitions
- `game/types.go:120` - ShipGalaxy constant declaration
- `game/types.go:354-386` - ShipGalaxy stats in ShipData map

## Client-Side References
- `static/game.html:577-578` - Ship selection UI for GA
- `static/netrek.js:199` - shipNames array includes 'GA'
- `static/netrek.js:2439` - getMaxSpeed array includes GA speed
- `static/netrek.js:2444` - getMaxArmies array includes GA armies
- `static/ship-bitmaps-all-teams.js:14` - GALAXY constant
- `static/ship-bitmaps-all-teams.js:25` - shipDimensions[GALAXY]
- `static/ship-bitmaps-all-teams.js:824-925` - fed_galaxy_bits arrays
- `static/ship-bitmaps-all-teams.js:1750+` - rom_galaxy_bits arrays
- `static/ship-bitmaps-all-teams.js:2676+` - kli_galaxy_bits arrays
- `static/ship-bitmaps-all-teams.js:3602+` - ori_galaxy_bits arrays
- `static/ship-bitmaps-all-teams.js:4528+` - ind_galaxy_bits arrays
- `static/ship-bitmaps-all-teams.js:4669,4678,4687,4696,4705` - Team bitmap mappings
- `static/ship-bitmaps-all-teams.js:4775` - SHIP_TYPES export

## Server-Side Business Logic
- `server/bot_combat.go:87` - Bot combat logic
- `server/bot_handlers.go:47,116,260,274,325` - Bot ship selection
- `server/bot_navigation.go:multiple` - Navigation logic
- `server/bot_planet.go:146` - Planet interaction
- `server/physics.go:70,121,122,132,133` - Physics calculations
- `server/systems.go:58,79` - Ship systems
- `server/websocket.go:273` - WebSocket validation
- `server/bots.go:617` - Bot creation

## Test Files
- `game/torp_test.go:39,40,147,194,223` - Torpedo tests
- `server/handlers_test.go:392` - Handler tests
- `server/weapon_direction_test.go:24` - Weapon tests

## Validation & Constants
- All occurrences of `value="6"` for ship type 6
- All array/slice indices expecting 7 ship types (now 6)
- Maximum ship type validations

## Ship Type Index Changes
After removal, ship types will be:
- 0: Scout (SC)
- 1: Destroyer (DD) 
- 2: Cruiser (CA)
- 3: Battleship (BB)
- 4: Assault (AS)
- 5: Starbase (SB)
- ~~6: Galaxy (GA)~~ - REMOVED

All arrays, loops, and validations expecting 7 elements need to be updated to 6.