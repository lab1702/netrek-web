package server

import (
	"encoding/json"
	"fmt"
	"github.com/lab1702/netrek-web/game"
	"html"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// sanitizeText escapes HTML special characters to prevent XSS
func sanitizeText(text string) string {
	// Limit message length
	const maxMessageLength = 500
	if len(text) > maxMessageLength {
		text = text[:maxMessageLength]
	}
	// html.EscapeString escapes <, >, &, ' and "
	return html.EscapeString(text)
}

// sanitizeName removes non-alphanumeric characters and escapes HTML
func sanitizeName(name string) string {
	// Limit name length
	const maxNameLength = 20
	if len(name) > maxNameLength {
		name = name[:maxNameLength]
	}

	// Remove non-alphanumeric characters
	cleaned := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, name)

	// Also escape HTML just in case
	return html.EscapeString(cleaned)
}

// validateDirection ensures direction is within valid range [0, 2*pi]
func validateDirection(dir float64) float64 {
	// Normalize to [0, 2*pi]
	for dir < 0 {
		dir += 2 * math.Pi
	}
	for dir >= 2*math.Pi {
		dir -= 2 * math.Pi
	}
	return dir
}

// validateTeam ensures team is valid
func validateTeam(team int) bool {
	return team == game.TeamFed || team == game.TeamRom ||
		team == game.TeamKli || team == game.TeamOri
}

// validateShipType ensures ship type is valid
func validateShipType(ship game.ShipType) bool {
	return ship >= 0 && ship < game.ShipType(len(game.ShipData))
}

// LoginData represents login request data
type LoginData struct {
	Name string        `json:"name"`
	Team int           `json:"team"`
	Ship game.ShipType `json:"ship"`
}

// MoveData represents movement commands
type MoveData struct {
	Dir   float64 `json:"dir"`   // Direction in radians
	Speed float64 `json:"speed"` // Desired speed
}

// FireData represents torpedo fire command
type FireData struct {
	Dir float64 `json:"dir"` // Direction to fire
}

// PhaserData represents phaser fire command
type PhaserData struct {
	Target int     `json:"target"` // Target player ID (-1 for direction)
	Dir    float64 `json:"dir"`    // Direction if no target
}

