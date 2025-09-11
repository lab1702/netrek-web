# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Run
```bash
# Build the binary
go build

# Run server (default port 8080)
go run main.go

# Run on custom port
go run main.go -port 3000
```

### Testing
```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...

# Run specific package tests
go test ./server
go test ./game
go test ./server/aimcalc

# Run a single test
go test -run TestName ./server
```

### Code Quality
```bash
# Format code
go fmt ./...

# Check for code issues
go vet ./...

# Verify all packages build
go build ./...
```

### Docker
```bash
# Build Docker image
docker build -t netrek-web .

# Run with Docker Compose
docker-compose up -d
```

## Architecture

### WebSocket Message Protocol
The game uses WebSocket for real-time communication. Messages are JSON-encoded with a "type" field indicating the message type. Key message types include:
- Client→Server: login, move, fire, phaser, plasma, shields, orbit, repair, lock, cloak, tractor, bomb, beam, message, teamMessage, addBot, quit
- Server→Client: loginResponse, gameState, playerJoined, playerLeft, message, teamMessage, error

### Module Organization

#### Server (`server/`)
The server is organized into focused handler modules:
- **Message Handlers**: Modular handlers for different game aspects (game_state_handlers.go, movement_handlers.go, combat_handlers.go, ship_management_handlers.go, communication_handlers.go, bot_handlers.go)
- **Core Systems**: websocket.go (main game loop), physics.go (simulation), projectiles.go (torpedoes/plasma), systems.go (ship systems), planets.go (planet mechanics)
- **Bot AI**: Comprehensive bot system with separate modules for combat, navigation, planet capture, weapons, and jitter prevention
- **Tournament**: tournament.go and victory.go handle tournament mode and win conditions

#### Game Core (`game/`)
- `types.go`: Core data structures (Player, Planet, Ship specs, Teams)
- `planets.go`: Planet configurations and initialization
- `torp.go`: Torpedo physics calculations

#### Client (`static/`)
- `netrek.js`: Main game client and WebSocket handling
- `ship-renderer.js`, `planet-renderer.js`: Canvas rendering
- `info-window.js`: HUD and game information display
- Files are embedded in the binary using Go's embed directive

### Key Technical Details

- **Embedded Static Files**: Uses Go 1.16+ embed for bundling static files into the binary
- **WebSocket Library**: Uses gorilla/websocket for WebSocket handling
- **Game Loop**: 100ms tick rate (10 FPS) for physics simulation
- **Coordinate System**: 2D space with (0,0) at top-left, max coordinates at (100000, 100000)
- **Team IDs**: 0=Federation, 1=Romulan, 2=Klingon, 3=Orion
- **Ship Types**: 0=Scout, 1=Destroyer, 2=Cruiser, 3=Battleship, 4=Assault, 5=Starbase, 6=Galaxy

### Testing Approach
- Test files follow Go convention (*_test.go)
- harness_test.go and test_helpers.go provide testing utilities
- Tests cover game logic, handlers, bot AI, combat systems, and victory conditions
- Use table-driven tests where appropriate