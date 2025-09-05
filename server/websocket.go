package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lab1702/netrek-web/game"
)

// isValidOrigin checks if the origin is allowed to connect
func isValidOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// No origin header - could be a non-browser client
		return true
	}

	originURL, err := url.Parse(origin)
	if err != nil {
		log.Printf("Invalid origin URL: %s", origin)
		return false
	}

	// Allow same-origin connections
	if r.Host == originURL.Host {
		return true
	}

	// Allow localhost connections for development
	if strings.HasPrefix(originURL.Host, "localhost:") ||
		strings.HasPrefix(originURL.Host, "127.0.0.1:") ||
		originURL.Host == "localhost" ||
		originURL.Host == "127.0.0.1" {
		return true
	}

	// Add any additional allowed origins here
	// Example: allowedOrigins := []string{"https://example.com", "https://app.example.com"}
	// for _, allowed := range allowedOrigins {
	//     if origin == allowed {
	//         return true
	//     }
	// }

	log.Printf("Rejected WebSocket connection from origin: %s", origin)
	return false
}

var upgrader = websocket.Upgrader{
	CheckOrigin:       isValidOrigin,
	EnableCompression: true, // Enable per-message deflate compression
}

// Message types
const (
	MsgTypeLogin      = "login"
	MsgTypeMove       = "move"
	MsgTypeFire       = "fire"
	MsgTypePhaser     = "phaser"
	MsgTypeShields    = "shields"
	MsgTypeOrbit      = "orbit"
	MsgTypeRepair     = "repair"
	MsgTypeLock       = "lock"
	MsgTypeBeam       = "beam"
	MsgTypeBomb       = "bomb"
	MsgTypeCloak      = "cloak"
	MsgTypeTractor    = "tractor"
	MsgTypePressor    = "pressor"
	MsgTypePlasma     = "plasma"
	MsgTypeDetonate   = "detonate"
	MsgTypeMessage    = "message"
	MsgTypeTeamMsg    = "teammsg"
	MsgTypePrivMsg    = "privmsg"
	MsgTypeQuit       = "quit"
	MsgTypeUpdate     = "update"
	MsgTypeError      = "error"
	MsgTypeTeamUpdate = "team_update"
)

// ClientMessage represents a message from client to server
type ClientMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// ServerMessage represents a message from server to client
type ServerMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Client represents a connected player
type Client struct {
	ID       int
	PlayerID int
	conn     *websocket.Conn
	send     chan ServerMessage
	server   *Server
}

// Server manages the game and client connections
type Server struct {
	mu          sync.RWMutex
	clients     map[int]*Client
	register    chan *Client
	unregister  chan *Client
	broadcast   chan ServerMessage
	gameState   *game.GameState
	nextID      int
	galaxyReset bool // Track if galaxy has been reset (true = already reset/empty)
}

// NewServer creates a new game server
func NewServer() *Server {
	return &Server{
		clients:     make(map[int]*Client),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan ServerMessage, 256),
		gameState:   game.NewGameState(),
		galaxyReset: true, // Start with galaxy already in reset state
	}
}

// Run starts the server main loop
func (s *Server) Run() {
	// Start game loop
	go s.gameLoop()

	// Handle client events
	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client.ID] = client
			s.mu.Unlock()
			log.Printf("Client %d connected", client.ID)

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client.ID]; ok {
				delete(s.clients, client.ID)
				close(client.send)

				// Immediately free the player slot on disconnect
				if client.PlayerID >= 0 && client.PlayerID < game.MaxPlayers {
					p := s.gameState.Players[client.PlayerID]
					// Only free if it's a human player (not a bot)
					if !p.IsBot {
						log.Printf("Freeing slot for disconnected player %s", p.Name)
						p.Status = game.StatusFree
						p.Name = ""
						p.Connected = false
						p.LastUpdate = time.Time{}

						// Broadcast updated team counts to all clients
						s.broadcastTeamCounts()
					}
				}
			}
			s.mu.Unlock()
			log.Printf("Client %d disconnected", client.ID)

		case message := <-s.broadcast:
			s.mu.RLock()
			for _, client := range s.clients {
				select {
				case client.send <- message:
					// Successfully sent
				default:
					// Client send channel is full, skip this message
					log.Printf("Warning: Client %d send buffer full, skipping broadcast", client.ID)
				}
			}
			s.mu.RUnlock()
		}
	}
}