// handleLogin processes login requests
func (c *Client) handleLogin(data json.RawMessage) {
	var loginData LoginData
	if err := json.Unmarshal(data, &loginData); err != nil {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid login data",
		}
		return
	}

	// Validate team and ship type
	if !validateTeam(loginData.Team) {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid team selection",
		}
		return
	}

	if !validateShipType(loginData.Ship) {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Invalid ship type",
		}
		return
	}

	// Sanitize the player name to prevent XSS
	loginData.Name = sanitizeName(loginData.Name)

	// Ensure name is not empty after sanitization
	if loginData.Name == "" {
		loginData.Name = fmt.Sprintf("Player%d", rand.Intn(1000))
	}

	// Find a player slot - check for reconnection first
	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	playerID := -1

	// First check if this player is reconnecting (same name, team, and ship)
	for i := 0; i < game.MaxPlayers; i++ {
		p := c.server.gameState.Players[i]
		if p.Status == game.StatusAlive && !p.Connected &&
			p.Name == loginData.Name &&
			p.Team == loginData.Team &&
			p.Ship == loginData.Ship {
			// Found their old slot - reconnect them
			playerID = i
			log.Printf("Player %s reconnecting to slot %d", loginData.Name, i)
			break
		}
	}

	// If not reconnecting, check team balance
	if playerID == -1 {
		// Count players per team (only count connected, alive players)
		teamCounts := make(map[int]int)
		// Initialize all teams to 0
		teamCounts[game.TeamFed] = 0
		teamCounts[game.TeamRom] = 0
		teamCounts[game.TeamKli] = 0
		teamCounts[game.TeamOri] = 0

		for _, p := range c.server.gameState.Players {
			if p.Status == game.StatusAlive && p.Connected {
				teamCounts[p.Team]++
			}
		}

		// Find the maximum team size
		maxCount := 0
		for _, count := range teamCounts {
			if count > maxCount {
				maxCount = count
			}
		}

		// Check if the requested team would have more players than others after joining
		requestedTeamCount := teamCounts[loginData.Team]

		// If this team already has the max number of players and at least one other team has fewer
		if requestedTeamCount >= maxCount && maxCount > 0 {
			// Check if at least one other team has fewer players
			hasFewerTeam := false
			for team, count := range teamCounts {
				if team != loginData.Team && count < requestedTeamCount {
					hasFewerTeam = true
					break
				}
			}

			if hasFewerTeam {
				// Reject - this team already has the most players
				log.Printf("Team balance enforced: Player %s denied joining team %d (would have %d players, other teams have fewer)",
					loginData.Name, loginData.Team, requestedTeamCount+1)
				c.send <- ServerMessage{
					Type: MsgTypeError,
					Data: "Team is full. Please join a team with fewer players for balance.",
				}
				return
			}
		}

		// Log successful team join (show counts BEFORE this player joins)
		log.Printf("Player %s joining team %d (current counts before join: Fed=%d, Rom=%d, Kli=%d, Ori=%d)",
			loginData.Name, loginData.Team,
			teamCounts[game.TeamFed], teamCounts[game.TeamRom],
			teamCounts[game.TeamKli], teamCounts[game.TeamOri])
	}

	// If not reconnecting, find a free slot
	if playerID == -1 {
		for i := 0; i < game.MaxPlayers; i++ {
			if c.server.gameState.Players[i].Status == game.StatusFree {
				playerID = i
				break
			}
		}
	}

	if playerID == -1 {
		c.send <- ServerMessage{
			Type: MsgTypeError,
			Data: "Server full",
		}
		return
	}

	// Set up the player (use pointer to modify in place)
	p := c.server.gameState.Players[playerID]

	// Check if this is a reconnection or new player
	isReconnecting := p.Status == game.StatusAlive && !p.Connected &&
		p.Name == loginData.Name

	if isReconnecting {
		// Reconnecting - just restore connection
		p.Connected = true
		// Clear the disconnect timestamp
		p.LastUpdate = time.Time{}
		// Keep all existing state (position, damage, fuel, etc.)
	} else {
		// New player - initialize everything
		p.Name = loginData.Name
		p.Team = loginData.Team
		p.Ship = loginData.Ship
		p.Status = game.StatusAlive

		// Set starting position near home planet with random offset (like original Netrek)
		var homeX, homeY float64
		switch loginData.Team {
		case game.TeamFed:
			homeX, homeY = 20000, 80000 // Earth
		case game.TeamRom:
			homeX, homeY = 20000, 20000 // Romulus
		case game.TeamKli:
			homeX, homeY = 80000, 20000 // Klingus
		case game.TeamOri:
			homeX, homeY = 80000, 80000 // Orion
		default:
			homeX, homeY = 50000, 50000 // Center
		}

		// Add random offset between -5000 and +5000 (from original: random() % 10000 - 5000)
		offsetX := float64(rand.Intn(10000) - 5000)
		offsetY := float64(rand.Intn(10000) - 5000)
		p.X = homeX + offsetX
		p.Y = homeY + offsetY

		// Initialize ship stats
		shipStats := game.ShipData[loginData.Ship]
		p.Shields = shipStats.MaxShields
		p.Damage = 0
		p.Fuel = shipStats.MaxFuel
		p.Armies = 0
		p.WTemp = 0
		p.ETemp = 0
		p.Dir = 0
		p.Speed = 0
		p.DesDir = 0
		p.DesSpeed = 0
		p.Connected = true
		p.Shields_up = false // Shields DOWN by default
	}

	c.PlayerID = playerID

	// Send success response
	c.send <- ServerMessage{
		Type: "login_success",
		Data: map[string]interface{}{
			"player_id": playerID,
			"team":      loginData.Team,
			"ship":      loginData.Ship,
		},
	}

	shipData := game.ShipData[p.Ship]
	if isReconnecting {
		log.Printf("Player %s reconnected as %s on team %d", loginData.Name, shipData.Name, loginData.Team)
	} else {
		log.Printf("Player %s joined as %s on team %d", loginData.Name, shipData.Name, loginData.Team)
	}
}

// handleMove processes movement commands
func (c *Client) handleMove(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var moveData MoveData
	if err := json.Unmarshal(data, &moveData); err != nil {
		return
	}

	// Validate direction
	moveData.Dir = validateDirection(moveData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Break orbit if setting new course
	if p.Orbiting >= 0 {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when leaving orbit
		// Send message about breaking orbit
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s has left orbit", formatPlayerName(p)),
				"type": "info",
			},
		}
	}

	// Set desired direction and speed
	p.DesDir = game.NormalizeAngle(moveData.Dir)

	// Clear lock when manually setting course
	p.LockType = "none"
	p.LockTarget = -1

	// Cancel repair mode and repair request when setting speed > 0 (unless orbiting)
	if moveData.Speed > 0 && p.Orbiting < 0 {
		if p.RepairRequest {
			p.RepairRequest = false
			// Send message about canceling repair request
			c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s canceled repair request", formatPlayerName(p)),
					"type": "info",
				},
			}
		}
		p.Repairing = false
	}

	// Calculate max speed based on damage
	shipStats := game.ShipData[p.Ship]
	maxSpeed := float64(shipStats.MaxSpeed)
	if p.Damage > 0 {
		// Formula from original Netrek: maxspeed = (max + 2) - (max + 1) * (damage / maxdamage)
		damageRatio := float64(p.Damage) / float64(shipStats.MaxDamage)
		maxSpeed = float64(shipStats.MaxSpeed+2) - float64(shipStats.MaxSpeed+1)*damageRatio
		maxSpeed = math.Max(1, maxSpeed) // Minimum speed of 1
	}

	// Clamp speed to damage-adjusted maximum
	p.DesSpeed = math.Max(0, math.Min(moveData.Speed, maxSpeed))
}

