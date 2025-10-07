# Changelog

## [Unreleased]

### Fixed
- **Bot plasma torpedoes disappearing before hitting targets**: Fixed bot AI plasma torpedo firing logic that was using incorrect range calculations based on torpedo ranges instead of actual plasma maximum ranges. Bots were firing plasma at distances where the fuse would expire before reaching the target, causing them to disappear in flight. Now uses proper plasma fuse time × speed calculations to ensure plasma can reach targets before expiring.

### Added
- **Plasma range calculation utilities**: Added `MaxPlasmaRange()`, `MaxPlasmaRangeForShip()`, and `EffectivePlasmaRange()` functions with comprehensive unit tests
- **Weapon debugging system**: Added `DebugWeapons` flag and logging infrastructure to trace plasma firing decisions
- **Comprehensive bot plasma tests**: Added integration tests covering firing decisions for all ship types and distance scenarios

### Technical Details
- Destroyer plasma max range: 9,000 units (30 ticks × 300 units/tick)
- Cruiser/Battleship plasma max range: 10,500 units (35 ticks × 300 units/tick)  
- Starbase plasma max range: 7,500 units (25 ticks × 300 units/tick)
- Added pre-fire sanity checks to prevent firing beyond maximum range
- Replaced hard-coded range coefficients with actual physics calculations