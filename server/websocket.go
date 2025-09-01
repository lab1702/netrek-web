package server

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
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
	MsgTypeLogin    = "login"
	MsgTypeMove     = "move"
	MsgTypeFire     = "fire"
	MsgTypePhaser   = "phaser"
	MsgTypeShields  = "shields"
	MsgTypeOrbit    = "orbit"
	MsgTypeRepair   = "repair"
	MsgTypeLock     = "lock"
	MsgTypeBeam     = "beam"
	MsgTypeBomb     = "bomb"
	MsgTypeCloak    = "cloak"
	MsgTypeTractor  = "tractor"
	MsgTypePressor  = "pressor"
	MsgTypePlasma   = "plasma"
	MsgTypeDetonate = "detonate"
	MsgTypeMessage  = "message"
	MsgTypeTeamMsg  = "teammsg"
	MsgTypePrivMsg  = "privmsg"
	MsgTypeQuit     = "quit"
	MsgTypeUpdate   = "update"
	MsgTypeError    = "error"
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

							// Update kill statistics
							if i >= 0 && i < game.MaxPlayers {
								p.Kills++
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
			// Respawn at home planet
			s.respawnPlayer(p)
			continue
		}

		if p.Status != game.StatusAlive {
			continue
		}

		// Update direction using original Netrek turning algorithm
		if p.Dir != p.DesDir {
			shipStats := game.ShipData[p.Ship]

			// Calculate turn increment based on speed (original NEWTURN=0 algorithm)
			var turnIncrement int
			speed := int(p.Speed)
			if speed < 30 {
				// Use bit shift: turnRate / (2^speed)
				turnIncrement = shipStats.TurnRate >> uint(speed)
			} else {
				// Very high speeds get minimal turning
				turnIncrement = 0
			}

			// Add to fractional accumulator
			p.SubDir += turnIncrement

			// Extract whole direction units and keep remainder
			ticks := p.SubDir / 1000
			p.SubDir = p.SubDir % 1000

			// Convert direction to 0-255 scale for calculation (like original)
			currentDir256 := int(p.Dir * 256.0 / (2 * math.Pi))
			desiredDir256 := int(p.DesDir * 256.0 / (2 * math.Pi))

			// Calculate shortest turn direction
			diff := desiredDir256 - currentDir256
			if diff > 128 {
				diff -= 256
			} else if diff < -128 {
				diff += 256
			}

			// Apply turn
			if math.Abs(float64(diff)) <= float64(ticks) {
				p.Dir = p.DesDir
			} else if diff > 0 {
				currentDir256 = (currentDir256 + ticks) % 256
				p.Dir = float64(currentDir256) * 2 * math.Pi / 256.0
			} else {
				currentDir256 = (currentDir256 - ticks + 256) % 256
				p.Dir = float64(currentDir256) * 2 * math.Pi / 256.0
			}
		}

		// Update speed (with damage-based max speed enforcement)
		if p.Speed != p.DesSpeed {
			// Calculate max speed based on damage
			shipStats := game.ShipData[p.Ship]
			maxSpeed := float64(shipStats.MaxSpeed)
			if p.Damage > 0 {
				// Formula from original Netrek: maxspeed = (max + 2) - (max + 1) * (damage / maxdamage)
				damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
				maxSpeed = float64(shipStats.MaxSpeed+2) - float64(shipStats.MaxSpeed+1)*damageRatio
				maxSpeed = math.Max(1, maxSpeed) // Minimum speed of 1
			}

			// Engine overheat limits speed to 1 (from original daemon.c)
			if p.EngineOverheat {
				maxSpeed = 1
				p.DesSpeed = math.Min(p.DesSpeed, 1)
			}

			// Enforce max speed limit
			actualDesSpeed := math.Min(p.DesSpeed, maxSpeed)

			// Ship-specific acceleration/deceleration using fractional accumulator
			// Based on original Netrek's acceleration system
			if actualDesSpeed > p.Speed {
				// Accelerating
				p.AccFrac += shipStats.AccInt
				// Each 1000 units of accumulator = 1 speed unit change (original Netrek uses 1000)
				if p.AccFrac >= 1000 {
					speedInc := p.AccFrac / 1000
					p.Speed = math.Min(p.Speed+float64(speedInc), actualDesSpeed)
					p.AccFrac = p.AccFrac % 1000
				}
			} else if actualDesSpeed < p.Speed {
				// Decelerating
				p.AccFrac += shipStats.DecInt
				// Each 1000 units of accumulator = 1 speed unit change (original Netrek uses 1000)
				if p.AccFrac >= 1000 {
					speedDec := p.AccFrac / 1000
					p.Speed = math.Max(p.Speed-float64(speedDec), actualDesSpeed)
					p.AccFrac = p.AccFrac % 1000
				}
			}
		}

		// Check if ship has slowed down to 0 for repair request
		if p.RepairRequest && p.Speed == 0 && p.Orbiting < 0 {
			// Transition from repair request to actual repair
			p.RepairRequest = false
			p.Repairing = true
			// Send message about starting repairs
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s is repairing damage", formatPlayerName(p)),
					"type": "info",
				},
			}
		}

		// Update position
		if p.Speed > 0 {
			// Convert speed to game units per tick
			// NOTE: Original Netrek uses WARP1=60, but we use 20 to maintain game balance
			// This difference is compensated by scaling factors elsewhere
			unitsPerTick := p.Speed * 20.0
			p.X += unitsPerTick * math.Cos(p.Dir)
			p.Y += unitsPerTick * math.Sin(p.Dir)

			// Bounce off galaxy edges
			if p.X < 0 {
				p.X = 0
				// Reverse X component of direction (bounce off left wall)
				p.Dir = math.Pi - p.Dir
				p.DesDir = p.Dir // Update desired direction to match bounced direction
			} else if p.X > game.GalaxyWidth {
				p.X = game.GalaxyWidth
				// Reverse X component of direction (bounce off right wall)
				p.Dir = math.Pi - p.Dir
				p.DesDir = p.Dir // Update desired direction to match bounced direction
			}
			if p.Y < 0 {
				p.Y = 0
				// Reverse Y component of direction (bounce off top wall)
				p.Dir = -p.Dir
				p.DesDir = p.Dir // Update desired direction to match bounced direction
			} else if p.Y > game.GalaxyHeight {
				p.Y = game.GalaxyHeight
				// Reverse Y component of direction (bounce off bottom wall)
				p.Dir = -p.Dir
				p.DesDir = p.Dir // Update desired direction to match bounced direction
			}
		}

		// Check orbit status
		if p.Orbiting >= 0 {
			// Orbit mechanics matching original Netrek
			if p.Orbiting >= len(s.gameState.Planets) {
				log.Printf("ERROR: Player %s orbiting invalid planet %d", p.Name, p.Orbiting)
				p.Orbiting = -1
				continue
			}
			planet := s.gameState.Planets[p.Orbiting]

			// Original increments direction by 2 units at 10 updates/sec (major updates)
			// where 256 units = 2*PI radians, so 2 units = 2*PI/256 = PI/128
			// Since we run at 10 FPS, we match the major update rate
			p.Dir += math.Pi / 64 // Double the speed to match original timing
			if p.Dir > 2*math.Pi {
				p.Dir -= 2 * math.Pi
			}
			p.DesDir = p.Dir

			// Calculate position from direction
			// Ship direction points tangent to orbit, so actual angle from planet is dir - PI/2
			angleFromPlanet := p.Dir - math.Pi/2
			p.X = planet.X + float64(game.OrbitDist)*math.Cos(angleFromPlanet)
			p.Y = planet.Y + float64(game.OrbitDist)*math.Sin(angleFromPlanet)
			p.Speed = 0
			p.DesSpeed = 0

			// Repair and refuel at friendly planets
			if planet.Owner == p.Team && (planet.Flags&game.PlanetRepair) != 0 {
				// Repair damage
				if p.Damage > 0 {
					p.Damage = int(math.Max(0, float64(p.Damage-2)))
				}
				// Recharge shields
				shipStats := game.ShipData[p.Ship]
				if p.Shields < shipStats.MaxShields {
					p.Shields = int(math.Min(float64(shipStats.MaxShields), float64(p.Shields+2)))
				}
			}
			if planet.Owner == p.Team && (planet.Flags&game.PlanetFuel) != 0 {
				// Refuel faster at fuel planets
				shipStats := game.ShipData[p.Ship]
				p.Fuel = int(math.Min(float64(shipStats.MaxFuel), float64(p.Fuel+50)))
			}

			// Handle planet damage to orbiting hostile ships
			// This happens every 5 frames (2 times per second at 10 FPS) matching plfight()
			if s.gameState.Frame%5 == 0 {
				if planet.Owner != p.Team && planet.Owner != game.TeamNone && planet.Armies > 0 {
					// Calculate damage: armies/10 + 2
					damage := planet.Armies/10 + 2

					// Apply damage to shields first, then hull
					if p.Shields_up && p.Shields > 0 {
						p.Shields -= damage
						if p.Shields < 0 {
							p.Damage -= p.Shields // Overflow damage goes to hull
							p.Shields = 0
						}
					} else {
						p.Damage += damage
					}

					// Check if ship destroyed by planet
					if p.Damage >= game.ShipData[p.Ship].MaxDamage {
						p.Status = game.StatusExplode
						p.ExplodeTimer = 10
						p.KilledBy = -1 // No player killer
						p.WhyDead = game.KillPlanet
						p.Bombing = false
						p.Beaming = false
						p.Orbiting = -1
						p.Deaths++ // Increment death count

						// Send death message
						s.broadcast <- ServerMessage{
							Type: MsgTypeMessage,
							Data: map[string]interface{}{
								"text": fmt.Sprintf("%s killed by %s [planet]", formatPlayerName(p), planet.Name),
								"type": "kill",
							},
						}
					}
				}
			}

			// Handle continuous bombing
			if p.Bombing && planet.Owner != p.Team {
				if planet.Armies > 0 {
					// Original Netrek bombing mechanics:
					// plfight() is called every 0.5 seconds (2 times per second)
					// 50% chance to bomb, then:
					// 60% chance: 1 army, 20% chance: 2 armies, 20% chance: 3 armies
					// This averages 1.6 armies per second

					// Only check bombing every 5 frames (2 times per second at 10 FPS)
					if s.gameState.Frame%5 == 0 {
						// Random check (50% chance to bomb)
						if rand.Float32() < 0.5 {
							// Determine number of armies to bomb
							rnd := rand.Float32()
							var killed int
							if rnd < 0.6 {
								killed = 1
							} else if rnd < 0.8 {
								killed = 2
							} else {
								killed = 3
							}

							// Assault ships get +1 bonus (if we add assault ships later)
							// if p.Ship == game.ShipAssault {
							//     killed++
							// }

							planet.Armies = game.Max(0, planet.Armies-killed)

							// If planet has no armies left, it becomes neutral and stop bombing
							if planet.Armies == 0 {
								oldOwner := planet.Owner
								planet.Owner = game.TeamNone
								p.Bombing = false
								// Send completion message
								s.broadcast <- ServerMessage{
									Type: MsgTypeMessage,
									Data: map[string]interface{}{
										"text": fmt.Sprintf("%s destroyed all armies on %s (now independent)", formatPlayerName(p), planet.Name),
										"type": "info",
									},
								}
								// Debug log
								log.Printf("Planet %s bombed to 0 armies, owner changed from %d to %d (TeamNone=%d)",
									planet.Name, oldOwner, planet.Owner, game.TeamNone)
							}
						}
					}
				} else {
					// No armies left, stop bombing
					p.Bombing = false
				}
			}

			// Handle continuous beaming
			if p.Beaming {
				// Beam armies every 0.5 seconds (5 frames at 10 FPS)
				if s.gameState.Frame%5 == 0 {
					shipStats := game.ShipData[p.Ship]

					if p.BeamingUp {
						// Beam up mode - requires 2 kills in classic Netrek
						if planet.Owner == p.Team && planet.Armies > 1 && p.Armies < shipStats.MaxArmies && p.Kills >= game.ArmyKillRequirement {
							// Beam up 1 army at a time (leave at least 1 for defense)
							p.Armies++
							planet.Armies--
						} else {
							// Can't beam up anymore (no armies, full, or not enough kills), stop
							p.Beaming = false
							p.BeamingUp = false
						}
					} else {
						// Beam down mode
						if p.Armies > 0 && (planet.Owner == p.Team || planet.Owner == game.TeamNone) {
							// Beam down 1 army at a time
							p.Armies--
							planet.Armies++

							// If beaming down to an independent planet, conquer it
							if planet.Owner == game.TeamNone {
								oldOwner := planet.Owner
								planet.Owner = p.Team

								log.Printf("Planet %s conquered by continuous beaming, owner changed from %d to %d",
									planet.Name, oldOwner, planet.Owner)
							}
						} else {
							// Can't beam down anymore, stop
							p.Beaming = false
							p.BeamingUp = false
						}
					}
				}
			}
		}

		// Update fuel and engine temperature
		fuelUsage := 0
		if p.Orbiting < 0 { // Only use fuel when not orbiting
			fuelUsage = int(p.Speed) * 2
			if p.Cloaked {
				// Use ship-specific cloak cost
				shipStats := game.ShipData[p.Ship]
				fuelUsage += shipStats.CloakCost
			}
			// Charge for shields (from original Netrek daemon.c)
			if p.Shields_up {
				switch p.Ship {
				case game.ShipScout:
					fuelUsage += 2
				case game.ShipDestroyer, game.ShipCruiser, game.ShipBattleship, game.ShipAssault:
					fuelUsage += 3
				case game.ShipStarbase:
					fuelUsage += 6
				case game.ShipGalaxy:
					fuelUsage += 3 // Using same as cruiser
				}
			}

			// Engine temperature increases with speed (from original daemon.c)
			// p_etemp += j->p_speed
			p.ETemp += int(p.Speed)
		}
		p.Fuel = int(math.Max(0, float64(p.Fuel-fuelUsage)))

		// Cap ETemp at a reasonable maximum (150% of overheat threshold)
		const maxETempCap = 1500
		if p.ETemp > maxETempCap {
			p.ETemp = maxETempCap
		}

		// Recharge fuel
		shipStats := game.ShipData[p.Ship]
		if p.Orbiting < 0 {
			// Normal fuel recharge when not orbiting
			p.Fuel = int(math.Min(float64(shipStats.MaxFuel), float64(p.Fuel+10)))
		}

		// Cool weapons and engines using ship-specific rates
		if p.WTemp > 0 {
			p.WTemp -= shipStats.WpnCool
			if p.WTemp < 0 {
				p.WTemp = 0
			}
		}
		if p.ETemp > 0 {
			p.ETemp -= shipStats.EngCool
			// Ensure it never goes below 0
			if p.ETemp < 0 {
				p.ETemp = 0
			}
		}

		// Handle engine overheat (from original daemon.c)
		// Use ship-specific max engine temp
		maxETemp := shipStats.MaxEngTemp

		if p.EngineOverheat {
			// Count down overheat timer
			p.OverheatTimer--
			if p.OverheatTimer <= 0 {
				p.EngineOverheat = false
				// Send message about engines cooling
				s.broadcast <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": fmt.Sprintf("%s's engines have cooled down", formatPlayerName(p)),
						"type": "info",
					},
				}
			}
		} else if p.ETemp > maxETemp {
			// Check for overheat - chance increases with temperature
			// At 1000: 1/40 chance, at 1500: 1/8 chance
			overheatChance := 40
			if p.ETemp > 1200 {
				overheatChance = 20
			}
			if p.ETemp > 1400 {
				overheatChance = 8
			}

			if rand.Intn(overheatChance) == 0 {
				p.EngineOverheat = true
				// Random duration between 100-250 frames (10-25 seconds at 10 FPS)
				p.OverheatTimer = rand.Intn(150) + 100
				p.DesSpeed = 0 // Stop the ship
				// Disable tractor/pressor beams
				p.Tractoring = -1
				p.Pressoring = -1

				// Send warning message
				s.broadcast <- ServerMessage{
					Type: MsgTypeMessage,
					Data: map[string]interface{}{
						"text": fmt.Sprintf("%s's engines have OVERHEATED!", formatPlayerName(p)),
						"type": "warning",
					},
				}
			}
		}

		// Handle planet damage for non-orbiting ships near hostile planets
		// This also happens every 5 frames matching plfight()
		if p.Orbiting < 0 && s.gameState.Frame%5 == 0 {
			for _, planet := range s.gameState.Planets {
				if planet == nil {
					continue
				}

				// Skip friendly or neutral planets with no armies
				if planet.Owner == p.Team || planet.Owner == game.TeamNone || planet.Armies == 0 {
					continue
				}

				// Check if within firing range
				dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
				if dist <= game.PlanetFireDist {
					// Calculate damage: armies/10 + 2
					damage := planet.Armies/10 + 2

					// Apply damage to shields first, then hull
					if p.Shields_up && p.Shields > 0 {
						p.Shields -= damage
						if p.Shields < 0 {
							p.Damage -= p.Shields // Overflow damage goes to hull
							p.Shields = 0
						}
					} else {
						p.Damage += damage
					}

					// Check if ship destroyed by planet
					if p.Damage >= game.ShipData[p.Ship].MaxDamage {
						p.Status = game.StatusExplode
						p.ExplodeTimer = 10
						p.KilledBy = -1 // No player killer
						p.WhyDead = game.KillPlanet
						p.Deaths++ // Increment death count

						// Send death message
						s.broadcast <- ServerMessage{
							Type: MsgTypeMessage,
							Data: map[string]interface{}{
								"text": fmt.Sprintf("%s killed by %s [planet]", formatPlayerName(p), planet.Name),
								"type": "kill",
							},
						}
						break // Ship is dead, no need to check other planets
					}
				}
			}
		}

		// Handle repair mode
		// Fuel recharge (always happens, faster when at a fuel planet)
		// shipStats already declared above
		if p.Fuel < shipStats.MaxFuel {
			rechargeRate := shipStats.FuelRecharge
			// Check if orbiting a fuel planet
			if p.Orbiting >= 0 {
				planet := s.gameState.Planets[p.Orbiting]
				if planet.Owner == p.Team && (planet.Flags&game.PlanetFuel) != 0 {
					rechargeRate *= 2 // Double rate at fuel planets
				}
			}
			// Apply recharge every 10 ticks to match original scale
			if s.gameState.TickCount%10 == 0 {
				p.Fuel = game.Min(p.Fuel+rechargeRate, shipStats.MaxFuel)
			}
		}

		if p.Repairing {
			// Repair only works when stopped or orbiting
			if p.Speed == 0 || p.Orbiting >= 0 {
				// Track repair progress with accumulator for fractional repairs
				p.RepairCounter++
				
				// Use ship-specific repair rate, scale down for reasonable gameplay
				// RepairRate values are 80-140, we'll divide by 8 for intervals of 10-17 ticks
				// This means repairing every 1-1.7 seconds
				repairInterval := shipStats.RepairRate / 8
				if repairInterval < 5 {
					repairInterval = 5 // Minimum interval (0.5 seconds)
				}
				
				// Check if at repair planet (halve the interval for double speed)
				if p.Orbiting >= 0 {
					planet := s.gameState.Planets[p.Orbiting]
					if planet.Owner == p.Team && (planet.Flags&game.PlanetRepair) != 0 {
						repairInterval = repairInterval / 2
						if repairInterval < 3 {
							repairInterval = 3
						}
					}
				}
				
				// Apply repairs when counter reaches interval
				if p.RepairCounter >= repairInterval {
					p.RepairCounter = 0
					
					// Repair shields by 3 points (even with shields up)
					if p.Shields < shipStats.MaxShields {
						p.Shields = game.Min(p.Shields+3, shipStats.MaxShields)
					}
					
					// Repair hull damage by 2 points (only with shields down)
					if !p.Shields_up && p.Damage > 0 {
						p.Damage = game.Max(p.Damage-2, 0)
					}
				}

				// Add small fuel consumption for repairs
				p.Fuel = game.Max(p.Fuel-1, 0)
			} else {
				// Cancel repair mode and repair request if moving while not orbiting
				p.Repairing = false
				p.RepairRequest = false
				p.RepairCounter = 0
			}
		}

		// Handle lock-on course tracking
		if p.LockType != "none" && p.LockTarget >= 0 && p.Orbiting < 0 {
			// Don't track while orbiting
			var targetX, targetY float64
			validTarget := false

			if p.LockType == "player" {
				if p.LockTarget < game.MaxPlayers {
					target := s.gameState.Players[p.LockTarget]
					if target.Status == game.StatusAlive {
						targetX = target.X
						targetY = target.Y
						validTarget = true
					}
				}
			} else if p.LockType == "planet" {
				if p.LockTarget < game.MaxPlanets {
					planet := s.gameState.Planets[p.LockTarget]
					targetX = planet.X
					targetY = planet.Y
					validTarget = true

					// Auto-orbit when close to locked planet
					dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
					if dist < 3000 && p.Speed < 4 {
						// Close enough and slow enough to orbit
						p.Orbiting = p.LockTarget
						p.Speed = 0
						p.DesSpeed = 0

						// Clear lock when entering orbit
						p.LockType = "none"
						p.LockTarget = -1

						// Send orbit confirmation
						s.broadcast <- ServerMessage{
							Type: MsgTypeMessage,
							Data: map[string]interface{}{
								"text": fmt.Sprintf("%s is orbiting %s", formatPlayerName(p), planet.Name),
								"type": "info",
							},
						}
					} else if dist > 5000 {
						// Far from planet - go fast (unless we need to turn)
						p.DesSpeed = float64(game.ShipData[p.Ship].MaxSpeed)
					} else {
						// Approaching planet - slow down based on distance
						maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)
						// Slow down from max speed to 3 as we approach from 5000 to 3000 units
						speedRatio := (dist - 3000) / 2000 // 0 to 1 as we get closer
						p.DesSpeed = 3 + (maxSpeed-3)*speedRatio
						p.DesSpeed = math.Max(3, math.Min(maxSpeed, p.DesSpeed))
					}
				}
			}

			if validTarget && p.Orbiting < 0 {
				// Update desired course toward target (ship will turn at normal rate)
				dx := targetX - p.X
				dy := targetY - p.Y
				targetDir := math.Atan2(dy, dx)

				// Calculate angle difference to target
				angleDiff := targetDir - p.Dir
				// Normalize to -PI to PI
				for angleDiff > math.Pi {
					angleDiff -= 2 * math.Pi
				}
				for angleDiff < -math.Pi {
					angleDiff += 2 * math.Pi
				}

				// If we're moving fast and need to turn significantly, slow down temporarily
				// This helps with turning since turn rate decreases exponentially with speed
				if p.LockType == "planet" && p.Speed > 6 && math.Abs(angleDiff) > math.Pi/4 {
					// Slow down to improve turning - the more we need to turn, the slower we go
					// Scale speed from 6 down to 3 based on angle (45-180 degrees)
					angleRatio := (math.Abs(angleDiff) - math.Pi/4) / (3 * math.Pi / 4) // 0 to 1
					angleRatio = math.Min(1.0, angleRatio)
					targetSpeed := 6.0 - 3.0*angleRatio // 6 to 3

					// Only reduce speed, don't increase it
					if targetSpeed < p.DesSpeed {
						p.DesSpeed = targetSpeed
					}
				}

				p.DesDir = targetDir
			} else if !validTarget {
				// Clear invalid lock
				p.LockType = "none"
				p.LockTarget = -1
			}
		}

		// Apply tractor/pressor beam physics (disabled when engines overheated, orbiting, or docked - like original)
		if (p.Tractoring >= 0 || p.Pressoring >= 0) && !p.EngineOverheat && p.Orbiting < 0 {
			var targetID int
			isPressor := false
			if p.Tractoring >= 0 {
				targetID = p.Tractoring
			} else {
				targetID = p.Pressoring
				isPressor = true
			}

			if targetID < game.MaxPlayers {
				target := s.gameState.Players[targetID]
				if target.Status == game.StatusAlive {
					dist := game.Distance(p.X, p.Y, target.X, target.Y)

					// Get ship stats for range check
					shipStats := game.ShipData[p.Ship]

					// Break beam if out of range (using ship-specific range)
					tractorRange := float64(game.TractorDist) * shipStats.TractorRange
					if dist > tractorRange {
						p.Tractoring = -1
						p.Pressoring = -1
					} else {
						// Original Netrek physics implementation from daemon.c
						targetStats := game.ShipData[target.Ship]

						// Calculate normalized direction vector (cosTheta, sinTheta in original)
						dx := target.X - p.X
						dy := target.Y - p.Y
						if dist == 0 {
							dist = 1 // prevent divide by zero
						}
						cosTheta := dx / dist
						sinTheta := dy / dist

						// Force of tractor is WARP1 * tractstr (from original code)
						// WARP1 = 20 in original Netrek
						halfforce := 20.0 * float64(shipStats.TractorStr)

						// Direction: 1 for tractor, -1 for pressor
						dir := 1.0
						if isPressor {
							dir = -1.0
						}

						// Original formula: change in position is tractor strength over mass
						// Source ship moves
						p.X += dir * cosTheta * halfforce / float64(shipStats.Mass)
						p.Y += dir * sinTheta * halfforce / float64(shipStats.Mass)

						// Target ship moves in opposite direction
						target.X -= dir * cosTheta * halfforce / float64(targetStats.Mass)
						target.Y -= dir * sinTheta * halfforce / float64(targetStats.Mass)

						// Break orbit immediately if target is orbiting (from original code)
						if target.Orbiting >= 0 {
							target.Orbiting = -1
							target.Bombing = false // Stop bombing if forced out of orbit
							target.Beaming = false // Stop beaming too
							// Send message about breaking orbit
							s.broadcast <- ServerMessage{
								Type: MsgTypeMessage,
								Data: map[string]interface{}{
									"text": fmt.Sprintf("%s was pulled out of orbit", formatPlayerName(target)),
									"type": "info",
								},
							}
						}

						// Use fuel for beam and add engine heat (from original: TRACTCOST=20, TRACTEHEAT=5)
						p.Fuel = int(math.Max(0, float64(p.Fuel-20)))
						p.ETemp += 5 // TRACTEHEAT from original
						// Cap at maximum
						if p.ETemp > 1500 {
							p.ETemp = 1500
						}
						if p.Fuel == 0 {
							p.Tractoring = -1
							p.Pressoring = -1
						}
					}
				} else {
					// Target died, release beam
					p.Tractoring = -1
					p.Pressoring = -1
				}
			}
		}

		// Cloak fuel consumption
		if p.Cloaked {
			// Cloak uses 10 fuel per frame
			p.Fuel = int(math.Max(0, float64(p.Fuel-10)))
			if p.Fuel == 0 {
				// Out of fuel, decloak
				p.Cloaked = false
			}
		}

		// Calculate alert level based on nearby enemy ships (from original daemon.c)
		// YRANGE = GWIDTH/7 = 100000/7 = ~14285
		// RRANGE = GWIDTH/10 = 100000/10 = 10000
		const YRANGE = 14285
		const RRANGE = 10000

		p.AlertLevel = "green" // Default to green

		// Check all other players for alert status
		for j := 0; j < game.MaxPlayers; j++ {
			if j == i {
				continue // Skip self
			}

			enemy := s.gameState.Players[j]

			// Skip if not alive or not at war
			if enemy.Status != game.StatusAlive {
				continue
			}

			// Check if players are at war (simplified - different teams are at war)
			if enemy.Team == p.Team {
				continue
			}

			// Calculate distance
			dx := math.Abs(p.X - enemy.X)
			dy := math.Abs(p.Y - enemy.Y)

			// Quick range check
			if dx > YRANGE || dy > YRANGE {
				continue
			}

			dist := dx*dx + dy*dy

			if dist < RRANGE*RRANGE {
				// Red alert - enemy very close
				p.AlertLevel = "red"
				break // Can't get worse than red
			} else if dist < YRANGE*YRANGE && p.AlertLevel != "red" {
				// Yellow alert - enemy moderately close
				p.AlertLevel = "yellow"
				// Don't break, keep checking for closer enemies
			}
		}
	}

	// Update torpedoes
	newTorps := make([]*game.Torpedo, 0)
	for _, torp := range s.gameState.Torps {
		// If torpedo is already exploding, remove it this frame
		if torp.Status == 3 {
			// Decrement owner's torpedo count
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Decrement fuse every tick (now running at 10 ticks/sec)
		torp.Fuse--
		if torp.Fuse <= 0 {
			// Torpedo exploded
			// Decrement owner's torpedo count
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Move torpedo
		torp.X += torp.Speed * math.Cos(torp.Dir)
		torp.Y += torp.Speed * math.Sin(torp.Dir)

		// Check if torpedo went out of bounds
		if torp.X < 0 || torp.X > game.GalaxyWidth || torp.Y < 0 || torp.Y > game.GalaxyHeight {
			// Torpedo hit galaxy edge - remove it
			if torp.Owner >= 0 && torp.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[torp.Owner]; owner != nil {
					owner.NumTorps--
				}
			}
			continue
		}

		// Check for hits
		for i := 0; i < game.MaxPlayers; i++ {
			p := s.gameState.Players[i]
			if p.Status != game.StatusAlive || p.ID == torp.Owner {
				continue
			}

			if game.Distance(torp.X, torp.Y, p.X, p.Y) < game.ExplosionDist {
				// Hit!
				p.Damage += torp.Damage
				if p.Damage >= game.ShipData[p.Ship].MaxDamage {
					// Ship destroyed!
					p.Status = game.StatusExplode
					p.ExplodeTimer = 10 // 10 frames of explosion animation
					p.KilledBy = torp.Owner
					p.WhyDead = game.KillTorp
					p.Bombing = false // Stop bombing when destroyed
					p.Beaming = false // Stop beaming when destroyed
					p.Orbiting = -1   // Break orbit when destroyed
					p.Deaths++        // Increment death count
					s.gameState.Players[torp.Owner].Kills += 1

					// Update tournament stats
					if s.gameState.T_mode {
						if stats, ok := s.gameState.TournamentStats[torp.Owner]; ok {
							stats.Kills++
							stats.DamageDealt += torp.Damage
						}
						if stats, ok := s.gameState.TournamentStats[i]; ok {
							stats.Deaths++
							stats.DamageTaken += torp.Damage
						}
					}

					// Send death message
					s.broadcastDeathMessage(p, s.gameState.Players[torp.Owner])
				}
				// Mark torpedo as exploding - it will be removed next frame
				torp.Status = 3
				break
			}
		}

		// Keep torpedo in list (even if exploding, so it shows for one frame)
		newTorps = append(newTorps, torp)
	}
	s.gameState.Torps = newTorps

	// Update plasmas
	newPlasmas := make([]*game.Plasma, 0)
	for _, plasma := range s.gameState.Plasmas {
		// Decrement fuse every tick (now running at 10 ticks/sec)
		plasma.Fuse--
		if plasma.Fuse <= 0 {
			// Plasma dissipated
			// Decrement owner's plasma count
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				s.gameState.Players[plasma.Owner].NumPlasma--
			}
			continue
		}

		// Move plasma
		plasma.X += plasma.Speed * math.Cos(plasma.Dir)
		plasma.Y += plasma.Speed * math.Sin(plasma.Dir)

		// Check if plasma went out of bounds
		if plasma.X < 0 || plasma.X > game.GalaxyWidth || plasma.Y < 0 || plasma.Y > game.GalaxyHeight {
			// Plasma hit galaxy edge - remove it
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				if owner := s.gameState.Players[plasma.Owner]; owner != nil {
					owner.NumPlasma--
				}
			}
			continue
		}

		// Check for hits
		hit := false
		explosionRadius := 1500.0 // Plasma has larger explosion radius
		for i := 0; i < game.MaxPlayers; i++ {
			p := s.gameState.Players[i]
			if p.Status != game.StatusAlive || p.ID == plasma.Owner {
				continue
			}

			if game.Distance(plasma.X, plasma.Y, p.X, p.Y) < explosionRadius {
				// Hit!
				p.Damage += plasma.Damage
				if p.Damage >= game.ShipData[p.Ship].MaxDamage {
					// Ship destroyed by plasma!
					p.Status = game.StatusExplode
					p.ExplodeTimer = 10
					p.KilledBy = plasma.Owner
					p.WhyDead = game.KillTorp // Using torp constant for now
					p.Bombing = false         // Stop bombing when destroyed
					p.Beaming = false         // Stop beaming when destroyed
					p.Orbiting = -1           // Break orbit when destroyed
					p.Deaths++                // Increment death count
					s.gameState.Players[plasma.Owner].Kills += 1

					// Update tournament stats
					if s.gameState.T_mode {
						if stats, ok := s.gameState.TournamentStats[plasma.Owner]; ok {
							stats.Kills++
							stats.DamageDealt += plasma.Damage
						}
						if stats, ok := s.gameState.TournamentStats[i]; ok {
							stats.Deaths++
							stats.DamageTaken += plasma.Damage
						}
					}

					// Send death message
					s.broadcastDeathMessage(p, s.gameState.Players[plasma.Owner])
				}
				hit = true
				break
			}
		}

		if hit {
			// Decrement owner's plasma count
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				s.gameState.Players[plasma.Owner].NumPlasma--
			}
		} else {
			newPlasmas = append(newPlasmas, plasma)
		}
	}
	s.gameState.Plasmas = newPlasmas

	// Handle planet army repopulation for AGRI planets
	// AGRI planets generate 1 army every 30 seconds (300 frames at 10 FPS)
	// Only planets with owner (not neutral) can grow armies
	if s.gameState.Frame%300 == 0 {
		for _, planet := range s.gameState.Planets {
			if planet == nil {
				continue
			}

			// Check if planet is owned and has AGRI flag
			if planet.Owner != game.TeamNone && (planet.Flags&game.PlanetAgri) != 0 {
				// Max armies on a planet is typically 40 in classic Netrek
				const maxPlanetArmies = 40
				if planet.Armies < maxPlanetArmies {
					planet.Armies++
				}
			}
		}
	}

	// Update bot AI
	s.UpdateBots()

	// Check tournament mode
	s.checkTournamentMode()

	// Check victory conditions
	s.checkVictoryConditions()
}