// gameLoop runs the main game simulation
func (s *Server) gameLoop() {
	ticker := time.NewTicker(game.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.updateGame()
			s.sendGameState()
		}
	}
}

// updateGame updates the game physics
func (s *Server) updateGame() {
	s.gameState.Mu.Lock()
	defer s.gameState.Mu.Unlock()

	s.gameState.Frame++
	s.gameState.TickCount++

	// Check player status
	hasHumanPlayers := false
	hasAnyPlayers := false

	// First pass: check player status
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]

		// Check for any active players
		if p.Status != game.StatusFree {
			hasAnyPlayers = true
			if !p.IsBot && p.Connected {
				hasHumanPlayers = true
			}
		}
	}

	// Clear all bots if no human players are connected
	if !hasHumanPlayers {
		botCount := 0
		for i := 0; i < game.MaxPlayers; i++ {
			p := s.gameState.Players[i]
			if p.IsBot && p.Status != game.StatusFree {
				p.Status = game.StatusFree
				p.Name = ""
				p.IsBot = false
				p.Connected = false
				botCount++
			}
		}
		if botCount > 0 {
			log.Printf("Cleared %d bots (no human players or reserved slots)", botCount)
			// After clearing bots, check again if any players remain
			hasAnyPlayers = false
			for i := 0; i < game.MaxPlayers; i++ {
				if s.gameState.Players[i].Status != game.StatusFree {
					hasAnyPlayers = true
					break
				}
			}
		}
	}

	// Reset galaxy to initial state if no players at all
	if !hasAnyPlayers {
		// Only reset if we haven't already reset (transition from players to no players)
		if !s.galaxyReset {
			// Re-initialize planets to startup state
			game.InitPlanets(s.gameState)
			game.InitINLPlanetFlags(s.gameState)

			// Reset game state
			s.gameState.Frame = 0
			s.gameState.T_mode = false
			s.gameState.T_start = 0
			s.gameState.T_remain = 0
			s.gameState.GameOver = false
			s.gameState.Winner = 0
			s.gameState.WinType = ""

			// Clear tournament stats
			s.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)

			// Clear all torpedoes and plasmas
			s.gameState.Torps = make([]*game.Torpedo, 0)
			s.gameState.Plasmas = make([]*game.Plasma, 0)

			s.galaxyReset = true
			log.Printf("Galaxy reset to initial state (no players connected)")
		}
	} else {
		// We have players, mark that we're no longer in reset state
		s.galaxyReset = false
	}

	// Update each player
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]

		// Handle explosion state
		if p.Status == game.StatusExplode {
			// On the first frame of explosion (timer = 10), deal damage to nearby ships
			if p.ExplodeTimer == 10 && p.WhyDead != game.KillQuit {
				// Calculate explosion damage to nearby ships
				explosionDamage := game.GetShipExplosionDamage(p.Ship)

				// Check all other players for explosion damage
				for j := 0; j < game.MaxPlayers; j++ {
					if i == j {
						continue // Skip self
					}

					target := s.gameState.Players[j]
					if target.Status != game.StatusAlive {
						continue // Skip non-alive players
					}

					// Calculate distance to explosion
					dist := game.Distance(p.X, p.Y, target.X, target.Y)

					// Apply damage based on distance
					var damage int
					if dist <= game.ShipExplosionDist {
						// Full damage within close range
						damage = explosionDamage
					} else if dist < game.ShipExplosionMaxDist {
						// Reduced damage with linear falloff
						damage = int(float64(explosionDamage) * (game.ShipExplosionMaxDist - dist) / game.ShipExplosionRange)
					}

					if damage > 0 {
						target.Damage += damage
						if target.Damage >= game.ShipData[target.Ship].MaxDamage {
							// Ship destroyed by explosion!
							target.Status = game.StatusExplode
							target.ExplodeTimer = 10
							target.KilledBy = i
							target.WhyDead = game.KillExplosion
							// Clear lock-on when destroyed
							target.LockType = "none"
							target.LockTarget = -1

							// Update kill statistics
							if i >= 0 && i < game.MaxPlayers {
								p.Kills++
								p.KillsStreak++
							}
							target.Deaths++

							// Send kill message
							s.broadcastDeathMessage(target, p)
						}
					}
				}
			}

			p.ExplodeTimer--
			if p.ExplodeTimer <= 0 {
				// Check if this was a self-destruct quit
				if p.WhyDead == game.KillQuit {
					// Player quit via self-destruct, free the slot
					p.Status = game.StatusFree
					p.Name = ""
					p.Connected = false
					p.WhyDead = 0
					log.Printf("Player slot freed after self-destruct")
				} else {
					// Normal death, move to dead state
					p.Status = game.StatusDead
					// Clear their torpedoes and plasmas
					p.NumTorps = 0
					p.NumPlasma = 0
					// Will respawn next frame
				}
			}
			continue
		}

		// Handle dead state - respawn
		if p.Status == game.StatusDead && p.Connected {
			// Check if team owns planets during t-mode
			if s.gameState.T_mode {
				teamPlanetCount := 0
				switch p.Team {
				case game.TeamFed:
					teamPlanetCount = s.gameState.TeamPlanets[0]
				case game.TeamRom:
					teamPlanetCount = s.gameState.TeamPlanets[1]
				case game.TeamKli:
					teamPlanetCount = s.gameState.TeamPlanets[2]
				case game.TeamOri:
					teamPlanetCount = s.gameState.TeamPlanets[3]
				}

				// Cannot respawn if team owns no planets in t-mode
				if teamPlanetCount == 0 {
					// Send message to player once (check if not already sent)
					if p.ExplodeTimer == 0 {
						p.ExplodeTimer = -1 // Use -1 as flag that message was sent
						// Find the client for this player
						for _, client := range s.clients {
							if client.PlayerID == p.ID {
								client.send <- ServerMessage{
									Type: MsgTypeMessage,
									Data: map[string]interface{}{
										"text": "Cannot respawn - your team owns no planets in tournament mode!",
										"type": "error",
									},
								}
								break
							}
						}
					}
					continue
				} else if p.ExplodeTimer == -1 {
					// Team has regained planets - reset flag and notify player
					p.ExplodeTimer = 0
					// Find the client for this player
					for _, client := range s.clients {
						if client.PlayerID == p.ID {
							client.send <- ServerMessage{
								Type: MsgTypeMessage,
								Data: map[string]interface{}{
									"text": "Your team has regained planets - respawning enabled!",
									"type": "info",
								},
							}
							break
						}
					}
				}
			}

			// Respawn at home planet
			s.respawnPlayer(p)
			continue
		}

		if p.Status != game.StatusAlive {
			continue
		}

	}

	// Update game systems using extracted modules
	s.updateShipSystems()        // Fuel, heat, repair, cloak for all players
	s.updatePlanetInteractions() // Planet interactions, orbital mechanics, bombing/beaming
	s.updateProjectiles()        // Torpedo and plasma movement/collision
	s.updateTractorBeams()       // Tractor/pressor beam physics
	s.updateAlertLevels()        // Alert level calculations

	// Update all player physics individually
	for i := 0; i < game.MaxPlayers; i++ {
		p := s.gameState.Players[i]
		if p.Status != game.StatusAlive {
			continue
		}

		// Update physics for this player
		s.updatePlayerPhysics(p, i)

		// Update orbital mechanics
		s.updatePlayerOrbit(p)

		// Update lock-on tracking
		s.updatePlayerLockOn(p)
	}

	// Update bot AI
	s.UpdateBots()

	// Check tournament mode
	s.checkTournamentMode()

	// Check victory conditions
	s.checkVictoryConditions()
}

