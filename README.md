# NETREK-WEB

A modern browser-based implementation of the classic Netrek game, written in Go with WebSocket communication and HTML5 Canvas rendering.

**⚠️ PROOF OF CONCEPT - This is a work in progress demonstration version ⚠️**

## Overview

This is a simplified but fully playable version of Netrek that runs entirely in a web browser. It features:

- **Single Go server** handling all game logic and client connections
- **WebSocket-based** real-time communication
- **HTML5 Canvas** rendering for smooth graphics
- **Modern web technologies** instead of the original X11/TCP protocol
- **10 FPS game loop** matching the original server tick rate (10 updates/second)

## Features Implemented

### Core Game Mechanics
- 4 teams (Federation, Romulan, Klingon, Orion)
- 40 planets with exact original positions and ownership
- 7 ship types with accurate stats from original Netrek (Scout, Destroyer, Cruiser, Battleship, Assault, Starbase, Galaxy)
- Real-time space combat with torpedoes (proper fuse timing) and phasers (with 10° auto-aim)
- Ship movement physics with speed-dependent turn rates (slower turning at high speed)
- Fuel and temperature management
- Shield and damage systems
- Planet orbiting with proper distance/speed checks (O key)
- Death and respawn system with explosion animations
- Plasma torpedoes for equipped ships (P key)
- Detonate own torpedoes (D key)
- Tractor/pressor beams (T/Y keys)
- Cloaking system (C key)
- Team and private messaging (A/T keys)
- Victory conditions (genocide and conquest)
- Tournament mode (automatic 4v4 activation)
- Bot players with intelligent AI (practice mode)

### User Interface
- **Tactical Display**: Local view centered on your ship (perfect square)
- **Galactic Map**: Overview of entire galaxy with planet labels (perfect square)
- **Dashboard**: Ship status (shields, damage, fuel, speed, kills, T-mode)
- **Player List**: Active players, scores, and bot indicators
- **Message Window**: Game notifications and team chat
- **Visual Effects**: Enhanced explosions, phaser beams, and particle effects

### Controls (Classic Netrek Style)
- **Mouse Controls** (ALL direction control is via mouse):
  - **Left Click**: Fire torpedo at cursor
  - **Middle Click**: Fire phaser at nearest enemy (10° auto-aim)
  - **Right Click**: Set course toward cursor (ONLY way to change direction)
- **Keyboard** (speed and ship functions only):
  - **0-9**: Set speed (0=stop, 9=speed 9)
  - **! @ #**: Set speed 10, 11, 12 (Shift+1/2/3 for fast ships like Scout)
  - **S**: Toggle shields
  - **C**: Toggle cloak
  - **D**: Detonate own torpedoes
  - **O**: Orbit planet (must be close and slow)
    - Orbit breaks automatically when: setting new course, changing speed, or locking different target
  - **R**: Repair mode (stop and repair with shields down)
  - **L**: Lock on nearest player/planet to mouse (auto-navigate)
  - **P**: Fire plasma torpedo (if equipped)
  - **T**: Tractor beam (pull enemy)
  - **Y**: Pressor beam (push enemy)
  - **Z/X**: Beam up/down armies when orbiting
  - **B**: Bomb planet when orbiting enemy planet
  - **A**: Send message to all
  - **Shift+T**: Send team message
  - **\\**: Toggle practice mode panel (add/remove bots)

## Installation and Running

### Prerequisites
- Go 1.25 or higher
- Modern web browser (Chrome, Firefox, Safari, Edge)

### Quick Installation

#### Option 1: Install from GitHub (Recommended)
```bash
# Install directly from GitHub
go install github.com/lab1702/netrek-web@latest

# Run the installed binary
netrek-web
```

#### Option 2: Clone and Build
```bash
# Clone the repository
git clone https://github.com/lab1702/netrek-web.git
cd netrek-web

# Install dependencies (minimal - only gorilla/websocket)
go mod download

# Run the server
go run main.go

# Or build a standalone binary
go build -o netrek-web main.go
./netrek-web
```

### Running the Server

```bash
# Default port 8080
netrek-web

# Custom port
netrek-web -port 3000

# Or with go run
go run main.go -port 8080
```

Open your browser and navigate to:
```
http://localhost:8080
```

### Building for Production

