package server

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// --- #2: angle-wrap (single-fold) bug in bot facing/heading checks ---

// An enemy facing nearly opposite to the bot must NOT trigger evasion. The
// buggy single-fold normalization can turn a ~167° separation into a small
// (or negative) angleDiff that passes the "< π/6" facing test.
func TestAssessUniversalThreatsFacingAwayNoEvasion(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 100)}
	gs.Frame = 1

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.Ship = game.ShipCruiser
	bot.X = 49010
	bot.Y = 49859

	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipCruiser
	enemy.X = 50000
	enemy.Y = 50000
	enemy.Dir = 6.2 // ~355°, facing away from the bot (which is at angle ~ -3.0 rad from enemy)
	enemy.Cloaked = false

	threat := server.assessUniversalThreats(bot)
	if threat.requiresEvasion {
		t.Errorf("expected no evasion: enemy faces ~167° away from bot, but requiresEvasion=true (angle-wrap bug)")
	}
}

// An enemy heading away from a planet must NOT be counted as a planet threat.
func TestGetPlanetThreatsEnemyHeadingAwayNotThreat(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 100)}
	gs.Frame = 1

	// Make every planet neutral (skipped) except planet 0.
	for _, pl := range gs.Planets {
		pl.Owner = game.TeamNone
	}
	planet := gs.Planets[0]
	planet.Owner = game.TeamFed
	planet.X = 42080
	planet.Y = 48871

	enemy := gs.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipCruiser
	enemy.X = 50000
	enemy.Y = 50000
	enemy.Speed = 2 // > 1 so the heading check runs
	enemy.Dir = 6.2 // heading away from the planet (planet is at ~ -3.0 rad from enemy)
	enemy.Cloaked = false

	threats := server.getPlanetThreats()
	if pt, ok := threats[0]; ok && pt.closestEnemy != nil {
		t.Errorf("expected planet 0 to have no threat: enemy heads ~167° away, but closestEnemy set (angle-wrap bug)")
	}
}

// --- #1: tournament mode must not flicker off when a player is exploding ---

func TestTournamentModeStaysActiveWhenPlayerExploding(t *testing.T) {
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 100)
	if server.gameState.TournamentStats == nil {
		server.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)
	}
	server.gameState.Frame = 200

	// 4 Fed + 4 Rom, all alive and connected.
	for i := 0; i < 8; i++ {
		p := server.gameState.Players[i]
		p.Status = game.StatusAlive
		p.Connected = true
		p.Ship = game.ShipCruiser
		if i < 4 {
			p.Team = game.TeamFed
		} else {
			p.Team = game.TeamRom
		}
	}

	server.checkTournamentMode()
	if !server.gameState.T_mode {
		t.Fatal("expected tournament mode to activate with 4v4")
	}

	// One Fed player dies and is mid-explosion (still connected).
	server.gameState.Players[0].Status = game.StatusExplode
	server.checkTournamentMode()
	if !server.gameState.T_mode {
		t.Error("tournament mode deactivated because an exploding player was not counted (flicker bug)")
	}
}

// --- #4: tournament stats backfilled for players who join after T-mode entry ---

func TestTournamentStatsBackfilledForLateJoiner(t *testing.T) {
	server := NewServer()
	server.broadcast = make(chan ServerMessage, 100)
	if server.gameState.TournamentStats == nil {
		server.gameState.TournamentStats = make(map[int]*game.TournamentPlayerStats)
	}
	server.gameState.Frame = 200

	for i := 0; i < 8; i++ {
		p := server.gameState.Players[i]
		p.Status = game.StatusAlive
		p.Connected = true
		p.Ship = game.ShipCruiser
		if i < 4 {
			p.Team = game.TeamFed
		} else {
			p.Team = game.TeamRom
		}
	}
	server.checkTournamentMode()
	if !server.gameState.T_mode {
		t.Fatal("expected tournament mode active")
	}

	// A new player joins after the tournament started.
	late := server.gameState.Players[8]
	late.Status = game.StatusAlive
	late.Connected = true
	late.Team = game.TeamFed
	late.Ship = game.ShipCruiser

	server.checkTournamentMode()
	if _, ok := server.gameState.TournamentStats[8]; !ok {
		t.Error("late joiner has no TournamentStats entry; their kills/damage would be silently dropped")
	}
}

// --- #3: non-lethal phaser hits credit tournament damage ---

func TestPhaserNonLethalCreditsTournamentDamage(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.X = 50000
	p.Y = 50000

	enemy := server.gameState.Players[1]
	enemy.Status = game.StatusAlive
	enemy.Team = game.TeamRom
	enemy.Ship = game.ShipCruiser
	enemy.X = 52000
	enemy.Y = 50000
	shipStats := game.ShipData[game.ShipCruiser]
	enemy.Shields = shipStats.MaxShields
	enemy.Damage = 0

	server.gameState.T_mode = true
	server.gameState.TournamentStats = map[int]*game.TournamentPlayerStats{
		0: {},
		1: {},
	}

	client.handlePhaser(json.RawMessage(`{"target":1,"dir":0}`))

	if server.gameState.TournamentStats[0].DamageDealt <= 0 {
		t.Error("attacker DamageDealt not credited for non-lethal phaser hit in tournament mode")
	}
	if server.gameState.TournamentStats[1].DamageTaken <= 0 {
		t.Error("target DamageTaken not credited for non-lethal phaser hit in tournament mode")
	}
}

