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
go build -o netrek-web main.go
./netrek-web -port 8080
```

### Docker

```bash
docker build -t netrek-web .
docker run -d -p 8080:8080 netrek-web
```

## Game Features

- 4 teams with 40 planets
- 7 ship types (Scout to Galaxy class)
- Real-time space combat
- Tournament mode (4v4+)
- Practice mode with bots
- Team messaging system

## Development

### Code Organization

The server-side handler code is organized into focused modules for maintainability:

- **`server/handler_utils.go`** - Shared utilities, constants, and data structures
- **`server/game_state_handlers.go`** - Login and quit handlers  
- **`server/movement_handlers.go`** - Movement, orbit, and lock handlers
- **`server/combat_handlers.go`** - Weapons and combat systems
- **`server/ship_management_handlers.go`** - Repair, beam, and bomb handlers
- **`server/communication_handlers.go`** - All messaging systems
- **`server/bot_handlers.go`** - Bot management commands

This modular structure makes it easier for developers to:
- Navigate to specific game systems
- Work on features without conflicts
- Understand code organization
- Maintain and extend functionality

### Testing

```bash
go test ./...
go vet ./...
go build ./...
```

## Credits

Based on the original Netrek (1986). Visit https://www.netrek.org/ for the classic game.

## License

Educational implementation. Original Netrek is open source.
