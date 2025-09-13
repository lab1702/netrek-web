# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Netrek-Web is a browser-based implementation of the classic Netrek game, written in Go with a WebSocket-based real-time multiplayer architecture. The game features 4 teams competing across 40 planets with 7 ship types, real-time space combat, and AI bot opponents.

## Development Commands

### Build and Run
```bash
# Build the server
go build

# Run the server (default port 8080)
./netrek-web

# Run on custom port
./netrek-web -port 3000

# Build and run directly
go run main.go
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
go test -v -run TestName ./server

# Code quality checks
go fmt ./...
go vet ./...
go build ./...
```

### Docker
```bash
# Build Docker image
docker build -t netrek-web .

# Run with Docker
docker run -d -p 8080:8080 netrek-web

# Use Docker Compose
docker-compose up -d
```

## Architecture Overview

### Core Game Loop
The server uses a tick-based game loop (server/websocket.go) running at 10 Hz (100ms ticks). Each tick processes:
1. Player inputs from WebSocket messages
2. Bot AI decisions
3. Physics updates (movement, collisions)
4. Combat calculations (weapons, damage)
5. Planet interactions (orbiting, bombing, beaming)
6. Victory condition checks
7. State broadcasts to all connected clients

### Message Handler Architecture
The server uses a modular handler system where incoming WebSocket messages are routed to specialized handlers based on message type. Each handler module focuses on a specific game aspect:

- **Game State**: Login/quit flow, player initialization (server/game_state_handlers.go)
- **Movement**: Ship movement, orbit, course setting, lock-on (server/movement_handlers.go)
- **Combat**: Weapons fire, damage calculation, explosions (server/combat_handlers.go)
- **Ship Management**: Repairs, shields, cloaking, tractor beams (server/ship_management_handlers.go)
- **Communication**: Team/all chat, message broadcasting (server/communication_handlers.go)
- **Bot Control**: Adding/removing bots, difficulty settings (server/bot_handlers.go)

### Bot AI System
The bot system (server/bots.go) uses a behavior tree approach with specialized modules:
- **Combat AI**: Target selection, weapon timing, tactical decisions (server/bot_combat.go)
- **Navigation**: Pathfinding, obstacle avoidance, orbit mechanics (server/bot_navigation.go)
- **Strategic AI**: Planet capture priorities, team coordination (server/bot_planet.go)
- **Weapon Systems**: Aim prediction using intercept calculations (server/bot_weapons.go, server/aimcalc/)

Bots run decision cycles every 5 game ticks (500ms) with position jittering to prevent clustering.

### Client-Server Protocol
WebSocket messages use a simple JSON protocol:
- Client→Server: `{type: "command", data: {...}}`
- Server→Client: Game state updates, player updates, combat events

Key message types:
- Movement: setcourse, speed, orbit, lock
- Combat: torp, phaser, plasma, detonate
- Systems: shields, cloak, repair, tractor
- Planet: bomb, beamup, beamdown

### Tournament Mode
Activates automatically with 4+ players per team. Features:
- Faster combat pace (reduced repair rates)
- Strategic planet scouting system
- Enhanced scoring and statistics
- Victory conditions based on planet control

## Key Implementation Details

### Physics System (server/physics.go)
- Position updates use velocity vectors with speed/direction
- Collision detection for ship-planet and ship-projectile
- Tractor beam force calculations
- Explosion damage falloff calculations

### Combat Mechanics (server/projectiles.go, server/combat_handlers.go)
- Torpedo travel time and fuel limits
- Phaser instant-hit with range/damage falloff
- Plasma torpedoes with area damage
- Detonation mechanics for friendly torpedoes

### Planet System (server/planets.go)
- Agricultural/fuel/repair planet types
- Army production and capacity
- Bombing/beaming mechanics with proximity checks
- Ownership and team control tracking

### WebSocket State Management (server/websocket.go)
- Player connection lifecycle handling
- Message queuing and broadcast optimization
- Automatic disconnection cleanup
- State synchronization on reconnect

## Testing Approach

Tests use a harness system (server/harness_test.go) that simulates the full game environment:
- Mock WebSocket connections
- Controlled game tick advancement
- State verification helpers
- Bot behavior validation

Focus areas for testing:
- Combat calculations and damage
- Movement and physics accuracy
- Tournament mode transitions
- Victory condition detection
- Bot decision-making logic