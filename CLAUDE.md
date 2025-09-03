# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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
```

Note: The project currently has no tests. When adding tests, use standard Go testing commands:
```bash
go test ./...
go test -v ./...
```

## Architecture

### Server-Side Structure
- **main.go**: Entry point, sets up HTTP server, serves static files, and WebSocket endpoint
- **server/**: Core game server logic
  - `handlers.go`: HTTP and WebSocket handlers, client message processing
  - `websocket.go`: Game loop, physics, combat mechanics, planet interactions
  - `bots.go`: Bot AI implementation with combat and strategic behaviors
  - `game_helpers.go`: Utility functions for game mechanics
- **game/**: Game data structures and initialization
  - `types.go`: Core game types (Player, Planet, Torpedo, GameState, etc.)
  - `planets.go`: Planet initialization and configurations

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

## Development Notes

- The game state is maintained server-side in `GameState` struct
- All game physics and collision detection happen server-side
- Client receives state updates and renders based on server data
- Bot AI uses strategic evaluation for planet selection and combat decisions
- No external dependencies beyond gorilla/websocket
