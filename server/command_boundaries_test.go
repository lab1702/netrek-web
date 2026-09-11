package server

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestHiddenTargetRejectsAutoaimButAllowsBlindPhasers(t *testing.T) {
	s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.X, p.Y = 50000, 50000
	target := s.gameState.Players[1]
	target.Status, target.Team, target.Ship, target.Cloaked = game.StatusAlive, game.TeamRom, game.ShipCruiser, true
	target.X, target.Y = p.X, p.Y+3000
	c.handlePhaser(json.RawMessage(`{"target":1,"dir":0}`))
	if target.Damage != 0 {
		t.Fatal("target-ID phaser autoaimed at a hidden enemy")
	}
	c.handlePhaser(json.RawMessage(`{"target":-1,"dir":0}`))
	if target.Damage != 0 {
		t.Fatal("directional shot aimed away from enemy hit")
	}
	data, _ := json.Marshal(PhaserData{Target: -1, Dir: math.Pi / 2})
	c.handlePhaser(data)
	if target.Damage == 0 {
		t.Fatal("legitimate blind shot could not hit cloaked enemy")
	}
}

func TestRepairRequestCancelsPlanetNavigation(t *testing.T) {
	s, c, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.X, p.Y, p.Speed, p.DesSpeed = 50000, 50000, 7, 7
	p.Damage, p.LockType, p.LockTarget = 20, "planet", 0
	c.handleRepair(nil)
	for i := 0; i < 100; i++ {
		s.updatePlayerSystems(p, p.ID)
		s.updatePlayerPhysics(p, p.ID)
		s.updatePlayerLockOn(p)
	}
	if p.LockType != "none" || p.LockTarget != -1 || p.Speed != 0 || p.RepairRequest || (!p.Repairing && p.Damage != 0) {
		t.Fatalf("repair failed to take over navigation: speed=%v request=%v repair=%v lock=%s", p.Speed, p.RepairRequest, p.Repairing, p.LockType)
	}
}

func TestProjectileFinalTickCanHitAndExpiresOnce(t *testing.T) {
	for _, plasma := range []bool{false, true} {
		for _, hit := range []bool{false, true} {
			s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
			target := s.gameState.Players[1]
			target.Status, target.Team, target.Ship = game.StatusAlive, game.TeamRom, game.ShipCruiser
			radius := game.ExplosionDist
			if plasma {
				radius = game.PlasmaExplosionDist
			}
			target.X, target.Y = float64(10000+240+radius-1), 10000
			if !hit {
				target.Y += 10000
			}
			shot := &game.Torpedo{Owner: p.ID, Team: p.Team, X: 10000, Y: 10000, Speed: 240, Fuse: 1, Status: game.TorpMove, Damage: 40}
			if plasma {
				s.gameState.Plasmas = []*game.Plasma{shot}
				p.NumPlasma = 1
			} else {
				s.gameState.Torps = []*game.Torpedo{shot}
				p.NumTorps = 1
			}
			s.updateProjectiles()
			if hit && (target.Damage != 40 || shot.Status != game.TorpDet) {
				t.Fatalf("last tick missed: damage=%d status=%d", target.Damage, shot.Status)
			}
			if !hit && target.Damage != 0 {
				t.Fatal("expired projectile damaged distant target")
			}
			s.updateProjectiles()
			if len(s.gameState.Torps) != 0 || len(s.gameState.Plasmas) != 0 || p.NumTorps != 0 || p.NumPlasma != 0 {
				t.Fatal("last-tick projectile was not cleaned up once")
			}
		}
	}
}
