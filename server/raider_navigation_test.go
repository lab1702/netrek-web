package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestPracticeRaiderLeavesOldOrbitAndBrakesForItsTarget(t *testing.T) {
	for _, departing := range []bool{false, true} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		p.IsBot, p.KillsStreak = true, game.ArmyKillRequirement
		for i, planet := range s.gameState.Planets {
			planet.Owner = game.TeamRom
			if i < 20 {
				planet.Owner = p.Team
			}
			planet.X, planet.Y = 95000, 95000
		}
		home, target := s.gameState.Planets[0], s.gameState.Planets[20]
		home.X, home.Y = 50000, 50000
		target.X, target.Y, target.Armies = 53000, 50000, 10
		p.X, p.Y, p.Speed, p.DesSpeed = 52200, 50000, 9, 9
		if departing {
			p.X, p.Speed, p.DesSpeed = home.X+800, 0, 0
			p.Orbiting, p.Beaming, p.BeamingUp, p.Bombing = home.ID, true, true, true
		}
		s.updateBotHard(p)
		if p.Orbiting != -1 || p.Beaming || p.BeamingUp || p.Bombing {
			t.Fatal("raider kept its old orbit or planet actions")
		}
		if !departing && p.DesSpeed >= 9 {
			t.Fatal("raider did not request braking near its target")
		}
		for i := 0; i < 300 && p.Orbiting < 0; i++ {
			s.gameState.Frame++
			s.updatePlayerPhysics(p, p.ID)
			s.updatePlayerOrbit(p)
			s.UpdateBots()
		}
		if p.Orbiting != target.ID || !p.Bombing {
			t.Fatalf("departing=%v: raider failed to reach and bomb target: orbit=%d speed=%v distance=%v", departing, p.Orbiting, p.Speed, game.Distance(p.X, p.Y, target.X, target.Y))
		}
	}
}
