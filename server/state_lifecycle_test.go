package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestReusedBotSlotStartsFresh(t *testing.T) {
	s := NewServer()
	p := s.gameState.Players[0]
	p.Orbiting, p.Tractoring, p.Pressoring = 4, 2, 3
	p.Cloaked, p.Repairing, p.RepairRequest = true, true, true
	p.Bombing, p.Beaming, p.BeamingUp = true, true, true
	p.Armies, p.KillsStreak, p.Kills, p.Deaths = 5, 8, 12, 4
	p.LockType, p.LockTarget = "planet", 4
	p.OwnerClientID, p.BotHitTimer, p.BotHasGoal = 42, 20, true
	s.gameState.TournamentStats[0] = &game.TournamentPlayerStats{Kills: 12}
	if !s.AddBot(game.TeamRom, game.ShipCruiser) {
		t.Fatal("bot was not added")
	}
	if p.Orbiting != -1 || p.Tractoring != -1 || p.Pressoring != -1 || p.LockType != "none" || p.LockTarget != -1 {
		t.Fatalf("new bot inherited navigation/beam state: %+v", p)
	}
	if p.Cloaked || p.Repairing || p.RepairRequest || p.Bombing || p.Beaming || p.BeamingUp || p.BotHasGoal || p.BotHitTimer != 0 {
		t.Fatal("new bot inherited action state")
	}
	if p.Armies != 0 || p.Kills != 0 || p.KillsStreak != 0 || p.Deaths != 0 || p.OwnerClientID != -1 {
		t.Fatal("new bot inherited pilot state")
	}
	if _, ok := s.gameState.TournamentStats[0]; ok {
		t.Fatal("new bot inherited tournament statistics")
	}
	x, y := p.X, p.Y
	s.updatePlayerOrbit(p)
	if p.X != x || p.Y != y {
		t.Fatal("new bot teleported to previous pilot's orbit")
	}
}

func TestProjectileCountersAcrossRespawnAndSlotReuse(t *testing.T) {
	for _, plasma := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			name := "torpedo"
			if plasma {
				name = "plasma"
			}
			if reuse {
				name += "/new-pilot"
			} else {
				name += "/respawn"
			}
			t.Run(name, func(t *testing.T) {
				s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
				fire := func() {
					if plasma {
						c.handlePlasma(json.RawMessage(`{"dir":0}`))
					} else {
						c.handleFire(json.RawMessage(`{"dir":0}`))
					}
				}
				list := func() []*game.Torpedo {
					if plasma {
						return s.gameState.Plasmas
					}
					return s.gameState.Torps
				}
				count := func() int {
					if plasma {
						return p.NumPlasma
					}
					return p.NumTorps
				}
				fire()
				if len(list()) != 1 {
					t.Fatal("initial shot failed")
				}
				old := list()[0]
				old.Fuse, old.X, old.Y = 1, 50000, 50000
				if reuse {
					p.OwnerClientID = c.ID
					if !s.freeDisconnectedSlot(c.ID, p.ID) {
						t.Fatal("disconnect failed")
					}
					c.SetPlayerID(-1)
					c.handleLogin(json.RawMessage(`{"name":"Replacement","team":2,"ship":2}`))
					if c.GetPlayerID() != p.ID {
						t.Fatal("replacement login failed")
					}
				} else {
					p.Status = game.StatusDead
					s.respawnPlayer(p)
				}
				fire()
				if len(list()) != 2 || count() != 1 {
					t.Fatal("new life shot failed")
				}
				if plasma {
					s.updatePlasmas()
				} else {
					s.updateTorpedoes()
				}
				if len(list()) != 1 || count() != 1 {
					t.Fatalf("old shot changed new counter: shots=%d count=%d", len(list()), count())
				}
				if plasma {
					fire()
					if len(list()) != 1 {
						t.Fatal("old shot allowed an extra plasma")
					}
				}
			})
		}
	}
}