```bash
# Build optimized binary with embedded static files
go build -ldflags="-s -w" -o netrek-web main.go

# Run the production binary
./netrek-web -port 8080
```

**Windows Note**: If Windows Defender flags the binary, use:
```bash
go build -buildmode=exe -ldflags="-s -w" -o netrek-web.exe main.go
```

### Docker Deployment (Optional)
```bash
# Build Docker image
docker build -t netrek-web .

# Run container
docker run -d -p 8080:8080 netrek-web

# Or use docker-compose
docker-compose up -d
```

## Architecture

### Server (Go)
- `main.go`: HTTP server and static file serving
- `game/types.go`: Core game data structures (Player, Planet, Torpedo, etc.)
- `game/planets.go`: Planet initialization and layout
- `server/websocket.go`: WebSocket handling and game loop
- `server/handlers.go`: Client message processing (login, movement, combat)

### Client (JavaScript)
- `static/index.html`: Game UI layout
- `static/netrek.js`: Game client, rendering, and input handling

### Communication Protocol
Uses JSON over WebSocket instead of the original binary protocol:

```javascript
// Client to Server
{"type": "move", "data": {"dir": 1.57, "speed": 8}}
{"type": "fire", "data": {"dir": 0.785}}
{"type": "phaser", "data": {"target": 5}}

// Server to Client
{"type": "update", "data": {"players": [...], "planets": [...], "torps": [...]}}
```

## Differences from Original Netrek

### Simplifications
- No RSA authentication (uses simple name/team selection)
- Simplified network protocol (JSON over WebSocket vs binary TCP/UDP)
- No starbase transwarp or docking mechanics yet

### Improvements
- Single binary deployment (no daemon/client separation)
- Cross-platform (runs on any OS with Go)
- No installation required for clients (just a web browser)
- Modern networking (WebSocket vs TCP/UDP)
- Smooth 10 FPS rendering matching server ticks
- Responsive controls with visual feedback
- Enhanced visual effects (particle systems, gradients)

### Complete Feature Parity
- All 40 planets with exact original positions and attributes
- All 7 ship types with accurate statistics
- Combat mechanics (torpedoes, phasers, plasma)
- Planet mechanics (orbit, bomb, beam armies)
- Tractor/pressor beams with physics
- Cloaking system with fuel consumption
- Tournament mode with time limits
- Victory conditions (genocide and conquest)
- Bot players with advanced AI

## Recent Updates (As of 2025-08-24)

### Latest Updates (Session 3 - 2025-08-24)
- **Bot System Simplified**: 
  - Unified to single "hard mode" AI with dual-mode behavior
  - Tournament mode: Bots focus on strategic planet conquest
  - Practice mode: Bots prioritize combat for training
  - Bots can now use Scout through Assault ship classes
  - Auto-clear all bots when no human players connected
  
- **Player Management**:
  - Player list shows dead players with reduced opacity
  - Players sorted by team then alphabetically
  - 1-minute reconnection timeout for disconnected players
  - Self-destruct properly frees player slot
  
- **Galaxy Reset**: 
  - Automatically resets to initial state when server is empty
  - Home worlds now have AGRI flag for complete resources
  
- **Plasma Torpedoes Fixed**:
  - Authentic fuse values from original Netrek
  - Correct travel distances (~9000 units for destroyers)
  
- **UI Improvements**:
  - Help window properly shows single backslash for practice mode
  - Message input no longer starts with hotkey character
  - Landing page rebranded to NETREK-WEB with proof-of-concept notice

### Previous Features (Session 2 - 2025-08-23)
- **Complete Combat System**:
  - Planet bombing to 0 armies makes them independent (gray)
  - Beaming down armies conquers independent planets  
  - Continuous beam up/down modes (separate, not simultaneous)
  - Engine temperature (ETEMP) system with progressive overheat chances
  - Weapon temperature (WTEMP) tracking
  - Fuel consumption for shields (from original Netrek)
  - Engine overheat penalty limits speed to 1
  - Kills/Deaths/KD ratio tracking in player list

- **Team Balance System**:
  - Server population display on login screen
  - Real-time team player counts
  - Prevents joining teams with most players
  - Auto-selects available team when preferred team is full
  - Visual indicators for team balance (stars for recommended teams)
  - Radio buttons disabled for full teams