// handleFire processes torpedo fire commands
func (c *Client) handleFire(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var fireData FireData
	if err := json.Unmarshal(data, &fireData); err != nil {
		return
	}

	// Validate direction
	fireData.Dir = validateDirection(fireData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	// Check if can fire torpedo
	if p.NumTorps >= game.MaxTorps {
		return // Too many torps out
	}

	shipStats := game.ShipData[p.Ship]

	// Check fuel (using ship-specific multiplier)
	torpCost := shipStats.TorpDamage * shipStats.TorpFuelMult
	if p.Fuel < torpCost {
		return // Not enough fuel
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return // Weapons too hot
	}

	// Fire torpedo
	torp := &game.Torpedo{
		ID:     len(c.server.gameState.Torps),
		Owner:  c.PlayerID,
		X:      p.X,
		Y:      p.Y,
		Dir:    fireData.Dir,
		Speed:  float64(shipStats.TorpSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.TorpDamage,
		Fuse:   shipStats.TorpFuse, // Use ship-specific torpedo fuse
		Status: 1,                  // Moving
		Team:   p.Team,
	}

	c.server.gameState.Torps = append(c.server.gameState.Torps, torp)
	p.NumTorps++
	p.Fuel -= torpCost
	p.WTemp += 50
}

// handlePhaser processes phaser fire commands (using original Netrek algorithm)
func (c *Client) handlePhaser(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var phaserData PhaserData
	if err := json.Unmarshal(data, &phaserData); err != nil {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p == nil || p.Status != game.StatusAlive {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	shipStats := game.ShipData[p.Ship]

	// Check fuel (using ship-specific multiplier)
	phaserCost := shipStats.PhaserDamage * shipStats.PhaserFuelMult
	if p.Fuel < phaserCost {
		return
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return
	}

	// Consume fuel and increase weapon temp regardless of hit
	p.Fuel -= phaserCost
	p.WTemp += 70

	// Calculate phaser range using original formula: PHASEDIST * phaserdamage / 100
	myPhaserRange := float64(game.PhaserDist * shipStats.PhaserDamage / 100)

	// Get phaser direction (use provided direction or calculate from target)
	var course float64
	if phaserData.Target >= 0 && phaserData.Target < game.MaxPlayers {
		// Calculate direction to specific target
		targetPlayer := c.server.gameState.Players[phaserData.Target]
		if targetPlayer != nil && targetPlayer.Status == game.StatusAlive {
			course = math.Atan2(targetPlayer.Y-p.Y, targetPlayer.X-p.X)
		} else {
			return // Invalid target
		}
	} else {
		// Use the provided direction (can be any angle from -π to π)
		course = phaserData.Dir
	}

	// (C, D) is a point on the phaser line, relative to me
	// Using 10*PHASEDIST like original to prevent round-off errors
	C := math.Cos(course) * 10 * float64(game.PhaserDist)
	D := math.Sin(course) * 10 * float64(game.PhaserDist)

	// Initialize search parameters
	var target *game.Player
	var targetDist float64
	rangeSq := myPhaserRange*myPhaserRange + 1 // +1 to ensure we check exact range

	// Check all enemy players using the original line-to-circle algorithm
	for _, enemy := range c.server.gameState.Players {
		if enemy == nil || enemy.Status != game.StatusAlive || enemy.Team == p.Team {
			continue
		}

		// (A, B) is the position of the possible target relative to me
		A := enemy.X - p.X
		B := enemy.Y - p.Y

		// Quick bounds check
		if math.Abs(A) >= myPhaserRange || math.Abs(B) >= myPhaserRange {
			continue
		}

		// Check if within phaser range
		thisRangeSq := A*A + B*B
		if thisRangeSq >= rangeSq {
			continue
		}

		// Calculate point on phaser line nearest to target
		// s is the parameter for the point on the line closest to the target
		s := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))

		if s < 0 {
			s = 0 // Handle case where target is behind the ship
		}

		E := C * s
		F := D * s

		// Check if the closest point on the phaser line is within hit distance
		dx := E - A
		dy := F - B

		// Use ZAPPLAYERDIST for hit detection
		if dx*dx+dy*dy <= float64(game.ZAPPLAYERDIST*game.ZAPPLAYERDIST) {
			// A hit! Update if this is closer than previous target
			if target == nil || thisRangeSq < targetDist*targetDist {
				target = enemy
				targetDist = math.Sqrt(thisRangeSq)
				rangeSq = thisRangeSq // Narrow search to closer targets
			}
		}
	}

	// Check plasma torpedoes (if they exist)
	for _, plasma := range c.server.gameState.Plasmas {
		if plasma == nil || plasma.Status != 1 || plasma.Owner == c.PlayerID {
			continue
		}

		// Check if plasma is enemy
		if plasma.Team == p.Team {
			continue
		}

		A := plasma.X - p.X
		B := plasma.Y - p.Y

		if math.Abs(A) >= myPhaserRange || math.Abs(B) >= myPhaserRange {
			continue
		}

		thisRangeSq := A*A + B*B
		if thisRangeSq >= rangeSq {
			continue
		}

		s := (A*C + B*D) / (10.0 * float64(game.PhaserDist) * 10.0 * float64(game.PhaserDist))
		if s < 0 {
			continue
		}

		E := C * s
		F := D * s
		dx := E - A
		dy := F - B

		// Use ZAPPLASMADIST for plasma hit detection
		if dx*dx+dy*dy <= float64(game.ZAPPLASMADIST*game.ZAPPLASMADIST) {
			// Destroy the plasma
			plasma.Status = 3 // Detonate

			// Update plasma count for owner
			if plasma.Owner >= 0 && plasma.Owner < game.MaxPlayers {
				if owner := c.server.gameState.Players[plasma.Owner]; owner != nil {
					owner.NumPlasma--
				}
			}

			log.Printf("Phaser destroyed plasma: player %d destroyed plasma from player %d", c.PlayerID, plasma.Owner)

			// Send phaser visual to plasma location
			c.server.broadcast <- ServerMessage{
				Type: "phaser",
				Data: map[string]interface{}{
					"from": c.PlayerID,
					"to":   -2, // Special code for plasma hit
					"x":    plasma.X,
					"y":    plasma.Y,
				},
			}
			return // Plasma takes priority if hit
		}
	}

	// Fire at target if found
	if target != nil {
		// Calculate damage based on distance using original formula
		damage := float64(shipStats.PhaserDamage) * (1.0 - targetDist/myPhaserRange)
		log.Printf("Phaser hit: player %d hit player %d for %.1f damage at range %.0f", c.PlayerID, target.ID, damage, targetDist)

		if target.Shields_up && target.Shields > 0 {
			// Damage shields first
			shieldDamage := int(math.Min(float64(target.Shields), damage))
			target.Shields -= shieldDamage
			damage -= float64(shieldDamage)
		}

		if damage > 0 {
			target.Damage += int(damage)
			if target.Damage >= game.ShipData[target.Ship].MaxDamage {
				// Ship destroyed by phaser!
				target.Status = game.StatusExplode
				target.ExplodeTimer = 10
				target.KilledBy = c.PlayerID
				target.WhyDead = game.KillPhaser
				target.Bombing = false // Stop bombing when destroyed
				target.Orbiting = -1   // Break orbit when destroyed
				target.Deaths++        // Increment death count
				p.Kills += 1

				// Update tournament stats
				if c.server.gameState.T_mode {
					if stats, ok := c.server.gameState.TournamentStats[c.PlayerID]; ok {
						stats.Kills++
						stats.DamageDealt += int(damage)
					}
					if stats, ok := c.server.gameState.TournamentStats[target.ID]; ok {
						stats.Deaths++
						stats.DamageTaken += int(damage)
					}
				}

				// Send death message
				c.server.broadcastDeathMessage(target, p)
			}

			// Send phaser visual
			c.server.broadcast <- ServerMessage{
				Type: "phaser",
				Data: map[string]interface{}{
					"from": c.PlayerID,
					"to":   target.ID,
				},
			}
		}
	} else {
		// No target - phaser fires but misses
		// Send phaser visual with direction but no target
		c.server.broadcast <- ServerMessage{
			Type: "phaser",
			Data: map[string]interface{}{
				"from": c.PlayerID,
				"to":   -1,     // -1 indicates no target
				"dir":  course, // Direction the phaser was fired
			},
		}
	}
}

// handleShields toggles shields
func (c *Client) handleShields(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Toggle shields
	p.Shields_up = !p.Shields_up

	// Raising shields cancels repair mode and repair request
	if p.Shields_up {
		if p.RepairRequest {
			p.RepairRequest = false
		}
		if p.Repairing {
			p.Repairing = false
		}
	}
}

// handleRepair toggles repair mode
func (c *Client) handleRepair(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	if !p.Repairing && !p.RepairRequest {
		// If moving while not orbiting, set repair request and slow down
		if p.Speed > 0 && p.Orbiting < 0 {
			p.RepairRequest = true
			p.DesSpeed = 0 // Start slowing down
			// Send message about slowing to repair
			c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s is slowing to repair", formatPlayerName(p)),
					"type": "info",
				},
			}
			return
		}

		// If stopped or orbiting, enter repair mode immediately
		p.Repairing = true
		p.DesSpeed = 0       // Stop the ship
		p.Shields_up = false // Lower shields
		// Cancel any locks, beaming, bombing
		p.Bombing = false
		p.Beaming = false
	} else if p.RepairRequest {
		// Cancel repair request
		p.RepairRequest = false
		// Send message about canceling repair
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s canceled repair request", p.Name),
				"type": "info",
			},
		}
	} else {
		// Exit repair mode
		p.Repairing = false
	}
}

