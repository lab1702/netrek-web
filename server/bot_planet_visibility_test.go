package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestStrategicPlanetSelectionRespectsCloaking(t *testing.T) {
	for _, selector := range []struct {
		name     string
		owner    int
		selectFn func(*Server, *game.Player) *game.Planet
	}{
		{"starbase defense", game.TeamFed, (*Server).findMostThreatenedFriendlyPlanet},
		{"ship defense", game.TeamFed, (*Server).findPlanetToDefend},
		{"raid", game.TeamRom, (*Server).findPlanetToRaid},
	} {
		t.Run(selector.name, func(t *testing.T) {
			s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
			p.X, p.Y = 10000, 10000
			for _, planet := range s.gameState.Planets {
				planet.Owner = game.TeamNone
			}
			planet := s.gameState.Planets[0]
			planet.Owner, planet.Armies = selector.owner, 10
			planet.X, planet.Y = 25000, 10000
			enemy := s.gameState.Players[1]
			enemy.Status, enemy.Team = game.StatusAlive, game.TeamRom
			enemy.X, enemy.Y, enemy.Armies = planet.X, planet.Y, 5

			for _, cloaked := range []bool{false, true, false} {
				enemy.Cloaked = cloaked
				selected := selector.selectFn(s, p)
				wantPlanet := !cloaked
				if selector.owner == game.TeamRom {
					wantPlanet = cloaked // Visible defenders deter a raid.
				}
				if (wantPlanet && selected != planet) || (!wantPlanet && selected != nil) {
					t.Fatalf("cloaked=%v: selected=%v, wantPlanet=%v", cloaked, selected, wantPlanet)
				}
			}
		})
	}
}

func TestDistantCloakedEnemyDoesNotRedirectStarbasePatrol(t *testing.T) {
	s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipStarbase)
	p.IsBot, p.X, p.Y = true, 50000, 50000
	s.gameState.Frame = 200
	for _, planet := range s.gameState.Planets {
		planet.Owner = game.TeamFed
		planet.X, planet.Y = p.X, p.Y
	}
	s.gameState.Planets[0].X, s.gameState.Planets[0].Y = 10000, 10000
	enemy := s.gameState.Players[1]
	enemy.Status, enemy.Team, enemy.Cloaked = game.StatusAlive, game.TeamRom, true
	enemy.X, enemy.Y, enemy.Armies = 10000, 15000, 5
	s.updateStarbaseBot(p)
	if p.DesSpeed != 1 || p.Shields_up {
		t.Fatalf("hidden enemy redirected starbase from patrol: speed=%v shields=%v", p.DesSpeed, p.Shields_up)
	}
}
