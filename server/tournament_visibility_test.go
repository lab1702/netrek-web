package server

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/lab1702/netrek-web/game"
)

// Populate authoritative planet ownership for tests that specify cached totals.
func setTournamentPlanetOwners(s *Server) {
	next := 0
	for team, count := range s.gameState.TeamPlanets {
		for j := 0; j < count; j++ {
			s.gameState.Planets[next].Owner = 1 << team
			next++
		}
	}
	for ; next < game.MaxPlanets; next++ {
		s.gameState.Planets[next].Owner = game.TeamNone
	}
}

func addTournamentParticipants(s *Server) {
	for i := 0; i < 8; i++ {
		p := s.gameState.Players[i]
		p.Status, p.Connected, p.Ship = game.StatusAlive, true, game.ShipCruiser
		p.Team = game.TeamFed
		if i >= 4 {
			p.Team = game.TeamRom
		}
	}
}

func TestResetWaitsForLoginBeforeClearingAssignment(t *testing.T) {
	s := NewServer()
	c := &Client{ID: 1, server: s}
	c.SetPlayerID(-1)
	s.clients[c.ID] = c
	// Model a login already inside the game-state critical section.
	s.gameState.Mu.Lock()
	done := make(chan struct{})
	go func() { s.resetGame(); close(done) }()
	deadline := time.Now().Add(2 * time.Second)
	for s.mu.TryLock() {
		s.mu.Unlock()
		if time.Now().After(deadline) {
			s.gameState.Mu.Unlock()
			t.Fatal("reset did not acquire client lock")
		}
		runtime.Gosched()
	}
	c.SetPlayerID(0)
	s.gameState.Players[0].Status = game.StatusAlive
	s.gameState.Mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reset blocked")
	}
	if c.GetPlayerID() != -1 || s.gameState.Players[0].Status != game.StatusFree {
		t.Fatal("reset left a client assigned to a freed slot")
	}
}

func TestFatalOrbitPlanetDamageCountsOneDeath(t *testing.T) {
	s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	planet := s.gameState.Planets[0]
	planet.Owner, planet.Armies = game.TeamRom, 17
	p.X, p.Y = planet.X+game.OrbitDist, planet.Y
	p.Orbiting, p.Damage, p.Shields_up = 0, 99, false
	s.gameState.Frame = 5
	s.updatePlanetInteractions()
	if p.Status != game.StatusExplode || p.Deaths != 1 {
		t.Fatalf("status=%d deaths=%d", p.Status, p.Deaths)
	}
}

func TestTournamentEntryResetsDeadParticipants(t *testing.T) {
	for _, status := range []int{game.StatusDead, game.StatusExplode} {
		s := NewServer()
		addTournamentParticipants(s)
		p := s.gameState.Players[0]
		p.Status, p.ExplodeTimer, p.Kills, p.Deaths, p.Damage = status, 8, 9, 7, 100
		s.gameState.TournamentStats[p.ID] = &game.TournamentPlayerStats{Kills: 9, Deaths: 7}
		s.checkTournamentMode()
		stats := s.gameState.TournamentStats[p.ID]
		if !s.gameState.T_mode || p.Status != game.StatusAlive || p.ExplodeTimer != 0 || p.Kills != 0 || p.Deaths != 0 || p.Damage != 0 || stats.Kills != 0 || stats.Deaths != 0 {
			t.Fatal("tournament entry retained pre-tournament death/stat state")
		}
	}
}

func TestTournamentTimeoutUsesCurrentPlanetOwners(t *testing.T) {
	s := NewServer()
	t.Cleanup(s.Shutdown)
	addTournamentParticipants(s)
	s.gameState.T_mode, s.gameState.Frame = true, 18000
	s.gameState.TeamPlanets = [4]int{20, 19, 1, 0}
	setTournamentPlanetOwners(s)
	// The final tick changes the lead, while the cached counts still favor Fed.
	s.gameState.Planets[0].Owner = game.TeamRom
	s.checkTournamentMode()
	if !s.gameState.GameOver || s.gameState.Winner != game.TeamRom || s.gameState.TeamPlanets != [4]int{19, 20, 1, 0} {
		t.Fatalf("wrong final result: winner=%d planets=%v", s.gameState.Winner, s.gameState.TeamPlanets)
	}
}

func TestLowFuelCloakedBotCanDecloak(t *testing.T) {
	for _, ship := range []game.ShipType{game.ShipScout, game.ShipDestroyer} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, ship)
		p.Cloaked, p.Fuel = true, 1400
		target := s.gameState.Players[1]
		target.Status, target.Ship, target.Team = game.StatusAlive, game.ShipCruiser, game.TeamRom
		target.X, target.Y = p.X+800, p.Y
		s.engageCombat(p, target, 800)
		if p.Cloaked {
			t.Fatal("low-fuel bot stayed cloaked at close range")
		}
	}
}