// handleLock handles lock-on to players or planets
func (c *Client) handleLock(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var lockData struct {
		Type   string `json:"type"`   // "player" or "planet"
		Target int    `json:"target"` // Target ID
	}
	if err := json.Unmarshal(data, &lockData); err != nil {
		return
	}

	// Validate lock target type
	if lockData.Type != "player" && lockData.Type != "planet" {
		return
	}

	// Validate target ID based on type
	if lockData.Type == "player" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlayers {
			return
		}
	} else if lockData.Type == "planet" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlanets {
			return
		}
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Break orbit when locking onto a new target (unless locking the planet we're orbiting)
	if p.Orbiting >= 0 && (lockData.Type != "planet" || lockData.Target != p.Orbiting) {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when leaving orbit
		p.Beaming = false // Stop beaming when leaving orbit
		// Send message about breaking orbit
		c.server.broadcast <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s has left orbit", formatPlayerName(p)),
				"type": "info",
			},
		}
	}

	// Validate target and set course
	if lockData.Type == "player" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlayers {
			return
		}
		target := c.server.gameState.Players[lockData.Target]
		if target.Status != game.StatusAlive {
			return
		}
		p.LockType = "player"
		p.LockTarget = lockData.Target

		// Set desired course toward target (ship will turn at normal rate)
		dx := target.X - p.X
		dy := target.Y - p.Y
		p.DesDir = math.Atan2(dy, dx)
	} else if lockData.Type == "planet" {
		if lockData.Target < 0 || lockData.Target >= game.MaxPlanets {
			return
		}
		planet := c.server.gameState.Planets[lockData.Target]
		p.LockType = "planet"
		p.LockTarget = lockData.Target

		// Set desired course toward planet (ship will turn at normal rate)
		dx := planet.X - p.X
		dy := planet.Y - p.Y
		p.DesDir = math.Atan2(dy, dx)
	} else if lockData.Type == "none" {
		// Clear lock
		p.LockType = "none"
		p.LockTarget = -1
	}
}

