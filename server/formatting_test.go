package server

import (
	"strings"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestPlayerSlotFormatting tests that player slots are displayed with zero padding
func TestPlayerSlotFormatting(t *testing.T) {
	tests := []struct {
		name         string
		playerID     int
		team         int
		playerName   string
		expectedSlot string
	}{
		{
			name:         "Single digit Federation player",
			playerID:     2,
			team:         game.TeamFed,
			playerName:   "TestPlayer",
			expectedSlot: "[F02]",
		},
		{
			name:         "Single digit Romulan player",
			playerID:     5,
			team:         game.TeamRom,
			playerName:   "BotPlayer",
			expectedSlot: "[R05]",
		},
		{
			name:         "Double digit Klingon player",
			playerID:     12,
			team:         game.TeamKli,
			playerName:   "Captain",
			expectedSlot: "[K12]",
		},
		{
			name:         "Single digit Orion player",
			playerID:     8,
			team:         game.TeamOri,
			playerName:   "Admiral",
			expectedSlot: "[O08]",
		},
		{
			name:         "Zero slot player",
			playerID:     0,
			team:         game.TeamFed,
			playerName:   "FirstPlayer",
			expectedSlot: "[F00]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			player := &game.Player{
				ID:   tt.playerID,
				Name: tt.playerName,
				Team: tt.team,
			}

			result := formatPlayerName(player)
			expected := tt.playerName + " " + tt.expectedSlot

			if result != expected {
				t.Errorf("formatPlayerName() = %q, want %q", result, expected)
			}

			// Also check that the slot part specifically has the zero padding
			if !strings.Contains(result, tt.expectedSlot) {
				t.Errorf("Expected slot format %q not found in result %q", tt.expectedSlot, result)
			}
		})
	}
}

// TestSanitizeName tests player name sanitization
func TestSanitizeName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Normal name passes through",
			input:    "TestPlayer",
			expected: "TestPlayer",
		},
		{
			name:     "Name starting with number strips leading digits",
			input:    "123Player",
			expected: "Player",
		},
		{
			name:     "All digits returns empty string",
			input:    "12345",
			expected: "",
		},
		{
			name:     "Leading zeros stripped",
			input:    "007Bond",
			expected: "Bond",
		},
		{
			name:     "Numbers in middle preserved",
			input:    "Player123",
			expected: "Player123",
		},
		{
			name:     "Special characters removed",
			input:    "Test!@#Player",
			expected: "TestPlayer",
		},
		{
			name:     "Name truncated at 20 chars",
			input:    "ThisIsAVeryLongPlayerNameThatExceedsTwentyCharacters",
			expected: "ThisIsAVeryLongPlaye",
		},
		{
			name:     "Empty string stays empty",
			input:    "",
			expected: "",
		},
		{
			name:     "Mixed leading special chars and digits",
			input:    "!@#123ABC",
			expected: "ABC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
