package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestSelectCombatManeuver(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.Ship = game.ShipCruiser
	bot.Speed = 6
	bot.X, bot.Y = 50000, 50000

	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamRom
	target.Ship = game.ShipCruiser
	target.Speed = 6
	target.X, target.Y = 53000, 50000

	interceptDir := math.Atan2(target.Y-bot.Y, target.X-bot.X)

	t.Run("Close range returns circle-strafe or intercept", func(t *testing.T) {
		// At close range with equal ships, maneuver depends on turn rate advantage
		m := server.SelectCombatManeuver(bot, target, 2500, interceptDir)
		if m.maneuver != "circle-strafe" && m.maneuver != "intercept" {
			t.Errorf("got maneuver %q at close range, want circle-strafe or intercept", m.maneuver)
		}
	})

	t.Run("Long range returns intercept or offset-approach", func(t *testing.T) {
		m := server.SelectCombatManeuver(bot, target, 8000, interceptDir)
		if m.maneuver != "intercept" && m.maneuver != "offset-approach" {
			t.Errorf("got maneuver %q at long range, want intercept or offset-approach", m.maneuver)
		}
	})

	t.Run("Dead target returns idle", func(t *testing.T) {
		target.Status = game.StatusDead
		m := server.SelectCombatManeuver(bot, target, 3000, interceptDir)
		if m.maneuver != "idle" {
			t.Errorf("got maneuver %q for dead target, want idle", m.maneuver)
		}
		target.Status = game.StatusAlive
	})

	t.Run("Speed advantage triggers boom-zoom at close range", func(t *testing.T) {
		// Scout (fast) vs Battleship (slow) — scout turns slower at speed
		// so maneuverAdvantage may be negative, triggering boom-zoom if speedAdvantage > 0
		bot.Ship = game.ShipScout
		bot.Speed = 10
		target.Ship = game.ShipBattleship
		target.Speed = 5
		m := server.SelectCombatManeuver(bot, target, 2000, interceptDir)
		// Scout at speed 10 has low turn rate, so maneuverAdvantage <= 0,
		// but speedAdvantage > 0, so boom-zoom
		if m.maneuver != "boom-zoom" && m.maneuver != "intercept" {
			t.Errorf("got maneuver %q for fast-vs-slow close range, want boom-zoom or intercept", m.maneuver)
		}
		bot.Ship = game.ShipCruiser
		bot.Speed = 6
		target.Ship = game.ShipCruiser
		target.Speed = 6
	})
}

func TestIsTorpedoThreatening(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.Ship = game.ShipCruiser
	bot.Speed = 5
	bot.Dir = 0 // Facing east
	bot.X, bot.Y = 50000, 50000

	t.Run("Torpedo heading directly at bot is threatening", func(t *testing.T) {
		torp := &game.Torpedo{
			Owner:  1,
			Team:   game.TeamRom,
			X:      52000,
			Y:      50000,
			Dir:    math.Pi, // Heading west (toward bot)
			Speed:  300,
			Status: game.TorpMove,
		}
		if !server.IsTorpedoThreatening(bot, torp) {
			t.Error("torpedo heading directly at bot should be threatening")
		}
	})

	t.Run("Torpedo heading away is not threatening", func(t *testing.T) {
		torp := &game.Torpedo{
			Owner:  1,
			Team:   game.TeamRom,
			X:      52000,
			Y:      50000,
			Dir:    0, // Heading east (away from bot)
			Speed:  300,
			Status: game.TorpMove,
		}
		if server.IsTorpedoThreatening(bot, torp) {
			t.Error("torpedo heading away should not be threatening")
		}
	})

	t.Run("Very close torpedo is always threatening", func(t *testing.T) {
		torp := &game.Torpedo{
			Owner:  1,
			Team:   game.TeamRom,
			X:      50800,
			Y:      50000,
			Dir:    math.Pi / 2, // Heading north (perpendicular)
			Speed:  300,
			Status: game.TorpMove,
		}
		if !server.IsTorpedoThreatening(bot, torp) {
			t.Error("very close torpedo should be threatening regardless of heading")
		}
	})

	t.Run("Far torpedo is not threatening", func(t *testing.T) {
		torp := &game.Torpedo{
			Owner:  1,
			Team:   game.TeamRom,
			X:      60000,
			Y:      50000,
			Dir:    math.Pi,
			Speed:  300,
			Status: game.TorpMove,
		}
		if server.IsTorpedoThreatening(bot, torp) {
			t.Error("torpedo beyond 5000 units should not be threatening")
		}
	})

	t.Run("Torpedo at 50° off-axis is not threatening after tightening", func(t *testing.T) {
		// After tightening from 72° to 45°, a torpedo at 50° off-axis should not trigger
		angle := math.Pi + 50.0*math.Pi/180.0 // 50° off direct heading to bot
		torp := &game.Torpedo{
			Owner:  1,
			Team:   game.TeamRom,
			X:      53000,
			Y:      50000,
			Dir:    angle,
			Speed:  300,
			Status: game.TorpMove,
		}
		if server.IsTorpedoThreatening(bot, torp) {
			t.Error("torpedo at 50° off-axis should not be threatening with 45° threshold")
		}
	})
}