// handleOrbit toggles orbit around nearest planet
func (c *Client) handleOrbit(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// If already orbiting, break orbit
	if p.Orbiting >= 0 {
		p.Orbiting = -1
		p.Bombing = false // Stop bombing when breaking orbit
		p.Beaming = false // Stop beaming when breaking orbit
		return
	}

	// Check if going slow enough to orbit (max warp 2)
	if p.Speed > float64(game.ORBSPEED) {
		// Too fast to orbit - silently fail like original
		return
	}

	// Find nearest planet within orbit entry distance
	closestPlanet := -1
	closestDist := float64(game.EntOrbitDist)

	for i := 0; i < game.MaxPlanets; i++ {
		planet := c.server.gameState.Planets[i]
		dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
		if dist <= closestDist {
			closestDist = dist
			closestPlanet = i
		}
	}

	if closestPlanet >= 0 {
		p.Orbiting = closestPlanet
		p.Speed = 0
		p.DesSpeed = 0

		// Clear lock when entering orbit
		p.LockType = "none"
		p.LockTarget = -1

		// Clear tractor/pressor beams when entering orbit (from original)
		p.Tractoring = -1
		p.Pressoring = -1

		// Calculate initial orbit position at correct radius
		planet := c.server.gameState.Planets[closestPlanet]
		angle := math.Atan2(p.Y-planet.Y, p.X-planet.X)
		p.X = planet.X + float64(game.OrbitDist)*math.Cos(angle)
		p.Y = planet.Y + float64(game.OrbitDist)*math.Sin(angle)

		// Set direction tangent to orbit (perpendicular to radius)
		// In original: dir + 64 where 256 units = 2*PI, so 64 = PI/2
		p.Dir = angle + math.Pi/2
		p.DesDir = p.Dir
	}
}

// BeamData represents army beam request
type BeamData struct {
	Up bool `json:"up"` // true = beam up, false = beam down
}

