package server

import (
	"github.com/lab1702/netrek-web/game"
	"html"
	"math"
	"strings"
)

// shipAlias maps ship type abbreviations to ship type integers
var shipAlias = map[string]int{
	"SC": 0, "SCOUT": 0,
	"DD": 1, "DESTROYER": 1,
	"CA": 2, "CRUISER": 2,
	"BB": 3, "BATTLESHIP": 3,
	"AS": 4, "ASSAULT": 4,
	"SB": 5, "STARBASE": 5,
}

// Handler data structures

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

// PlasmaData represents plasma fire command
type PlasmaData struct {
	Dir float64 `json:"dir"` // Direction to fire
}

// BeamData represents army beam request
type BeamData struct {
	Up bool `json:"up"` // true = beam up, false = beam down
}

// MessageData represents a chat message
type MessageData struct {
	Text   string `json:"text"`
	Target int    `json:"target,omitempty"` // For private messages
}

// Utility functions

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
