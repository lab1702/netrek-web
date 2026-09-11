package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestMovingOutOfOrbitCancelsArmyTransfers(t *testing.T) {
	s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Orbiting, p.Beaming, p.BeamingUp, p.KillsStreak = 0, true, true, 2
	c.handleMove(json.RawMessage(`{"speed":0,"dir":0}`))
	planet := s.gameState.Planets[1]
	planet.Owner, planet.Armies = p.Team, 10
	p.X, p.Y = planet.X+850, planet.Y
	c.handleOrbit(nil)
	s.updateOrbitingPlayer(p, p.ID)
	if p.Orbiting != planet.ID || p.Beaming || p.BeamingUp || p.Armies != 0 || planet.Armies != 10 {
		t.Fatal("moving to another planet preserved continuous army transfer")
	}
}

func TestStarbaseApproachesBeforeEnteringOrbit(t *testing.T) {
	for _, posture := range []string{"core", "threatened"} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipStarbase)
		p.IsBot = true
		planet := s.gameState.Planets[0]
		for i, other := range s.gameState.Planets {
			if i == 0 {
				continue
			}
			other.Owner = game.TeamNone
			other.X, other.Y = 95000, 95000
			if posture == "threatened" && i < 10 {
				other.Owner = p.Team
			}
		}
		if posture == "threatened" {
			enemy := s.gameState.Players[1]
			enemy.Status, enemy.Team = game.StatusAlive, game.TeamRom
			enemy.X, enemy.Y = planet.X+14000, planet.Y
		}
		p.X, p.Y, p.Speed = planet.X-2500, planet.Y, 2
		startX, startY := p.X, p.Y
		s.updateStarbaseBot(p)
		if p.Orbiting != -1 || p.X != startX || p.Y != startY || p.DesSpeed > 2 || p.DesSpeed <= 0 {
			t.Fatalf("%s: starbase skipped its approach", posture)
		}
		p.X, p.Y, p.Speed = planet.X-850, planet.Y, 3
		s.updateStarbaseBot(p)
		if p.Orbiting != -1 {
			t.Fatalf("%s: starbase entered orbit above warp 2", posture)
		}
		p.Speed = 2
		s.updateStarbaseBot(p)
		if p.Orbiting != planet.ID {
			t.Fatalf("%s: starbase failed eligible orbit entry", posture)
		}
	}
}

func TestHumanAndBotPhasersHitTheNearestObject(t *testing.T) {
	for _, firing := range []string{"human", "bot_ship", "bot_plasma"} {
		for _, arrangement := range []string{"ship_first", "plasma_first", "reverse_plasma_order"} {
			s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
			p.X, p.Y = 50000, 50000
			target := s.gameState.Players[1]
			target.Status, target.Team, target.Ship = game.StatusAlive, game.TeamRom, game.ShipCruiser
			target.X, target.Y = 52000, 50000
			plasma := &game.Plasma{Owner: 1, Team: target.Team, X: 54000, Y: 50000, Status: game.TorpMove}
			s.gameState.Plasmas = []*game.Plasma{plasma}
			nearestPlasma := plasma
			if arrangement != "ship_first" {
				target.X, plasma.X = 54000, 52000
			}
			if arrangement == "reverse_plasma_order" {
				target.X, plasma.X = 55000, 54000
				nearestPlasma = &game.Plasma{Owner: 1, Team: target.Team, X: 52000, Y: 50000, Status: game.TorpMove}
				s.gameState.Plasmas = append(s.gameState.Plasmas, nearestPlasma)
			}
			s.gameState.T_mode = true
			s.gameState.TournamentStats[p.ID] = &game.TournamentPlayerStats{}
			s.gameState.TournamentStats[target.ID] = &game.TournamentPlayerStats{}
			fuel := p.Fuel
			switch firing {
			case "human":
				c.handlePhaser(json.RawMessage(`{"target":-1,"dir":0}`))
			case "bot_ship":
				s.fireBotPhaser(p, target)
			case "bot_plasma":
				if !s.fireBotPhaserAtPlasma(p, plasma) {
					t.Fatal("eligible bot shot was not fired")
				}
			}
			if arrangement == "ship_first" {
				if target.Damage != 67 || plasma.Status != game.TorpMove || s.gameState.TournamentStats[p.ID].DamageDealt != 67 {
					t.Fatalf("%s: ship failed to intercept plasma-defense shot: damage=%d plasma=%d", firing, target.Damage, plasma.Status)
				}
			} else if nearestPlasma.Status != game.TorpDet || target.Damage != 0 || (arrangement == "reverse_plasma_order" && plasma.Status != game.TorpMove) {
				t.Fatalf("%s/%s: nearest plasma did not intercept shot", firing, arrangement)
			}
			stats := game.ShipData[p.Ship]
			if p.Fuel != fuel-stats.PhaserDamage*stats.PhaserFuelMult || p.WTemp != 70 || len(s.broadcast) != 1 {
				t.Fatalf("%s/%s: wrong shot cost or number of visuals", firing, arrangement)
			}
		}
	}
}
