package server

import (
	"strings"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestAutoBalanceReportsOnlySuccessfulAdditions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		players    int
		eliminated bool
		wantBots   int
		wantText   []string
	}{
		{"full", game.MaxPlayers, false, 0, []string{"no bots added", "server full"}},
		{"eliminated", 1, true, 0, []string{"no bots added", "cannot spawn"}},
		{"partial", game.MaxPlayers - 1, false, 1, []string{"added 1 bot to Romulan", "balance incomplete"}},
		{"available", 1, false, 3, []string{"1 bot to Romulan", "1 bot to Klingon", "1 bot to Orion"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer()
			for _, p := range s.gameState.Players[:tc.players] {
				p.Status, p.Connected, p.Team, p.Ship = game.StatusAlive, true, game.TeamFed, game.ShipCruiser
			}
			if tc.eliminated {
				s.gameState.T_mode = true
				for _, planet := range s.gameState.Planets {
					planet.Owner = game.TeamFed
				}
			}
			s.AutoBalanceBots()
			bots := 0
			for _, p := range s.gameState.Players {
				if p.Connected && p.IsBot {
					bots++
				}
			}
			if bots != tc.wantBots {
				t.Fatalf("added %d actual bots, want %d", bots, tc.wantBots)
			}
			select {
			case msg := <-s.broadcast:
				text := msg.Data.(map[string]interface{})["text"].(string)
				for _, want := range tc.wantText {
					if !strings.Contains(text, want) {
						t.Errorf("feedback %q does not include %q", text, want)
					}
				}
			default:
				t.Fatal("missing auto-balance feedback")
			}
		})
	}
}