- **Enhanced UI**:
  - Radio button team/ship selection (replaced dropdowns)
  - Cruiser (CA) as default ship class
  - Army capacity bar in dashboard
  - Help window with complete controls reference (? key)
  - Info windows for planets and players (I key)
  - Escape key closes all windows
  - Quit/Self-destruct with confirmation (Q key)

### Previous Features (Session 1)
- **Repair Mode (R key)**: Stop and repair damage/fuel at 4x/2x normal rates
  - Ships must be stopped (speed 0) or orbiting to repair
  - Repair request state: Ships slow down to 0 before repairs begin
  - Repairs canceled when shields raised or ship starts moving
- **Lock-on System (L key)**: Lock onto nearest player or planet to mouse cursor
  - Auto-navigation toward locked target
  - Auto-orbit when approaching locked planet
  - Automatic speed adjustment when approaching planets
  - Smart turning: Ships automatically slow down when locked on planets for tighter turns
  - Lock clears automatically when entering orbit
  - Locking a different target breaks current orbit
- **Galaxy Edge Bouncing**: Ships bounce off galaxy edges (no wrapping)
- **Planet Mechanics**:
  - INL (International Netrek League) mode for dynamic planet flag distribution
  - Continuous bombing: Holds 'B' key state until planet has 0 armies
  - Fixed bombing rate to match original (averages 1.6 armies/second)
  - Planet damage to hostile ships (armies/10 + 2 damage within 1500 units)
- **Visual Improvements**:
  - Ship type letters displayed on vessels
  - Phaser hit animations
  - Enhanced explosion effects
  - Galaxy edge indicators on tactical map
  - Speed bar on dashboard showing current speed vs max speed
- **Practice Mode Panel (\\)**: Add/remove bot opponents
  - Single unified AI difficulty (optimized for practice)
  - Intelligent team balancing
  - Improved bot AI with repairs, dodging, and strategy
- **Speed Keys Extended**: Shift+1/2/3 for speeds 10/11/12
- **Damage-based Speed**: Ships slow down when damaged (original formula)
- **Player Reconnection**: Players can reconnect to same slot if disconnected
- **Landing Page**: Professional entry page with controls and credits

## Future Enhancements

### Gameplay
- [x] ~~Plasma torpedoes~~ ✅ Implemented
- [x] ~~Tractor/pressor beams~~ ✅ Implemented
- [x] ~~Planet mechanics (bombing, beaming armies)~~ ✅ Implemented
- [x] ~~Cloaking~~ ✅ Implemented
- [ ] Starbase docking and transwarp
- [x] ~~Tournament mode~~ ✅ Implemented
- [x] ~~Bot players~~ ✅ Implemented with intelligent AI
- [x] ~~Repair mode (R key)~~ ✅ Implemented
- [x] ~~Lock-on targeting (L key)~~ ✅ Implemented with auto-orbit
- [ ] Observer slots for spectating

### Technical
- [ ] Docker container deployment
- [ ] Kubernetes scaling
- [ ] Redis for game state persistence
- [ ] WebRTC for P2P voice chat
- [ ] Mobile touch controls
- [ ] Replay system
- [ ] Improved collision detection
- [ ] Smooth galaxy edge wrapping

## Development

### Project Structure
```
netrek-web/
├── main.go              # Entry point
├── go.mod               # Go module definition
├── game/                # Game logic
│   ├── types.go         # Data structures
│   └── planets.go       # Planet setup
├── server/              # Server logic
│   ├── websocket.go     # WebSocket & game loop
│   └── handlers.go      # Message handlers
└── static/              # Web client
    ├── index.html       # UI layout
    └── netrek.js        # Client logic
```

### Adding New Features

1. **New ship mechanics**: Modify `game/types.go` ShipData
2. **New weapons**: Add to combat logic in `server/websocket.go`
3. **New UI elements**: Update `static/index.html` and `static/netrek.js`
4. **New commands**: Add handlers in `server/handlers.go`

## Credits

Based on the original Netrek game (1986-present). This implementation is a tribute to the classic game and its community.

Original Netrek: https://www.netrek.org/

## License

This is an educational implementation for learning purposes. The original Netrek is open source under various licenses.

---

**Note**: This is a simplified implementation demonstrating how classic games can be modernized with current web technologies. For the full Netrek experience, play the original game!
