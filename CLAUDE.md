# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Netrek-Web is a browser-based real-time multiplayer space combat game faithful to the original Netrek (1986). Go backend with WebSocket communication, vanilla JavaScript frontend rendered on HTML5 Canvas. 4 teams, 6 ship types, 40 planets, up to 64 players, 10 FPS game loop.

## Dependencies

Go 1.25+. Single external dependency: `github.com/gorilla/websocket v1.5.3`.

## Build & Run Commands

```bash
# Build and run
go build && ./netrek-web
go run main.go
go run main.go -port 3000    # custom port (default 8080)
go install github.com/lab1702/netrek-web@latest  # install globally

# Docker
docker build -t netrek-web .
docker run -d -p 8080:8080 netrek-web

# Tests
go test ./...                # all tests
go test -v ./server          # server package verbose
go test -v ./game            # game package verbose
go test -run TestName ./server  # single test

# Test harness flags (for accuracy/benchmark tests)
go test -v ./server -run TestInterceptAccuracy -iterations=100 -target-speed=8.0 -target-pattern=zigzag

# Quality checks
go fmt ./...
go vet ./...
```

## Architecture

### Server-Side (Go)

**Entry point:** `main.go` — embeds static files via `go:embed`, sets up HTTP routes (`/`, `/ws`, `/api/teams`, `/health`), creates the Server, and starts the game loop with graceful shutdown.

**Game loop** (`server/websocket.go` `Server.gameLoop()`): Runs at 10 FPS. Each tick calls `updateGame()` which executes subsystems in order:
1. `updatePlayerPhysics` — movement, rotation, speed changes
2. `updateShipSystems` — fuel, temperature, repair, cloak, shields
3. `updateProjectiles` — torpedo and plasma movement, collision detection
4. `updatePlanets` — planet interactions, bombing, beaming, army growth
5. `UpdateBots` — AI decision-making
6. `checkVictoryConditions` — genocide/conquest detection
7. `sendGameState` — broadcast full state to all clients

**Handler pattern:** Client messages arrive as `{Type, Data}` JSON via WebSocket. `handleMessage()` in `websocket.go` routes to handler methods on the `Client` type, organized across files:
- `game_state_handlers.go` — login, quit
- `movement_handlers.go` — move, orbit, lock-on
- `combat_handlers.go` — torpedo, phaser, plasma
- `ship_management_handlers.go` — repair, beam, bomb
- `communication_handlers.go` — chat messages
- `bot_handlers.go` — bot management commands

**Concurrency:** `GameState` protected by `sync.RWMutex`. Handlers lock, mutate state, unlock. Broadcasting uses a non-blocking channel.

### Bot AI System (`server/bot_*.go`, `ai_constants.go`)

The bot AI is the largest subsystem (~9 files). Key entry: `updateBotHard()`. Modular sub-behaviors: shield management, target selection, weapon firing (with intercept calculations), navigation, planet capture strategy, and position jittering to prevent clustering. Tuning constants are centralized in `ai_constants.go`.

### Game Data (`game/`)

- `types.go` — all core structs (`Player`, `GameState`, `Torpedo`, `Plasma`, `Planet`, `ShipStats`) and game constants (galaxy size 100k×100k, damage values, ship stats for all 6 types)
- `planets.go` — initialization of 40 planets matching original Netrek layout
- `torp.go`, `plasma_range.go`, `damage.go` — physics calculations

### Client-Side (`static/`)

- `netrek.js` — main client (~2500 lines): WebSocket connection, input handling, game state sync, rendering orchestration
- `ship-renderer.js`, `planet-renderer.js` — canvas rendering
- `info-window.js` — HUD displays
- `ship-bitmaps-all-teams.js` — sprite data for all teams/ships

### Key Constants (from `game/types.go`)

- Galaxy: 100,000 × 100,000 units
- MaxPlayers: 64, MaxPlanets: 40
- MaxTorps: 8 per player, MaxPlasma: 1 per player
- PhaserDist/TractorDist: 6,000 units
- UpdateInterval: 100ms (10 FPS)

## Physics Fidelity

The codebase preserves original Netrek mechanics: ship turn rates use `turnRate >> speed`, damage reduces max speed via `(max+2) - (max+1)*(damage/maxdamage)`, movement uses fractional accumulators for sub-tick precision, and torpedo fuse mechanics use age-based expiration. Changes to physics should maintain compatibility with these formulas.

## Testing Patterns

- `server/test_helpers.go` exposes private server methods for testing
- `server/harness_test.go` provides configurable test harness with flags (`-iterations`, `-target-speed`, `-target-pattern`, `-verbose`, `-baseline`)
- Tests cover: physics, bot AI, combat systems, tournament mode, victory conditions, intercept accuracy, projectile lifecycle