// sendGameState sends the current game state to all clients
func (s *Server) sendGameState() {
	s.gameState.Mu.RLock()

	// Create update message with relevant game state
	update := struct {
		Frame    int64           `json:"frame"`
		Players  []*game.Player  `json:"players"`
		Planets  []*game.Planet  `json:"planets"`
		Torps    []*game.Torpedo `json:"torps"`
		Plasmas  []*game.Plasma  `json:"plasmas"`
		GameOver bool            `json:"gameOver"`
		Winner   int             `json:"winner,omitempty"`
		WinType  string          `json:"winType,omitempty"`
		TMode    bool            `json:"tMode"`
		TRemain  int             `json:"tRemain,omitempty"`
	}{
		Frame:    s.gameState.Frame,
		Players:  s.gameState.Players[:],
		Planets:  s.gameState.Planets[:],
		Torps:    s.gameState.Torps,
		Plasmas:  s.gameState.Plasmas,
		GameOver: s.gameState.GameOver,
		Winner:   s.gameState.Winner,
		WinType:  s.gameState.WinType,
		TMode:    s.gameState.T_mode,
		TRemain:  s.gameState.T_remain,
	}

	s.gameState.Mu.RUnlock()

	s.broadcast <- ServerMessage{
		Type: MsgTypeUpdate,
		Data: update,
	}
}