// checkTournamentMode checks if tournament mode should be active
func (s *Server) checkTournamentMode() {
	// Count players per team
	teamCounts := make(map[int]int)
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive && p.Connected {
			teamCounts[p.Team]++
		}
	}

	// Check if we have at least 4v4 (minimum 4 players on at least 2 teams)
	teamsWithEnough := 0
	for _, count := range teamCounts {
		if count >= 4 {
			teamsWithEnough++
		}
	}

	wasInTMode := s.gameState.T_mode
	shouldBeInTMode := teamsWithEnough >= 2

	if !wasInTMode && shouldBeInTMode {
		// Entering tournament mode
		s.gameState.T_mode = true
		s.gameState.T_start = s.gameState.Frame
		s.gameState.T_remain = 1800 // 30 minutes in seconds

		// Reset galaxy to ensure fair start
		// Re-initialize planets to startup state
		game.InitPlanets(s.gameState)
		game.InitINLPlanetFlags(s.gameState)
		
		// Clear all torpedoes and plasmas for clean start
		s.gameState.Torps = make([]*game.Torpedo, 0)
		s.gameState.Plasmas = make([]*game.Plasma, 0)

		// Reset all active players to spawn positions
		for _, p := range s.gameState.Players {
			if p.Status == game.StatusAlive && p.Connected {
				// Initialize tournament stats
				s.gameState.TournamentStats[p.ID] = &game.TournamentPlayerStats{}
				
				// Reset ship state
				shipStats := game.ShipData[p.Ship]
				p.Shields = shipStats.MaxShields
				p.Damage = 0
				p.Fuel = shipStats.MaxFuel
				p.WTemp = 0
				p.ETemp = 0
				p.Speed = 0
				p.DesSpeed = 0
				p.SubDir = 0  // Reset fractional turn accumulator
				p.AccFrac = 0 // Reset fractional acceleration accumulator
				p.Shields_up = false
				p.Cloaked = false
				p.Tractoring = -1
				p.Pressoring = -1
				p.Orbiting = -1
				p.Bombing = false
				p.Beaming = false
				p.BeamingUp = false
				p.Repairing = false
				p.RepairRequest = false
				p.RepairCounter = 0
				p.EngineOverheat = false
				p.OverheatTimer = 0
				p.Armies = 0 // Clear any armies being carried
				p.NumTorps = 0
				p.NumPlasma = 0
				
				// Reset lock-on
				p.LockType = "none"
				p.LockTarget = -1
				
				// Reset death tracking (in case they were exploding)
				p.ExplodeTimer = 0
				p.KilledBy = -1
				p.WhyDead = 0
				
				// Reset position to near home world
				homeX := float64(game.TeamHomeX[p.Team])
				homeY := float64(game.TeamHomeY[p.Team])
				
				// Add random offset to prevent ships spawning on top of each other
				offsetX := float64(rand.Intn(10000) - 5000)
				offsetY := float64(rand.Intn(10000) - 5000)
				p.X = homeX + offsetX
				p.Y = homeY + offsetY
				
				// Random starting direction
				p.Dir = rand.Float64() * 2 * math.Pi
				p.DesDir = p.Dir
				
				// Reset alert level
				p.AlertLevel = "green"
				
				// Clear bot-specific state
				if p.IsBot {
					p.BotTarget = -1
					p.BotCooldown = 0
					p.BotGoalX = 0
					p.BotGoalY = 0
				}
			}
		}

		// Announce T-mode
		s.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "⚔️ TOURNAMENT MODE ACTIVATED! 4v4 minimum reached. 30 minute time limit. Galaxy and all ships reset for fair start!",
				"type": "info",
			},
		}
	} else if wasInTMode && !shouldBeInTMode {
		// Leaving tournament mode
		s.gameState.T_mode = false

		// Announce T-mode end
		s.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Tournament mode deactivated - not enough players",
				"type": "info",
			},
		}
	}

	// Update tournament timer if in T-mode
	if s.gameState.T_mode {
		elapsedFrames := s.gameState.Frame - s.gameState.T_start
		elapsedSeconds := elapsedFrames / 10 // 10 ticks per second
		s.gameState.T_remain = 1800 - int(elapsedSeconds)

		// Check for time limit
		if s.gameState.T_remain <= 0 && !s.gameState.GameOver {
			// Time's up - determine winner by planets owned
			maxPlanets := 0
			winningTeam := 0
			for i, count := range s.gameState.TeamPlanets {
				if count > maxPlanets {
					maxPlanets = count
					winningTeam = 1 << i
				}
			}

			if winningTeam > 0 {
				s.gameState.GameOver = true
				s.gameState.Winner = winningTeam
				s.gameState.WinType = "timeout"
				s.announceVictory()
			}
		}

		// Announce time warnings
		if s.gameState.T_remain == 600 && s.gameState.Frame%10 == 0 { // 10 minutes
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 10 minutes remaining in tournament!",
					"type": "warning",
				},
			}
		} else if s.gameState.T_remain == 300 && s.gameState.Frame%10 == 0 { // 5 minutes
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 5 minutes remaining in tournament!",
					"type": "warning",
				},
			}
		} else if s.gameState.T_remain == 60 && s.gameState.Frame%10 == 0 { // 1 minute
			s.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": "⏰ 1 minute remaining in tournament!",
					"type": "warning",
				},
			}
		}
	}
}