func TestProjectileKeepsLaunchTeamAndPilotAfterSlotReuse(t *testing.T) {
	for _, scenario := range []string{"original-ally", "replacement-ally", "replacement-pilot"} {
		t.Run(scenario, func(t *testing.T) {
			s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
			c.handleFire(json.RawMessage(`{"dir":0}`))
			shot := s.gameState.Torps[0]
			shot.X, shot.Y, shot.Speed = 50000, 50000, 0
			p.OwnerClientID = c.ID
			s.freeDisconnectedSlot(c.ID, p.ID)
			c.SetPlayerID(-1)
			c.handleLogin(json.RawMessage(`{"name":"Replacement","team":2,"ship":2}`))
			if c.GetPlayerID() != p.ID {
				t.Fatal("replacement login failed")
			}
			target := s.gameState.Players[1]
			target.Team = game.TeamRom
			if scenario == "original-ally" {
				target.Team = game.TeamFed
			}
			if scenario == "replacement-pilot" {
				target = p
			}
			target.Status, target.Connected, target.Ship = game.StatusAlive, true, game.ShipCruiser
			target.X, target.Y, target.Shields, target.Damage = shot.X, shot.Y, 0, 99
			s.gameState.T_mode = true
			s.gameState.TournamentStats[p.ID] = &game.TournamentPlayerStats{}
			s.updateProjectiles()
			if scenario == "original-ally" {
				if target.Damage != 99 {
					t.Fatal("old shot damaged its original ally")
				}
			} else if target.Status != game.StatusExplode || target.KilledBy != -1 {
				t.Fatalf("old enemy shot did not hit without crediting replacement: status=%d killer=%d", target.Status, target.KilledBy)
			}
			if p.Kills != 0 || s.gameState.TournamentStats[p.ID].Kills != 0 || s.gameState.TournamentStats[p.ID].DamageDealt != 0 {
				t.Fatal("replacement received old pilot's weapon credit")
			}
		})
	}
}

func TestDeferredRepairEntersWithShieldsAndActionsDisabled(t *testing.T) {
	s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.Speed, p.DesSpeed, p.Damage = 4, 4, 50
	p.Shields_up, p.Bombing, p.Beaming, p.BeamingUp = true, true, true, true
	p.Tractoring, p.Pressoring = 1, 2
	p.LockType, p.LockTarget = "planet", 3
	c.handleRepair(nil)
	if !p.RepairRequest || p.DesSpeed != 0 {
		t.Fatal("moving repair did not request a stop")
	}
	p.Speed = 0
	s.updatePlayerSystems(p, p.ID)
	if !p.Repairing || p.RepairRequest || p.Shields_up || p.Bombing || p.Beaming || p.BeamingUp || p.Tractoring != -1 || p.Pressoring != -1 || p.LockType != "none" {
		t.Fatal("deferred repair retained conflicting actions")
	}
	for i := 0; i < 20; i++ {
		s.updatePlayerSystems(p, p.ID)
	}
	if p.Damage >= 50 {
		t.Fatal("deferred repair failed to repair hull")
	}
}

func TestCloakCancelsActiveBeams(t *testing.T) {
	for _, pressor := range []bool{false, true} {
		s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		target := s.gameState.Players[1]
		target.Status, target.Ship = game.StatusAlive, game.ShipCruiser
		target.X, target.Y = p.X+1000, p.Y
		if pressor {
			p.Pressoring = 1
		} else {
			p.Tractoring = 1
		}
		c.handleCloak(nil)
		if !p.Cloaked || p.Tractoring != -1 || p.Pressoring != -1 {
			t.Fatal("cloak retained beam")
		}
		// The simulation also rejects a beam set by another internal path.
		if pressor {
			p.Pressoring = 1
		} else {
			p.Tractoring = 1
		}
		x, y := target.X, target.Y
		s.updateTractorBeams()
		if target.X != x || target.Y != y || p.Tractoring != -1 || p.Pressoring != -1 {
			t.Fatal("cloaked beam applied force")
		}
	}
}

func TestRepairingStarbaseCanEnterCombat(t *testing.T) {
	for _, defense := range []bool{false, true} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipStarbase)
		p.IsBot, p.Repairing, p.RepairRequest = true, true, true
		p.Damage, p.RepairCounter = 100, 3
		enemy := s.gameState.Players[1]
		enemy.Status, enemy.Team, enemy.Ship = game.StatusAlive, game.TeamRom, game.ShipCruiser
		enemy.X, enemy.Y = p.X+1500, p.Y
		if defense {
			s.starbaseDefendPlanet(p, s.gameState.Planets[0], enemy, 1500)
		} else {
			s.starbaseDefensiveCombat(p, enemy, 1500)
		}
		if p.Repairing || p.RepairRequest || p.RepairCounter != 0 {
			t.Fatal("combat retained repair state")
		}
		if len(s.gameState.Torps) == 0 && len(s.gameState.Plasmas) == 0 && enemy.Damage == 0 {
			t.Fatal("starbase failed to fire")
		}
	}
}

func TestChatTextPreservesCharactersAndRuneLimit(t *testing.T) {
	text := "I'm ready & waiting <here>"
	if got := sanitizeText(text); got != text {
		t.Fatalf("chat text changed: %q", got)
	}
	if got := sanitizeText(strings.Repeat("界", 501)); got != strings.Repeat("界", 500) {
		t.Fatal("chat rune limit failed")
	}
}
