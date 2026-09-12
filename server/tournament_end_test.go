package server

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestGenocideRespectsTournamentRespawnEligibility(t *testing.T) {
	for _, status := range []int{game.StatusDead, game.StatusExplode} {
		for _, ownsPlanets := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%d/ownsPlanets=%v", status, ownsPlanets), func(t *testing.T) {
				s := NewServer()
				t.Cleanup(s.Shutdown)
				addTournamentParticipants(s)
				s.gameState.T_mode, s.gameState.Frame = true, 200
				for _, planet := range s.gameState.Planets {
					if !ownsPlanets && planet.Owner == game.TeamRom {
						planet.Owner, planet.Armies = game.TeamNone, 0
					}
				}
				for i := 4; i < 8; i++ {
					p := s.gameState.Players[i]
					p.Status, p.ExplodeTimer = status, 8
				}
				s.updateGame()
				if ownsPlanets {
					if s.gameState.GameOver {
						t.Fatal("a team able to respawn was eliminated")
					}
				} else if !s.gameState.GameOver || s.gameState.Winner != game.TeamFed || s.gameState.WinType != "genocide" {
					t.Fatalf("planetless eliminated team blocked victory: gameOver=%v winner=%d winType=%q", s.gameState.GameOver, s.gameState.Winner, s.gameState.WinType)
				}
			})
		}
	}
}

func TestPracticeRespawnsDoNotRequirePlanets(t *testing.T) {
	s := NewServer()
	t.Cleanup(s.Shutdown)
	addTournamentParticipants(s)
	s.gameState.Frame = 200
	for _, planet := range s.gameState.Planets {
		if planet.Owner == game.TeamRom {
			planet.Owner, planet.Armies = game.TeamNone, 0
		}
	}
	for i := 4; i < 8; i++ {
		s.gameState.Players[i].Status = game.StatusDead
	}
	s.checkVictoryConditions()
	if s.gameState.GameOver {
		t.Fatal("practice players can respawn without owning planets")
	}
}

func TestTournamentTimeoutWithoutOwnedPlanets(t *testing.T) {
	for _, status := range []int{game.StatusAlive, game.StatusDead} {
		t.Run(fmt.Sprintf("status=%d", status), func(t *testing.T) {
			s := NewServer()
			t.Cleanup(s.Shutdown)
			addTournamentParticipants(s)
			s.gameState.T_mode, s.gameState.Frame = true, 17999
			for _, planet := range s.gameState.Planets {
				planet.Owner, planet.Armies = game.TeamNone, 0
			}
			for _, p := range s.gameState.Players {
				if p.Connected {
					p.Status = status
				}
			}
			s.updateGame()
			if !s.gameState.GameOver || s.gameState.Winner != game.TeamNone || s.gameState.WinType != "timeout" || !s.resetScheduled {
				t.Fatalf("zero-planet timeout did not end and schedule reset: gameOver=%v winner=%d winType=%q resetScheduled=%v", s.gameState.GameOver, s.gameState.Winner, s.gameState.WinType, s.resetScheduled)
			}
			select {
			case msg := <-s.broadcast:
				data, ok := msg.Data.(map[string]interface{})
				if !ok || data["type"] != "victory" || !strings.Contains(data["text"].(string), "DRAW") {
					t.Fatalf("missing draw announcement: %#v", msg)
				}
			default:
				t.Fatal("missing draw announcement")
			}
			s.resetRound(s.roundGeneration, true)
			if s.gameState.GameOver || s.resetScheduled || s.gameState.Players[0].Status != game.StatusFree {
				t.Fatal("draw did not reset to the lobby")
			}
		})
	}
}