// --- #5: beam-down must not push planet armies past the cap ---

func TestBeamDownCapsPlanetArmies(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 100)}
	gs.Frame = 5 // Frame%5==0 so continuous beaming runs

	planet := gs.Planets[0]
	planet.Owner = game.TeamFed
	planet.Armies = maxPlanetArmies // already at cap

	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipAssault
	p.Armies = 5
	p.Orbiting = 0
	p.Beaming = true
	p.BeamingUp = false

	server.updateOrbitingPlayer(p, 0)

	if planet.Armies > maxPlanetArmies {
		t.Errorf("beam-down exceeded cap: planet.Armies=%d > %d", planet.Armies, maxPlanetArmies)
	}
}

// --- #6: detonating an enemy torp must neutralize it in place, not let it
// travel and run its collision check one more tick ---

func TestDetonateMarksEnemyTorpForExplosion(t *testing.T) {
	server, client, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
	p.X = 50000
	p.Y = 50000
	p.Fuel = 30000

	torp := &game.Torpedo{
		Owner:  1,
		Team:   game.TeamRom,
		Status: game.TorpMove,
		X:      51000,
		Y:      50000,
		Dir:    math.Pi, // heading toward the player
		Speed:  200,
		Fuse:   20,
	}
	server.gameState.Torps = []*game.Torpedo{torp}

	client.handleDetonate(json.RawMessage(`{}`))

	if torp.Status != game.TorpDet {
		t.Errorf("expected detonated enemy torp to be marked TorpDet (so it stops); got Status=%d", torp.Status)
	}
}

// --- #11: navigation intercept must use the bot's actual speed, not its
// commanded (desired) speed, when predicting the lead point ---

func TestEnhancedInterceptUsesActualSpeed(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 100)}

	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipCruiser
	p.X = 50000
	p.Y = 50000
	p.Speed = 2     // actual speed (slow)
	p.DesSpeed = 12 // commanded speed (much faster) — must NOT be used

	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamRom
	target.Ship = game.ShipCruiser
	target.X = 50500
	target.Y = 50000
	target.Dir = math.Pi / 2 // crossing north
	target.Speed = 5
	target.DesSpeed = 5

	got := server.calculateEnhancedInterceptCourse(p, target)

	// Compute the expected lead using ACTUAL speed.
	mySpeed := math.Max(p.Speed, 2) * 20
	tti := 500.0 / mySpeed
	if tti > 15 {
		tti = 15
	}
	predictX := target.X + target.Speed*math.Cos(target.Dir)*tti*20
	predictY := target.Y + target.Speed*math.Sin(target.Dir)*tti*20
	want := math.Atan2(predictY-p.Y, predictX-p.X)

	if math.Abs(got-want) > 0.01 {
		t.Errorf("intercept course used commanded speed, not actual: got %.4f, want %.4f", got, want)
	}
}

// --- #8: coordinated team-volley cooldown must survive weapon firing ---

func TestCoordinatedCooldownSurvivesWeaponFire(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 100)}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.Ship = game.ShipCruiser
	bot.IsBot = true
	bot.X = 50000
	bot.Y = 50000
	bot.Dir = 0
	bot.Fuel = 30000
	bot.NumTorps = 0
	bot.WTemp = 0
	bot.Orbiting = -1

	ally := gs.Players[1]
	ally.Status = game.StatusAlive
	ally.Team = game.TeamFed
	ally.Ship = game.ShipCruiser
	ally.IsBot = true
	ally.BotTarget = 2 // same target as the bot will engage
	ally.BotCooldown = 25

	target := gs.Players[2]
	target.Status = game.StatusAlive
	target.Team = game.TeamRom
	target.Ship = game.ShipCruiser
	target.X = 51000
	target.Y = 50000
	target.Shields = game.ShipData[game.ShipCruiser].MaxShields
	target.Damage = 0

	server.engageCombat(bot, target, 1000)

	if bot.BotCooldown != 25 {
		t.Errorf("coordinated cooldown (25) was overwritten by weapon-fire cooldown; got %d", bot.BotCooldown)
	}
}

// --- #12: AddBot reports success/failure so /fillbots doesn't count rejects ---

func TestAddBotReturnsTrueOnSuccess(t *testing.T) {
	server := NewServer()
	if !server.AddBot(game.TeamFed, game.ShipCruiser) {
		t.Fatal("AddBot should return true when a bot is successfully added")
	}
	bots := 0
	for _, p := range server.gameState.Players {
		if p.IsBot && p.Status != game.StatusFree {
			bots++
		}
	}
	if bots != 1 {
		t.Errorf("expected exactly 1 bot after a successful AddBot, got %d", bots)
	}
}

func TestAddBotReturnsFalseWhenStarbaseLimitReached(t *testing.T) {
	server := NewServer()
	if !server.AddBot(game.TeamFed, game.ShipStarbase) {
		t.Fatal("first starbase should be added successfully")
	}
	if server.AddBot(game.TeamFed, game.ShipStarbase) {
		t.Error("AddBot should return false when the team already has a starbase")
	}
}

func TestAddBotReturnsFalseWhenServerFull(t *testing.T) {
	server := NewServer()
	// Occupy every slot.
	for _, p := range server.gameState.Players {
		p.Status = game.StatusAlive
		p.Connected = true
	}
	if server.AddBot(game.TeamFed, game.ShipCruiser) {
		t.Error("AddBot should return false when there are no free slots")
	}
}