// handleBeam handles army beaming
func (c *Client) handleBeam(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var beamData BeamData
	if err := json.Unmarshal(data, &beamData); err != nil {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive || p.Orbiting < 0 {
		return
	}

	planet := c.server.gameState.Planets[p.Orbiting]
	shipStats := game.ShipData[p.Ship]

	if beamData.Up {
		// Toggle beam up mode or turn it off if already beaming up
		if p.Beaming && p.BeamingUp {
			// Already beaming up, turn it off
			p.Beaming = false
			p.BeamingUp = false
		} else {
			// Start beaming up (only if planet has armies and is friendly)
			// Must leave at least 1 army on the planet
			// Classic Netrek requires 2 kills to pick up armies
			if planet.Owner == p.Team && planet.Armies > 1 && p.Armies < shipStats.MaxArmies {
				if p.Kills >= game.ArmyKillRequirement {
					p.Beaming = true
					p.BeamingUp = true
				} else {
					// Send message about needing kills
					errorMsg := ServerMessage{
						Type: MsgTypeMessage,
						Data: map[string]interface{}{
							"text": fmt.Sprintf("You need %.1f more kills to pick up armies", game.ArmyKillRequirement-p.Kills),
							"type": "error",
						},
					}
					select {
					case c.send <- errorMsg:
					default:
						// Client's send channel is full, skip
					}
				}
			}
		}
	} else {
		// Toggle beam down mode or turn it off if already beaming down
		if p.Beaming && !p.BeamingUp {
			// Already beaming down, turn it off
			p.Beaming = false
			p.BeamingUp = false
		} else {
			// Start beaming down (only if we have armies and planet is friendly or independent)
			if p.Armies > 0 && (planet.Owner == p.Team || planet.Owner == game.TeamNone) {
				p.Beaming = true
				p.BeamingUp = false
			}
		}
	}
}

// handleBomb handles planet bombing
func (c *Client) handleBomb(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive || p.Orbiting < 0 {
		return
	}

	planet := c.server.gameState.Planets[p.Orbiting]

	// Can only bomb enemy or independent planets
	if planet.Owner != p.Team {
		// Toggle bombing state
		p.Bombing = !p.Bombing
		if p.Bombing && planet.Armies > 0 {
			// Send message about starting bombing
			c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s is bombing %s", formatPlayerName(p), planet.Name),
					"type": "info",
				},
			}
		} else if !p.Bombing {
			// Send message about stopping bombing
			c.server.broadcast <- ServerMessage{
				Type: MsgTypeMessage,
				Data: map[string]interface{}{
					"text": fmt.Sprintf("%s stopped bombing %s", formatPlayerName(p), planet.Name),
					"type": "info",
				},
			}
		}
	}
}

// PlasmaData represents plasma fire command
type PlasmaData struct {
	Dir float64 `json:"dir"` // Direction to fire
}

// handlePlasma processes plasma torpedo fire commands
func (c *Client) handlePlasma(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var plasmaData PlasmaData
	if err := json.Unmarshal(data, &plasmaData); err != nil {
		return
	}

	// Validate direction
	plasmaData.Dir = validateDirection(plasmaData.Dir)

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't fire while cloaked or repairing
	if p.Cloaked || p.Repairing {
		return
	}

	// Check if ship has plasma capability
	shipStats := game.ShipData[p.Ship]
	if !shipStats.HasPlasma {
		return // Ship can't fire plasma
	}

	// Check if can fire plasma (only 1 at a time)
	if p.NumPlasma >= game.MaxPlasma {
		return // Already have plasma out
	}

	// Check fuel (using ship-specific multiplier)
	plasmaCost := shipStats.PlasmaDamage * shipStats.PlasmaFuelMult
	if p.Fuel < plasmaCost {
		return // Not enough fuel
	}

	// Check weapon temperature against ship-specific limit
	if p.WTemp > shipStats.MaxWpnTemp-100 {
		return // Weapons too hot
	}

	// Fire plasma torpedo
	plasma := &game.Plasma{
		ID:     len(c.server.gameState.Plasmas),
		Owner:  c.PlayerID,
		X:      p.X,
		Y:      p.Y,
		Dir:    plasmaData.Dir,
		Speed:  float64(shipStats.PlasmaSpeed * 20), // Warp speed: 20 units per tick at 10 ticks/sec
		Damage: shipStats.PlasmaDamage,
		Fuse:   shipStats.PlasmaFuse, // Use original fuse value directly (already scaled for our 10 FPS)
		Status: 1,                    // Moving
		Team:   p.Team,
	}

	c.server.gameState.Plasmas = append(c.server.gameState.Plasmas, plasma)
	p.NumPlasma++
	p.Fuel -= plasmaCost
	p.WTemp += 100 // Plasma heats weapons more
}

// handleDetonate handles detonating own torpedoes
func (c *Client) handleDetonate(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't detonate while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for detonate cost
	shipStats := game.ShipData[p.Ship]

	// Find and detonate all torpedoes owned by this player
	detonatedCount := 0
	for _, torp := range c.server.gameState.Torps {
		if torp.Owner == c.PlayerID {
			// Check if we have enough fuel
			if p.Fuel < shipStats.DetCost {
				// Not enough fuel to detonate
				c.server.broadcast <- ServerMessage{
					Type: "message",
					Data: map[string]interface{}{
						"text": "Not enough fuel to detonate",
						"type": "error",
						"to":   c.PlayerID, // Send only to this player
					},
				}
				break
			}
			// Set fuse to 1 so it will explode next frame
			torp.Fuse = 1
			detonatedCount++
			// Deduct fuel cost
			p.Fuel -= shipStats.DetCost
		}
	}

	// Send feedback message
	if detonatedCount > 0 {
		c.server.broadcast <- ServerMessage{
			Type: "message",
			Data: map[string]interface{}{
				"text": fmt.Sprintf("%s detonated %d torpedo(es)", formatPlayerName(p), detonatedCount),
				"type": "info",
			},
		}
	}
}

