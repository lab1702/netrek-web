package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lab1702/netrek-web/game"
)

const (
	// WebSocket connection timeouts
	wsReadTimeout  = 60 * time.Second // Read deadline for client messages
	wsPingInterval = 54 * time.Second // Ping interval (must be less than wsReadTimeout)

	// Maximum concurrent WebSocket connections to prevent memory exhaustion.
	// Each connection spawns 2 goroutines and a 256-entry channel buffer.
	maxConnections = 128
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
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Client represents a connected player
type Client struct {
	ID       int
	playerID atomic.Int32 // Use atomic to avoid data race between gameLoop and handlers
	conn     *websocket.Conn
	send     chan ServerMessage
	server   *Server

	// Rate limiting for destructive bot commands
	lastBotCmd     time.Time // Last /fillbots or /clearbots execution
	botCmdCooldown time.Duration
}

// GetPlayerID returns the player ID atomically
func (c *Client) GetPlayerID() int {
	return int(c.playerID.Load())
}

// SetPlayerID sets the player ID atomically
func (c *Client) SetPlayerID(id int) {
	c.playerID.Store(int32(id))
}

// Server manages the game and client connections
type Server struct {
	mu                     sync.RWMutex
	clients                map[int]*Client
	register               chan *Client
	unregister             chan *Client
	broadcast              chan ServerMessage
	gameState              *game.GameState
	nextID                 int
	nextTorpID             int  // Monotonically increasing torpedo ID
	nextPlasmaID           int  // Monotonically increasing plasma ID
	galaxyReset            bool // Track if galaxy has been reset (true = already reset/empty)
	done                   chan struct{}
	playerGrid             *SpatialGrid       // Spatial index for efficient collision detection
	pendingSuggestions     []targetSuggestion // Buffered target suggestions applied after UpdateBots
	cachedTeamPlanets      map[int]int           // Cached planet counts per team
	cachedTeamPlanetsFrame int64                 // Frame when cache was last computed
	cachedThreats          map[int]CombatThreat  // Per-bot threat cache
	cachedThreatsFrame     int64                 // Frame when threat cache was last valid
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
		done:        make(chan struct{}),
		playerGrid:  NewSpatialGrid(),
	}
}

// Shutdown signals the server to stop background goroutines
func (s *Server) Shutdown() {
	close(s.done)
}

