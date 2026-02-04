# NETREK-WEB

A browser-based version of the classic Netrek game. Play instantly in your web browser - no installation required!

## Quick Start

### For Players

1. Visit the server in your browser: `http://server-address:8080`
2. Enter your name and choose a team
3. Click "Enter Game" to start playing

### For Server Owners

```bash
# Install the server
go install github.com/lab1702/netrek-web@latest
```

```bash
# Run the server
netrek-web
```

```bash
# Or run on custom port
netrek-web -port 3000
```

Server is now running at `http://localhost:8080`

## Game Controls

### Mouse
- **Left Click**: Fire torpedo
- **Middle Click**: Fire phaser  
- **Right Click**: Set course

### Keyboard
- **0-9**: Set speed
- **S**: Shields
- **O**: Orbit planet
- **R**: Repair
- **L**: Lock on target
- **C**: Cloak
- **T/Y**: Tractor/Pressor beam
- **P**: Plasma torpedo
- **D**: Detonate torpedoes
- **B**: Bomb planet
- **Z/X**: Beam armies up/down
- **A**: Message all
- **Shift+T**: Team message
- **?**: Help window
- **\\**: Practice mode (add bots)

## Advanced Setup

### Build from Source

Prerequisites: Go 1.25+

```bash
git clone https://github.com/lab1702/netrek-web.git
cd netrek-web
go build
./netrek-web
```

### Docker

```bash
docker build -t netrek-web .
docker run -d -p 8080:8080 netrek-web
```

## Game Features

- 4 teams with 40 planets
- 6 ship types (Scout, Destroyer, Cruiser, Battleship, Assault, Starbase)
- Real-time space combat
- Tournament mode (4v4+)
- Practice mode with bots
- Team messaging system

## Development

### Code Organization

The project is organized into clear modules for maintainability and scalability:

#### Server Architecture (`server/`)
- **Handler Modules**: Focused, modular message handlers
  - `game_state_handlers.go` - Login and quit handlers  
  - `movement_handlers.go` - Movement, orbit, and lock handlers
  - `combat_handlers.go` - Weapons and combat systems
  - `ship_management_handlers.go` - Repair, beam, and bomb handlers
  - `communication_handlers.go` - All messaging systems
  - `bot_handlers.go` - Bot management commands
  - `handler_utils.go` - Shared utilities and data structures

- **Game Engine**: Core game mechanics and systems
  - `websocket.go` - WebSocket handling and game loop
  - `physics.go` - Physics simulation and collision detection
  - `projectiles.go` - Torpedo and projectile systems
  - `systems.go` - Ship systems (shields, weapons, engines)
  - `planets.go` - Planet mechanics and interactions
  - `tournament.go` - Tournament mode logic
  - `victory.go` - Victory conditions and game ending

- **Bot AI System**: Intelligent computer opponents
  - `bots.go` - Main bot coordination and initialization
  - `bot_combat.go` - Combat decision-making and targeting
  - `bot_navigation.go` - Movement and pathfinding
  - `bot_planet.go` - Strategic planet capture decisions
  - `bot_weapons.go` - Weapon firing and aim calculation
  - `bot_jitter.go` - Position randomization to prevent clustering
  - `bot_helpers.go`, `bot_types.go` - Supporting utilities

- **Utilities**: Supporting systems
  - `intercept.go` - Advanced torpedo targeting calculations
  - `game_helpers.go` - Game utility functions

#### Client Architecture (`static/`)
- `index.html`, `game.html` - Landing page and game interface
- `netrek.js` - Main game client and WebSocket communication
- `ship-renderer.js`, `planet-renderer.js` - Rendering systems
- `info-window.js` - HUD and information displays
- `ship-bitmaps-all-teams.js` - Ship sprite data
- `convert_bitmaps.js` - Utility to convert X11 bitmap data for planet sprites

#### Game Data (`game/`)
- `types.go` - Core game data structures
- `planets.go` - Planet configurations and initialization
- `torp.go` - Torpedo physics and range calculations

This modular structure enables:
- Clear separation of concerns
- Easy navigation to specific systems
- Parallel development without conflicts
- Comprehensive test coverage
- Maintainable and extensible codebase

### Testing

The project includes comprehensive test coverage for all critical systems:

#### Test Files
- **Game Logic**: `game/torp_test.go` - Torpedo physics and range calculations
- **Server Core**: 
  - `server/handlers_test.go` - HTTP and WebSocket handlers
  - `server/harness_test.go` - Test harness utilities
  - `server/test_helpers.go` - Testing support functions (helper file)
- **Bot AI System**: 
  - `server/bots_test.go` - Bot behaviors and decision-making
  - `server/bot_combat_test.go` - Bot combat improvements and targeting
  - `server/bot_jitter_test.go` - Position jittering system
- **Combat Systems**:
  - `server/starbase_fire_test.go` - Starbase weapon systems
  - `server/weapon_direction_test.go` - Weapon targeting calculations
  - `server/orbit_weapons_test.go` - Orbital velocity and weapon accuracy
- **Game Features**:
  - `server/tournament_test.go` - Tournament mode logic
  - `server/victory_test.go` - Victory condition testing
  - `server/formatting_test.go` - Message formatting tests
- **Utilities**: `server/intercept_test.go` - Advanced targeting algorithms

#### Running Tests
```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage report
go test -cover ./...

# Run specific test package
go test ./server
go test ./game

# Code quality checks
go fmt ./...
go vet ./...
go build ./...
```

## Credits

Based on the original Netrek (1986). Visit https://www.netrek.org/ for the classic game.

## License

Educational implementation. Original Netrek is open source.