// handleTractor handles tractor beam engagement
func (c *Client) handleTractor(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var tractorData struct {
		TargetID int `json:"targetId"`
	}
	if err := json.Unmarshal(data, &tractorData); err != nil {
		log.Printf("Error unmarshaling tractor data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't use tractor while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for range calculation
	shipStats := game.ShipData[p.Ship]

	// Clear pressor if engaging tractor
	p.Pressoring = -1

	// Toggle tractor beam
	if p.Tractoring == tractorData.TargetID {
		// Turn off tractor
		p.Tractoring = -1
	} else {
		// Check if target is valid and in range
		if tractorData.TargetID >= 0 && tractorData.TargetID < game.MaxPlayers {
			target := c.server.gameState.Players[tractorData.TargetID]
			if target.Status == game.StatusAlive && target.ID != c.PlayerID {
				// Check range (using ship-specific range)
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				tractorRange := float64(game.TractorDist) * shipStats.TractorRange
				if dist <= tractorRange {
					p.Tractoring = tractorData.TargetID
				}
			}
		}
	}
}

// handlePressor handles pressor beam engagement
func (c *Client) handlePressor(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	var pressorData struct {
		TargetID int `json:"targetId"`
	}
	if err := json.Unmarshal(data, &pressorData); err != nil {
		log.Printf("Error unmarshaling pressor data: %v", err)
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Can't use pressor while cloaked
	if p.Cloaked {
		return
	}

	// Get ship stats for range calculation
	shipStats := game.ShipData[p.Ship]

	// Clear tractor if engaging pressor
	p.Tractoring = -1

	// Toggle pressor beam
	if p.Pressoring == pressorData.TargetID {
		// Turn off pressor
		p.Pressoring = -1
	} else {
		// Check if target is valid and in range
		if pressorData.TargetID >= 0 && pressorData.TargetID < game.MaxPlayers {
			target := c.server.gameState.Players[pressorData.TargetID]
			if target.Status == game.StatusAlive && target.ID != c.PlayerID {
				// Check range (using ship-specific range)
				dist := game.Distance(p.X, p.Y, target.X, target.Y)
				tractorRange := float64(game.TractorDist) * shipStats.TractorRange
				if dist <= tractorRange {
					p.Pressoring = pressorData.TargetID
				}
			}
		}
	}
}

// MessageData represents a chat message
type MessageData struct {
	Text   string `json:"text"`
	Target int    `json:"target,omitempty"` // For private messages
}

// handleChatMessage handles all-players messages
func (c *Client) handleChatMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	// Check for bot commands (before sanitization)
	if strings.HasPrefix(msgData.Text, "/") {
		c.handleBotCommand(msgData.Text)
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	c.server.mu.RUnlock()

	// Broadcast to all players
	c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[ALL] %s: %s", formatPlayerName(p), msgData.Text),
			"type": "all",
			"from": c.PlayerID,
		},
	}
}

// handleBotCommand processes bot-related slash commands
func (c *Client) handleBotCommand(cmd string) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return
	}

	switch parts[0] {
	case "/addbot":
		// /addbot [team] [ship]
		team := game.TeamFed
		ship := 1 // Destroyer

		if len(parts) > 1 {
			switch parts[1] {
			case "fed":
				team = game.TeamFed
			case "rom":
				team = game.TeamRom
			case "kli":
				team = game.TeamKli
			case "ori":
				team = game.TeamOri
			}
		}

		if len(parts) > 2 {
			ship, _ = strconv.Atoi(parts[2])
			if ship < 0 || ship > 6 {
				ship = 1
			}
		}

		// Level parameter is ignored - all bots are hard mode now
		if len(parts) > 3 {
			// Silently ignore the level parameter for backward compatibility
		}

		c.server.AddBot(team, ship)

	case "/removebot":
		// Remove a random bot
		for i, p := range c.server.gameState.Players {
			if p.IsBot && p.Connected {
				c.server.RemoveBot(i)
				break
			}
		}

	case "/balance":
		// Auto-balance teams with bots
		c.server.AutoBalanceBots()

	case "/clearbots":
		// Remove all bots
		for i, p := range c.server.gameState.Players {
			if p.IsBot && p.Connected {
				c.server.RemoveBot(i)
			}
		}

	case "/fillbots":
		// Fill all available slots with bots
		// All bots use hard difficulty mode

		// Count free slots
		freeSlots := 0
		for _, p := range c.server.gameState.Players {
			if p.Status == game.StatusFree {
				freeSlots++
			}
		}

		// Distribute bots evenly across teams
		teams := []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri}
		shipTypes := []int{0, 1, 2, 3, 4} // Scout, Destroyer, Cruiser, Battleship, Assault

		botsAdded := 0
		for i := 0; i < freeSlots && botsAdded < game.MaxPlayers-4; i++ { // Max 28 bots to leave room for humans
			// Round-robin team assignment
			team := teams[botsAdded%4]
			// Random ship type
			ship := shipTypes[rand.Intn(5)]

			c.server.AddBot(team, ship)
			botsAdded++
		}

		// Send confirmation message
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": fmt.Sprintf("Added %d bots to fill available slots", botsAdded),
				"type": "info",
			},
		}

	case "/help":
		// Send help message
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Bot commands: /addbot [fed/rom/kli/ori] [0-6] [0-2] | /removebot | /balance | /clearbots | /fillbots [easy/medium/hard]",
				"type": "info",
			},
		}

	default:
		c.send <- ServerMessage{
			Type: MsgTypeMessage,
			Data: map[string]interface{}{
				"text": "Unknown command. Type /help for bot commands.",
				"type": "warning",
			},
		}
	}
}