func TestHunterCannotAcquireOrRetainDistantCloakedTarget(t *testing.T) {
	for _, locked := range []bool{false, true} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipDestroyer)
		target := s.gameState.Players[1]
		target.Status, target.Team, target.Ship, target.Cloaked = game.StatusAlive, game.TeamRom, game.ShipCruiser, true
		target.X, target.Y = p.X+10000, p.Y
		if locked {
			p.BotTarget, p.BotTargetLockTime = 1, 30
		}
		if s.selectBestCombatTarget(p) != nil || p.BotTarget != -1 {
			t.Fatal("hunter targeted distant cloaked pilot")
		}
		target.X = p.X + 1000
		if s.selectBestCombatTarget(p) != target {
			t.Fatal("hunter failed to detect target inside cloak detection range")
		}
	}
}

func TestOverheatActivationPreservesSpeedCommand(t *testing.T) {
	s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.DesSpeed, p.Speed = 7, 7
	// Exercise actual probabilistic activation (at least 1/8 chance each tick).
	for i := 0; i < 1000 && !p.EngineOverheat; i++ {
		p.ETemp = game.MaxEngineTempCap
		s.updatePlayerSystems(p, p.ID)
	}
	if !p.EngineOverheat {
		t.Fatal("overheat did not activate")
	}
	if p.DesSpeed != 7 {
		t.Fatal("overheat destroyed requested speed")
	}
	p.Speed, p.ETemp, p.OverheatTimer = 1, 0, 1
	s.updatePlayerSystems(p, p.ID)
	for i := 0; i < 100; i++ {
		s.updatePlayerPhysics(p, p.ID)
	}
	if p.EngineOverheat || p.Speed != 7 {
		t.Fatalf("speed did not recover: %v", p.Speed)
	}
}

func TestGameUpdatesRespectTeamVisibility(t *testing.T) {
	s := NewServer()
	s.gameState.T_mode = true
	planet := s.gameState.Planets[0]
	planet.Owner, planet.Armies, planet.Flags, planet.Info = game.TeamRom, 23, game.PlanetFuel|game.PlanetRepair, game.TeamRom
	enemy := s.gameState.Players[1]
	enemy.Status, enemy.Connected, enemy.Team, enemy.Cloaked = game.StatusAlive, true, game.TeamRom, true
	enemy.X, enemy.Y = 12345, 67890
	for i, team := range []int{game.TeamFed, game.TeamRom, game.TeamNone} {
		c := &Client{ID: i + 10, server: s, send: make(chan ServerMessage, 2)}
		c.SetPlayerID(-1)
		if team != game.TeamNone {
			p := s.gameState.Players[i+2]
			p.Status, p.Team, p.OwnerClientID = game.StatusAlive, team, c.ID
			c.SetPlayerID(p.ID)
		}
		s.clients[c.ID] = c
	}
	s.sendGameState()
	for id, c := range s.clients {
		var update gameUpdate
		select {
		case msg := <-c.send:
			if err := json.Unmarshal(msg.Data.(json.RawMessage), &update); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("no snapshot delivered")
		}
		known := id == 11
		if known {
			if update.Planets[0].Armies != 23 || update.Players[1] == nil || update.Players[1].X != enemy.X {
				t.Fatal("team lost known state")
			}
		} else if update.Planets[0].Owner != game.TeamNone || update.Planets[0].Armies != 0 || update.Planets[0].Flags != 0 || update.Planets[0].Info != 0 || update.Players[1] != nil {
			t.Fatal("snapshot disclosed hidden planet or cloaked enemy")
		}
		if update.Planets[0].Name != planet.Name || update.Planets[0].X != planet.X {
			t.Fatal("public planet location was lost")
		}
	}
	if planet.Armies != 23 || planet.Flags == 0 || !enemy.Cloaked {
		t.Fatal("snapshot mutated authoritative state")
	}
	if len(s.broadcast) != 0 {
		t.Fatal("shared broadcast leaked a game snapshot")
	}
	planet.Info |= game.TeamFed
	if s.snapshotForTeam(game.TeamFed).Planets[0].Armies != 23 {
		t.Fatal("scouting did not reveal planet")
	}
	s.gameState.T_mode = false
	if s.snapshotForTeam(game.TeamNone).Planets[0].Armies != 23 {
		t.Fatal("practice planets should be public")
	}
}
