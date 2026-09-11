package server

import (
	"encoding/json"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestTournamentJoinRequiresAnOwnedPlanet(t *testing.T) {
	for _, team := range []int{game.TeamFed, game.TeamRom, game.TeamKli, game.TeamOri} {
		for _, mode := range []string{"eliminated", "practice", "regained"} {
			s := NewServer()
			s.gameState.T_mode = mode != "practice"
			for _, planet := range s.gameState.Planets {
				planet.Owner = game.TeamNone
			}
			// Deliberately stale totals must not allow an eliminated team to join.
			s.gameState.TeamPlanets = [4]int{10, 10, 10, 10}
			if mode == "regained" {
				s.gameState.Planets[0].Owner = team
			}
			c := &Client{ID: 1, server: s, send: make(chan ServerMessage, 64)}
			c.SetPlayerID(-1)
			data, _ := json.Marshal(LoginData{Name: "Returning pilot", Team: team, Ship: game.ShipCruiser})
			c.handleLogin(data)
			allowed := mode != "eliminated"
			if (c.GetPlayerID() >= 0) != allowed {
				t.Fatalf("team=%d mode=%s: unexpected login assignment %d", team, mode, c.GetPlayerID())
			}
			if s.AddBot(team, game.ShipCruiser) != allowed {
				t.Fatalf("team=%d mode=%s: bot admission bypassed spawn eligibility", team, mode)
			}
		}
	}
}

func TestBeamsCannotAcquireOrKeepHiddenEnemies(t *testing.T) {
	for _, pressor := range []bool{false, true} {
		s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		p.X, p.Y = 50000, 50000
		target := s.gameState.Players[1]
		target.Status, target.Team, target.Ship, target.Cloaked = game.StatusAlive, game.TeamRom, game.ShipCruiser, true
		target.X, target.Y = p.X+3000, p.Y
		target.Orbiting = 0
		beam := &p.Tractoring
		if pressor {
			beam = &p.Pressoring
		}
		c.handleBeamEngage(json.RawMessage(`{"targetId":1}`), pressor)
		if *beam != -1 {
			t.Fatal("beam acquired hidden target")
		}
		target.Cloaked = false
		c.handleBeamEngage(json.RawMessage(`{"targetId":1}`), pressor)
		if *beam != 1 {
			t.Fatal("beam could not acquire visible target")
		}
		target.Cloaked = true
		s.updateTractorBeams()
		if *beam != -1 || p.X != 50000 || target.X != 53000 || target.Orbiting != 0 {
			t.Fatal("beam tracked or moved a target after it cloaked")
		}
		target.Team = p.Team
		c.handleBeamEngage(json.RawMessage(`{"targetId":1}`), pressor)
		s.updateTractorBeams()
		if *beam != 1 || p.X == 50000 {
			t.Fatal("beam failed for a visible cloaked teammate")
		}
	}
}

func TestChatRecipientsMustOwnTheirSlots(t *testing.T) {
	for _, private := range []bool{false, true} {
		s, sender, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		s.clients[sender.ID] = sender
		recipient := &Client{ID: 2, server: s, send: make(chan ServerMessage, 8)}
		recipient.SetPlayerID(1)
		s.clients[recipient.ID] = recipient
		target := s.gameState.Players[1]
		target.Status, target.Team, target.Connected, target.OwnerClientID = game.StatusAlive, p.Team, true, recipient.ID
		// A stale connection still references the now-reassigned slot.
		stale := &Client{ID: 3, server: s, send: make(chan ServerMessage, 8)}
		stale.SetPlayerID(1)
		s.clients[stale.ID] = stale
		data := json.RawMessage(`{"text":"secret","target":1}`)
		if private {
			sender.handlePrivateMessage(data)
		} else {
			sender.handleTeamMessage(data)
		}
		if len(sender.send) != 1 || len(recipient.send) != 1 || len(stale.send) != 0 {
			t.Fatalf("private=%v: delivery counts sender=%d owner=%d stale=%d", private, len(sender.send), len(recipient.send), len(stale.send))
		}
	}
}

func TestQuitCannotDestroyReassignedSlot(t *testing.T) {
	_, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.OwnerClientID = c.ID + 1
	c.handleQuit(nil)
	if p.Status != game.StatusAlive || c.quitting.Load() {
		t.Fatal("stale client self-destructed another pilot's ship")
	}
}

func TestEliminatedTeamsDoNotBlockBalancedTournamentJoins(t *testing.T) {
	s := NewServer()
	s.gameState.T_mode = true
	for _, planet := range s.gameState.Planets {
		if planet.Owner == game.TeamFed || planet.Owner == game.TeamOri {
			planet.Owner = game.TeamNone
		}
	}
	for i, team := range []int{game.TeamRom, game.TeamKli} {
		p := s.gameState.Players[i]
		p.Team, p.Status, p.Connected = team, game.StatusAlive, true
	}
	counts := s.computeTeamCounts()
	if counts.SpawnTeams != game.TeamRom|game.TeamKli {
		t.Fatalf("wrong lobby eligibility: %d", counts.SpawnTeams)
	}
	c := &Client{ID: 5, server: s, send: make(chan ServerMessage, 8)}
	c.SetPlayerID(-1)
	c.handleLogin(json.RawMessage(`{"name":"Pilot","team":2,"ship":2}`))
	if c.GetPlayerID() < 0 {
		t.Fatal("empty eliminated teams prevented joining balanced remaining teams")
	}
}
