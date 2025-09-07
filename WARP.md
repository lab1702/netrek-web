# WARP.md

This file provides guidance to Warp (warp.dev) when working with code in this repository.

## Project Overview

Netrek-Web is a browser-based implementation of the classic Netrek game using Go for the server and vanilla JavaScript for the client. The game runs on port 8080 by default and uses WebSockets for real-time communication.

## Build and Run Commands

### Development
```bash
# Build the server
go build

# Run the server
./netrek-web
```

### Docker
```bash
# Build and run with Docker Compose
docker-compose up -d

# Rebuild after changes
./update_docker.sh

# Watch logs
./watch_docker.sh
```

### Code Quality
```bash
# Format Go code
go fmt ./...

# Run static analysis
go vet ./...

# Clean up dependencies
go mod tidy

# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -cover ./...
```

## Architecture

### Server-Side Structure
- **main.go**: Entry point, sets up HTTP server, serves static files, and WebSocket endpoint

- **server/**: Core game server logic organized into focused modules
  - **Handler Modules**: Modular message processing system
    - `handlers.go`: Main HTTP and WebSocket handler coordination
    - `handler_utils.go`: Shared handler utilities and data structures
    - `game_state_handlers.go`: Login, quit, and player state management
    - `movement_handlers.go`: Movement, orbit, and lock-on handlers
    - `combat_handlers.go`: Weapon systems and combat mechanics
    - `ship_management_handlers.go`: Repair, beam up/down, and bombing
    - `communication_handlers.go`: All messaging systems (team, all, individual)
    - `bot_handlers.go`: Bot management commands and practice mode

  - **Game Engine**: Core game mechanics and physics
    - `websocket.go`: Game loop, WebSocket management, and state broadcasting
    - `physics.go`: Physics simulation, collision detection, and movement
    - `projectiles.go`: Torpedo and projectile systems
    - `systems.go`: Ship systems (shields, weapons, engines, cloak)
    - `planets.go`: Planet mechanics, capture logic, and interactions
    - `tournament.go`: Tournament mode activation and management
    - `victory.go`: Victory conditions and game ending logic

  - **Bot AI System**: Intelligent computer opponents
    - `bots.go`: Main bot coordination, initialization, and lifecycle
    - `bot_combat.go`: Combat decision-making and target selection
    - `bot_navigation.go`: Movement, pathfinding, and strategic positioning
    - `bot_planet.go`: Planet capture strategies and army management
    - `bot_weapons.go`: Weapon firing, targeting, and aim calculation
    - `bot_jitter.go`: Position randomization to prevent clustering
    - `bot_helpers.go`: Utility functions for bot AI operations
    - `bot_types.go`: Bot-specific data structures and constants

  - **Utilities and Support**:
    - `game_helpers.go`: General game utility functions
    - `aimcalc/intercept.go`: Advanced torpedo targeting and interception calculations
    - `aimcalc/intercept_test.go`: Test coverage for targeting algorithms

- **game/**: Game data structures and initialization
  - `types.go`: Core game types (Player, Planet, Torpedo, GameState, etc.)
  - `planets.go`: Planet initialization and strategic configurations
  - `torp.go`: Torpedo physics calculations, range functions, and ballistics

### Client-Side Structure  
- **static/**: Web client files
  - `index.html`: Landing page with team selection
  - `game.html`: Main game interface
  - `netrek.js`: Core game client, WebSocket communication, rendering loop
  - `ship-renderer.js`: Ship rendering logic
  - `planet-renderer.js`: Planet rendering logic
  - `info-window.js`: HUD and information displays
  - `ship-bitmaps-all-teams.js`: Ship sprite data

### Key Architectural Patterns

1. **WebSocket Communication**: The server maintains persistent WebSocket connections with clients, broadcasting game state updates at 10 FPS
2. **Game Loop**: Server runs a tick-based game loop updating physics, checking collisions, and processing player inputs
3. **Bot System**: Bots run server-side with AI routines for combat, planet capture, and strategic decisions
4. **Embedded Static Files**: Static files are embedded in the binary using Go's embed directive for single-file deployment

## Game Mechanics

- 4 teams (Federation, Romulan, Klingon, Orion) with 40 planets
- 7 ship types with varying stats (Scout to Galaxy class)
- Combat includes torpedoes, phasers, and plasma torpedoes
- Strategic elements: planet capture, army transport, resource management
- Tournament mode activates with 4+ players per team

## API Endpoints

- `/` - Serves static files (landing page)
- `/ws` - WebSocket endpoint for game communication
- `/api/teams` - REST endpoint for team statistics
- `/health` - Health check endpoint

## Testing

The project includes comprehensive test coverage for critical components:

### Test Files

Comprehensive test coverage across all critical systems:

#### Game Logic Tests
- **game/torp_test.go**: Torpedo physics calculations, range functions, and ballistics

#### Server Core Tests
- **server/handlers_test.go**: HTTP and WebSocket message handlers
- **server/harness_test.go**: Test harness utilities and setup functions
- **server/test_helpers.go**: Shared testing utilities and mock functions

#### Bot AI System Tests
- **server/bots_test.go**: Bot behaviors, decision-making, and coordination
- **server/bot_jitter_test.go**: Position jittering and clustering prevention

#### Combat System Tests
- **server/starbase_fire_test.go**: Starbase weapon systems and defensive capabilities
- **server/weapon_direction_test.go**: Weapon targeting calculations and accuracy

#### Game Feature Tests
- **server/tournament_test.go**: Tournament mode activation and management
- **server/victory_test.go**: Victory conditions and game ending scenarios

#### Utility Tests
- **server/aimcalc/intercept_test.go**: Advanced targeting algorithms and interception calculations

### Running Tests
```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage report
go test -cover ./...

# Run specific test file
go test ./server -run TestSpecificFunction
```

## Development Notes

- The game state is maintained server-side in `GameState` struct
- All game physics and collision detection happen server-side
- Client receives state updates and renders based on server data
- Bot AI uses strategic evaluation for planet selection and combat decisions
- Bot system is modularized across multiple files for maintainability
- Torpedo physics calculations are centralized in game/torp.go
- No external dependencies beyond gorilla/websocket
