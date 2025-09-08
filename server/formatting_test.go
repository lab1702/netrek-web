package server

import (
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
			if !containsString(result, tt.expectedSlot) {
				t.Errorf("Expected slot format %q not found in result %q", tt.expectedSlot, result)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