// checkVictoryConditions checks for genocide or conquest victory
func (s *Server) checkVictoryConditions() {
	if s.gameState.GameOver {
		return // Game already over
	}

	// Count active players and planets per team
	for i := range s.gameState.TeamPlayers {
		s.gameState.TeamPlayers[i] = 0
		s.gameState.TeamPlanets[i] = 0
	}

	// Count active players per team
	for _, p := range s.gameState.Players {
		if p.Status == game.StatusAlive {
			switch p.Team {
			case game.TeamFed:
				s.gameState.TeamPlayers[0]++
			case game.TeamRom:
				s.gameState.TeamPlayers[1]++
			case game.TeamKli:
				s.gameState.TeamPlayers[2]++
			case game.TeamOri:
				s.gameState.TeamPlayers[3]++
			}
		}
	}

	// Count planets per team
	for _, planet := range s.gameState.Planets {
		switch planet.Owner {
		case game.TeamFed:
			s.gameState.TeamPlanets[0]++
		case game.TeamRom:
			s.gameState.TeamPlanets[1]++
		case game.TeamKli:
			s.gameState.TeamPlanets[2]++
		case game.TeamOri:
			s.gameState.TeamPlanets[3]++
		}
	}

	// Check for genocide (all players of other teams eliminated)
	// But require that multiple teams were playing (had players at some point)
	totalPlayers := 0
	teamsAlive := 0
	lastTeamAlive := -1
	teamsEverPlayed := 0 // Track how many teams have ever had players

	// Check current players
	for i, count := range s.gameState.TeamPlayers {
		totalPlayers += count
		if count > 0 {
			teamsAlive++
			lastTeamAlive = 1 << i // Convert to team flag (1, 2, 4, 8)
		}
	}

	// Count how many teams have ever had players in this game
	for _, p := range s.gameState.Players {
		if p.Status != game.StatusFree && p.Team > 0 {
			// This player slot was used by a team
			switch p.Team {
			case game.TeamFed:
				teamsEverPlayed |= 1
			case game.TeamRom:
				teamsEverPlayed |= 2
			case game.TeamKli:
				teamsEverPlayed |= 4
			case game.TeamOri:
				teamsEverPlayed |= 8
			}
		}
	}

	// Count bits set in teamsEverPlayed to get number of teams that played
	numTeamsPlayed := 0
	for i := 0; i < 4; i++ {
		if (teamsEverPlayed>>i)&1 == 1 {
			numTeamsPlayed++
		}
	}

	// Only check for genocide if:
	// - At least 2 different teams have played
	// - Game has been running for a bit
	// - Only one team remains alive
	// - At least 2 total players currently
	if numTeamsPlayed >= 2 && totalPlayers >= 2 && s.gameState.Frame > 100 && teamsAlive == 1 && lastTeamAlive > 0 {
		// Genocide victory
		s.gameState.GameOver = true
		s.gameState.Winner = lastTeamAlive
		s.gameState.WinType = "genocide"
		s.announceVictory()
		return
	}

	// Check for conquest (one team owns all planets)
	// Also require multiple players for conquest victory
	if totalPlayers >= 2 && s.gameState.Frame > 100 {
		for i, count := range s.gameState.TeamPlanets {
			if count == game.MaxPlanets {
				// Conquest victory
				s.gameState.GameOver = true
				s.gameState.Winner = 1 << i // Convert to team flag
				s.gameState.WinType = "conquest"
				s.announceVictory()
				return
			}
		}
	}
}