// HandleTeamStats returns current team populations
func (s *Server) HandleTeamStats(w http.ResponseWriter, r *http.Request) {
	// Enable CORS for cross-origin requests
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	s.gameState.Mu.RLock()
	defer s.gameState.Mu.RUnlock()

	// Count players per team
	teamCounts := map[string]int{
		"fed": 0,
		"rom": 0,
		"kli": 0,
		"ori": 0,
	}

	totalPlayers := 0
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Connected {
			totalPlayers++
			switch p.Team {
			case game.TeamFed:
				teamCounts["fed"]++
			case game.TeamRom:
				teamCounts["rom"]++
			case game.TeamKli:
				teamCounts["kli"]++
			case game.TeamOri:
				teamCounts["ori"]++
			}
		}
	}

	response := map[string]interface{}{
		"total": totalPlayers,
		"teams": teamCounts,
	}

	json.NewEncoder(w).Encode(response)
}

// HandleWebSocket handles WebSocket connections
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}

	s.mu.Lock()
	clientID := s.nextID
	s.nextID++
	s.mu.Unlock()

	client := &Client{
		ID:       clientID,
		PlayerID: -1,
		conn:     conn,
		send:     make(chan ServerMessage, 256),
		server:   s,
	}

	s.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump handles incoming messages from the client
func (c *Client) readPump() {
	defer func() {
		c.server.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		var msg ClientMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		c.handleMessage(msg)
	}
}

// writePump sends messages to the client
func (c *Client) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage processes a message from the client
func (c *Client) handleMessage(msg ClientMessage) {
	// Recover from any panic to prevent disconnection
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC in handleMessage for client %d, type %s: %v", c.ID, msg.Type, r)
		}
	}()

	switch msg.Type {
	case MsgTypeLogin:
		c.handleLogin(msg.Data)
	case MsgTypeMove:
		c.handleMove(msg.Data)
	case MsgTypeFire:
		c.handleFire(msg.Data)
	case MsgTypePhaser:
		c.handlePhaser(msg.Data)
	case MsgTypeShields:
		c.handleShields(msg.Data)
	case MsgTypeOrbit:
		c.handleOrbit(msg.Data)
	case MsgTypeRepair:
		c.handleRepair(msg.Data)
	case MsgTypeLock:
		c.handleLock(msg.Data)
	case MsgTypeBeam:
		c.handleBeam(msg.Data)
	case MsgTypeBomb:
		c.handleBomb(msg.Data)
	case MsgTypeTractor:
		c.handleTractor(msg.Data)
	case MsgTypePressor:
		c.handlePressor(msg.Data)
	case MsgTypePlasma:
		c.handlePlasma(msg.Data)
	case MsgTypeDetonate:
		c.handleDetonate(msg.Data)
	case MsgTypeCloak:
		c.handleCloak(msg.Data)
	case MsgTypeMessage:
		c.handleChatMessage(msg.Data)
	case MsgTypeTeamMsg:
		c.handleTeamMessage(msg.Data)
	case MsgTypePrivMsg:
		c.handlePrivateMessage(msg.Data)
	case MsgTypeQuit:
		c.handleQuit(msg.Data)
	default:
		log.Printf("Unknown message type: %s", msg.Type)
	}
}