func TestCoordinateTeamAttack(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.IsBot = true
	bot.BotCooldown = 3

	ally1 := gs.Players[1]
	ally1.Status = game.StatusAlive
	ally1.Team = game.TeamFed
	ally1.IsBot = true

	ally2 := gs.Players[2]
	ally2.Status = game.StatusAlive
	ally2.Team = game.TeamFed
	ally2.IsBot = true

	target := gs.Players[3]
	target.Status = game.StatusAlive
	target.Team = game.TeamRom

	t.Run("No allies targeting same enemy returns -1", func(t *testing.T) {
		ally1.BotTarget = -1
		ally2.BotTarget = -1
		result := server.CoordinateTeamAttack(bot, target)
		if result != -1 {
			t.Errorf("got %d, want -1 (no coordination needed)", result)
		}
	})

	t.Run("Returns max ally cooldown for volley fire", func(t *testing.T) {
		ally1.BotTarget = target.ID
		ally1.BotCooldown = 5
		ally2.BotTarget = target.ID
		ally2.BotCooldown = 8
		result := server.CoordinateTeamAttack(bot, target)
		if result != 8 {
			t.Errorf("got %d, want 8 (max ally cooldown)", result)
		}
	})

	t.Run("Single ally returns that ally's cooldown", func(t *testing.T) {
		ally1.BotTarget = target.ID
		ally1.BotCooldown = 4
		ally2.BotTarget = -1 // Not targeting same
		result := server.CoordinateTeamAttack(bot, target)
		if result != 4 {
			t.Errorf("got %d, want 4", result)
		}
	})

	t.Run("Zero cooldown allies return 0", func(t *testing.T) {
		ally1.BotTarget = target.ID
		ally1.BotCooldown = 0
		ally2.BotTarget = target.ID
		ally2.BotCooldown = 0
		result := server.CoordinateTeamAttack(bot, target)
		if result != 0 {
			t.Errorf("got %d, want 0 (all allies ready to fire)", result)
		}
	})
}

func TestDetonatePassingTorpedoes(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed

	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.X, enemy.Y = 52000, 50000

	t.Run("Torpedo passing enemy is detonated", func(t *testing.T) {
		// Place torpedo 1500 units west of enemy, heading north (perpendicular)
		// Distance = 1500, which is > 800 and < 2500, so detonation zone applies
		torp := &game.Torpedo{
			Owner:  bot.ID,
			Team:   game.TeamFed,
			X:      50500,
			Y:      50000,
			Dir:    math.Pi / 2, // Heading north (perpendicular to enemy)
			Speed:  300,
			Status: game.TorpMove,
			Fuse:   50,
		}
		gs.Torps = []*game.Torpedo{torp}
		bot.NumTorps = 1

		server.DetonatePassingTorpedoes(bot)
		if torp.Fuse != 1 {
			t.Errorf("torpedo passing by enemy should be detonated (fuse=1), got fuse=%d", torp.Fuse)
		}
	})

	t.Run("Torpedo heading directly at enemy is not detonated", func(t *testing.T) {
		dirToEnemy := math.Atan2(enemy.Y-50000, enemy.X-50500)
		torp := &game.Torpedo{
			Owner:  bot.ID,
			Team:   game.TeamFed,
			X:      50500,
			Y:      50000,
			Dir:    dirToEnemy, // Heading straight at enemy
			Speed:  300,
			Status: game.TorpMove,
			Fuse:   50,
		}
		gs.Torps = []*game.Torpedo{torp}
		bot.NumTorps = 1

		server.DetonatePassingTorpedoes(bot)
		if torp.Fuse == 1 {
			t.Error("torpedo heading directly at enemy should not be detonated early")
		}
	})

	t.Run("No torpedoes does nothing", func(t *testing.T) {
		gs.Torps = nil
		bot.NumTorps = 0
		// Should not panic
		server.DetonatePassingTorpedoes(bot)
	})
}

func TestPhaserRangeHelper(t *testing.T) {
	for ship := game.ShipScout; ship <= game.ShipStarbase; ship++ {
		stats := game.ShipData[ship]
		got := game.PhaserRange(stats)
		want := float64(game.PhaserDist) * float64(stats.PhaserDamage) / 100.0
		if got != want {
			t.Errorf("PhaserRange(%s) = %f, want %f", stats.Name, got, want)
		}
	}
}