// handleTeamMessage handles team-only messages
func (c *Client) handleTeamMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	team := p.Team
	c.server.mu.RUnlock()

	// Send to team members only
	teamMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[TEAM] %s: %s", formatPlayerName(p), msgData.Text),
			"type": "team",
			"from": c.PlayerID,
			"team": team,
		},
	}

	c.server.mu.RLock()
	for _, client := range c.server.clients {
		if client.PlayerID >= 0 && client.PlayerID < game.MaxPlayers {
			clientPlayer := c.server.gameState.Players[client.PlayerID]
			if clientPlayer.Team == team {
				select {
				case client.send <- teamMsg:
				default:
					// Client's send channel is full, skip
				}
			}
		}
	}
	c.server.mu.RUnlock()
}

// handlePrivateMessage handles private messages
func (c *Client) handlePrivateMessage(data json.RawMessage) {
	var msgData MessageData
	if err := json.Unmarshal(data, &msgData); err != nil {
		return
	}

	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	if msgData.Target < 0 || msgData.Target >= game.MaxPlayers {
		return
	}

	// Sanitize the message text to prevent XSS
	msgData.Text = sanitizeText(msgData.Text)

	c.server.mu.RLock()
	p := c.server.gameState.Players[c.PlayerID]
	targetPlayer := c.server.gameState.Players[msgData.Target]
	c.server.mu.RUnlock()

	// Send to target and sender only
	privMsg := ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("[PRIV->%s] %s: %s", formatPlayerName(targetPlayer), formatPlayerName(p), msgData.Text),
			"type": "private",
			"from": c.PlayerID,
			"to":   msgData.Target,
		},
	}

	c.server.mu.RLock()
	for _, client := range c.server.clients {
		if client.PlayerID == msgData.Target || client.PlayerID == c.PlayerID {
			select {
			case client.send <- privMsg:
			default:
				// Client's send channel is full, skip
			}
		}
	}
	c.server.mu.RUnlock()
}

// handleCloak handles cloaking/uncloaking
func (c *Client) handleCloak(data json.RawMessage) {
	c.server.mu.Lock()
	defer c.server.mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		return
	}

	// Toggle cloak
	p.Cloaked = !p.Cloaked

	// Send cloak status message to all clients
	var message string
	if p.Cloaked {
		message = fmt.Sprintf("%s engaged cloaking device", formatPlayerName(p))
	} else {
		message = fmt.Sprintf("%s disengaged cloaking device", formatPlayerName(p))
	}

	c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": message,
			"type": "info",
		},
	}
}

// handleQuit handles player quit/self-destruct request
func (c *Client) handleQuit(data json.RawMessage) {
	if c.PlayerID < 0 || c.PlayerID >= game.MaxPlayers {
		return
	}

	c.server.gameState.Mu.Lock()
	defer c.server.gameState.Mu.Unlock()

	p := c.server.gameState.Players[c.PlayerID]
	if p.Status != game.StatusAlive {
		// If already dead, just disconnect
		c.conn.Close()
		return
	}

	// Self-destruct the ship
	p.Status = game.StatusExplode
	p.ExplodeTimer = 10       // Explosion animation frames
	p.KilledBy = c.PlayerID   // Killed by self
	p.WhyDead = game.KillQuit // Quit reason

	// Stop all movement
	p.Speed = 0
	p.DesSpeed = 0

	// Clear all states
	p.Shields_up = false
	p.Cloaked = false
	p.Repairing = false
	p.RepairRequest = false
	p.Bombing = false
	p.Beaming = false
	p.BeamingUp = false
	p.Tractoring = -1
	p.Pressoring = -1
	p.Orbiting = -1
	p.LockType = "none"
	p.LockTarget = -1

	// Broadcast self-destruct message
	c.server.broadcast <- ServerMessage{
		Type: MsgTypeMessage,
		Data: map[string]interface{}{
			"text": fmt.Sprintf("%s self-destructed", p.Name),
			"type": "warning",
		},
	}

	// Close the connection after a short delay to allow the explosion to be seen
	go func() {
		time.Sleep(1 * time.Second)
		c.conn.Close()
	}()
}