// Run starts the server main loop
func (s *Server) Run() {
	// Start game loop
	go s.gameLoop()

	// Handle client events
	for {
		select {
		case <-s.done:
			return
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client.ID] = client
			s.mu.Unlock()
			log.Printf("Client %d connected", client.ID)

		case client := <-s.unregister:
			needBroadcast := false
			// Capture playerID once to avoid race between multiple GetPlayerID() calls
			playerID := client.GetPlayerID()
			s.mu.Lock()
			if _, ok := s.clients[client.ID]; ok {
				delete(s.clients, client.ID)
				close(client.send)

				// Immediately free the player slot on disconnect
				if playerID >= 0 && playerID < game.MaxPlayers {
					s.gameState.Mu.Lock()
					p := s.gameState.Players[playerID]
					isBot := p.IsBot
					// Only free if it's a human player (not a bot)
					if !isBot {
						log.Printf("Freeing slot for disconnected player %s", p.Name)
						p.Status = game.StatusFree
						p.Name = ""
						p.Connected = false
						p.LastUpdate = time.Time{}
						needBroadcast = true
					}
					s.gameState.Mu.Unlock()
				}
			}
			s.mu.Unlock()
			// Broadcast updated team counts after releasing s.mu to avoid deadlock
			if needBroadcast {
				s.broadcastTeamCounts()
			}
			log.Printf("Client %d disconnected", client.ID)

		case message := <-s.broadcast:
			s.mu.RLock()
			// If the message has a "to" field, only send to that player's client
			// to avoid leaking private error/info messages to all players.
			targetPlayerID := -1
			if dataMap, ok := message.Data.(map[string]interface{}); ok {
				if to, exists := dataMap["to"]; exists {
					switch v := to.(type) {
					case int:
						targetPlayerID = v
					case float64:
						targetPlayerID = int(v)
					}
				}
			}
			for _, client := range s.clients {
				if targetPlayerID >= 0 && client.GetPlayerID() != targetPlayerID {
					continue // Skip clients that are not the intended recipient
				}
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

// pendingPlayerMsg is a message to send to a specific player after locks are released
type pendingPlayerMsg struct {
	playerID int
	msg      ServerMessage
}

// gameLoop runs the main game simulation
func (s *Server) gameLoop() {
	ticker := time.NewTicker(game.UpdateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.done:
			return
		case <-ticker.C:
			pending := s.updateGame()
			// Send buffered per-player messages after game state lock is released
			if len(pending) > 0 {
				s.mu.RLock()
				for _, pm := range pending {
					for _, client := range s.clients {
						if client.GetPlayerID() == pm.playerID {
							select {
							case client.send <- pm.msg:
							default:
							}
							break
						}
					}
				}
				s.mu.RUnlock()
			}
			s.sendGameState()
		}
	}
}

// updateGame updates the game physics and returns buffered per-player messages
func (s *Server) updateGame() []pendingPlayerMsg {
	s.gameState.Mu.Lock()
	defer s.gameState.Mu.Unlock()

	var pendingMsgs []pendingPlayerMsg

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

			// Reset projectile IDs to prevent eventual overflow after billions of shots
			s.nextTorpID = 0
			s.nextPlasmaID = 0

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
			// On the first frame of explosion, deal damage to nearby ships
			if p.ExplodeTimer == game.ExplodeTimerFrames && p.WhyDead != game.KillQuit {
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
						actualDamage := game.ApplyDamageWithShields(target, damage)
						if target.Damage >= game.ShipData[target.Ship].MaxDamage {
							s.killPlayer(target, i, game.KillExplosion, actualDamage)
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
					p.WhyDead = game.KillNone
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
					if !p.RespawnMsgSent {
						p.RespawnMsgSent = true
						pendingMsgs = append(pendingMsgs, pendingPlayerMsg{
							playerID: p.ID,
							msg: ServerMessage{
								Type: MsgTypeMessage,
								Data: map[string]interface{}{
									"text": "Cannot respawn - your team owns no planets in tournament mode!",
									"type": "error",
								},
							},
						})
					}
					continue
				} else if p.RespawnMsgSent {
					// Team has regained planets - reset flag and notify player
					p.RespawnMsgSent = false
					pendingMsgs = append(pendingMsgs, pendingPlayerMsg{
						playerID: p.ID,
						msg: ServerMessage{
							Type: MsgTypeMessage,
							Data: map[string]interface{}{
								"text": "Your team has regained planets - respawning enabled!",
								"type": "info",
							},
						},
					})
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
	// Apply buffered target suggestions after all bots have been processed,
	// so processing order does not affect targeting decisions.
	s.ApplyPendingTargetSuggestions()

	// Check tournament mode
	s.checkTournamentMode()

	// Check victory conditions
	s.checkVictoryConditions()

	return pendingMsgs
}

// sendGameState sends the current game state to all clients
func (s *Server) sendGameState() {
	s.gameState.Mu.RLock()

	// Marshal game state to JSON while holding the lock to prevent races.
	// Use pointers from GameState arrays directly to avoid copying large structs.
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

	data, err := json.Marshal(update)
	s.gameState.Mu.RUnlock()

	if err != nil {
		log.Printf("Error marshaling game state: %v", err)
		return
	}

	// Non-blocking send to prevent the game loop from stalling if the
	// broadcast channel is full (e.g. due to heavy chat traffic).
	select {
	case s.broadcast <- ServerMessage{
		Type: MsgTypeUpdate,
		Data: json.RawMessage(data),
	}:
	default:
		log.Printf("Warning: broadcast channel full, dropping game state update")
	}
}

// HandleTeamStats returns current team populations
func (s *Server) HandleTeamStats(w http.ResponseWriter, r *http.Request) {
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
		// Count all connected players including dead/exploding (they're still on the team)
		if p.Connected && p.Status != game.StatusFree {
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

	_ = json.NewEncoder(w).Encode(response)
}

// HandleWebSocket handles WebSocket connections
func (s *Server) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Reject new connections when at capacity to prevent memory exhaustion.
	s.mu.RLock()
	numClients := len(s.clients)
	s.mu.RUnlock()
	if numClients >= maxConnections {
		http.Error(w, "Server full", http.StatusServiceUnavailable)
		return
	}

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
		ID:             clientID,
		conn:           conn,
		send:           make(chan ServerMessage, 256),
		server:         s,
		botCmdCooldown: 10 * time.Second,
	}
	client.SetPlayerID(-1)

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

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
		return nil
	})

	// Per-client rate limiting: allow bursts but cap sustained rate
	const maxMessagesPerSecond = 50
	messageCount := 0
	rateLimitReset := time.Now()

	for {
		var msg ClientMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Rate limiting: reset counter each second, drop messages if over limit
		now := time.Now()
		if now.Sub(rateLimitReset) >= time.Second {
			messageCount = 0
			rateLimitReset = now
		}
		messageCount++
		if messageCount > maxMessagesPerSecond {
			continue // Drop excess messages silently
		}

		c.handleMessage(msg)
	}
}

// writePump sends messages to the client
func (c *Client) writePump() {
	ticker := time.NewTicker(wsPingInterval)
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
			log.Printf("PANIC in handleMessage for client %d, type %s: %v\n%s", c.ID, msg.Type, r, debug.Stack())
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