// announceVictory sends victory message to all clients
func (s *Server) announceVictory() {
	teamName := ""
	switch s.gameState.Winner {
	case game.TeamFed:
		teamName = "Federation"
	case game.TeamRom:
		teamName = "Romulan"
	case game.TeamKli:
		teamName = "Klingon"
	case game.TeamOri:
		teamName = "Orion"
	}

	var message string
	if s.gameState.WinType == "genocide" {
		message = fmt.Sprintf("🎉 GENOCIDE! %s team has eliminated all enemies! Victory!", teamName)
	} else if s.gameState.WinType == "conquest" {
		message = fmt.Sprintf("🎉 CONQUEST! %s team has captured all planets! Victory!", teamName)
	} else if s.gameState.WinType == "timeout" {
		message = fmt.Sprintf("⏱️ TIME LIMIT! %s team wins by controlling the most planets!", teamName)
	}

	// Broadcast victory message
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text":     message,
			"type":     "victory",
			"winner":   s.gameState.Winner,
			"win_type": s.gameState.WinType,
		},
	}

	// Schedule game reset after 10 seconds
	go func() {
		time.Sleep(10 * time.Second)
		s.resetGame()
	}()
}

// resetGame resets the game state for a new round
func (s *Server) resetGame() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create new game state
	newState := game.NewGameState()

	// Preserve connected players but reset their status
	for i, p := range s.gameState.Players {
		if p.Connected {
			// For bots, disconnect them when game resets
			if p.IsBot {
				// Mark bot as disconnected so slot becomes free
				newState.Players[i].Status = game.StatusFree
				newState.Players[i].Connected = false
				newState.Players[i].IsBot = false
			} else {
				// For human players, preserve connection
				newState.Players[i] = &game.Player{
					ID:         i,
					Name:       p.Name,
					Team:       p.Team,
					Ship:       p.Ship,
					Status:     game.StatusOutfit,
					Connected:  true,
					Tractoring: -1,
					Pressoring: -1,
					Orbiting:   -1,
				}
				// Set initial position and stats
				shipStats := game.ShipData[p.Ship]
				newState.Players[i].X = float64(game.TeamHomeX[p.Team]) + float64(i%4)*1000
				newState.Players[i].Y = float64(game.TeamHomeY[p.Team]) + float64(i/4)*1000
				newState.Players[i].Shields = shipStats.MaxShields
				newState.Players[i].Fuel = shipStats.MaxFuel
			}
		}
	}

	s.gameState = newState

	// Announce game reset
	s.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": "Game has been reset! New round starting...",
			"type": "info",
		},
	}
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
