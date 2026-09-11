package server

import (
	"encoding/json"
	"math"
	"testing"
	"testing/synctest"
	"time"

	"github.com/lab1702/netrek-web/game"
)

func TestLightDamageCannotIncreaseShipSpeed(t *testing.T) {
	for ship, stats := range game.ShipData {
		s, c, p := newTestClientAndPlayer(game.TeamFed, ship)
		p.Damage = 1
		p.X, p.Y = 50000, 50000
		c.handleMove(json.RawMessage(`{"speed":12,"dir":0}`))
		if p.DesSpeed > float64(stats.MaxSpeed) {
			t.Fatalf("%s accepts speed %v above %d", stats.Name, p.DesSpeed, stats.MaxSpeed)
		}
		// Physics must also enforce the cap for direct AI speed requests.
		p.DesSpeed = 12
		for i := 0; i < 300; i++ {
			s.updatePlayerPhysics(p, p.ID)
		}
		if p.Speed > float64(stats.MaxSpeed) {
			t.Fatalf("%s reaches speed %v above %d", stats.Name, p.Speed, stats.MaxSpeed)
		}
	}
}

func TestOrbitEntryKeepsApproachSideAndTangent(t *testing.T) {
	for _, entry := range []string{"manual", "automatic", "bot"} {
		for _, angle := range []float64{0, math.Pi / 2, math.Pi, -math.Pi / 2} {
			s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
			planet := s.gameState.Planets[0]
			p.X = planet.X + 850*math.Cos(angle)
			p.Y = planet.Y + 850*math.Sin(angle)
			p.Dir, p.DesDir = angle+math.Pi, angle+math.Pi
			p.Speed, p.DesSpeed = 1, 1
			p.Tractoring = 1
			startX, startY := p.X, p.Y
			switch entry {
			case "manual":
				c.handleOrbit(nil)
			case "automatic":
				p.LockType, p.LockTarget = "planet", planet.ID
				s.updatePlayerLockOn(p)
			case "bot":
				p.Damage = 75
				planet.Flags |= game.PlanetRepair
				s.updateBotHard(p)
			}
			s.updatePlayerOrbit(p)
			if p.Orbiting != planet.ID || p.Tractoring != -1 || p.Speed != 0 {
				t.Fatalf("%s failed to enter orbit", entry)
			}
			if distance := game.Distance(startX, startY, p.X, p.Y); distance > 100 {
				t.Fatalf("%s jumped %v units on orbit entry", entry, distance)
			}
			if radial := game.Distance(p.X, p.Y, planet.X, planet.Y); math.Abs(radial-float64(game.OrbitDist)) > 0.001 {
				t.Fatalf("%s has wrong orbit radius %v", entry, radial)
			}
		}
	}
}

func TestVictoryTimersBelongToTheirRound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		defer s.Shutdown()
		s.clients[c.ID] = c
		s.galaxyReset = false
		s.gameState.GameOver, s.gameState.Winner, s.gameState.WinType = true, p.Team, "genocide"
		s.announceVictory()
		synctest.Wait() // Start the first round's ten-second timer.
		s.freeDisconnectedSlot(c.ID, p.ID)
		c.SetPlayerID(-1)
		s.updateGame() // Empty-galaxy reset invalidates the first timer.
		c.handleLogin(json.RawMessage(`{"name":"New round","team":1,"ship":2}`))
		if c.GetPlayerID() < 0 {
			t.Fatal("new round login failed")
		}
		time.Sleep(5 * time.Second)
		// A new victory may schedule a timer while the old one still exists.
		s.gameState.GameOver, s.gameState.Winner, s.gameState.WinType = true, game.TeamFed, "genocide"
		s.announceVictory()
		synctest.Wait()
		time.Sleep(6 * time.Second)
		synctest.Wait()
		if c.GetPlayerID() < 0 {
			t.Fatal("old victory timer evicted the new round's player")
		}
		time.Sleep(5 * time.Second)
		synctest.Wait()
		if c.GetPlayerID() != -1 {
			t.Fatal("new round's own victory timer failed to reset it")
		}
	})
}
