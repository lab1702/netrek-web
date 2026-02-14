package server

import (
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestSelectBotBehavior(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.IsBot = true

	t.Run("Losing badly returns defender when few defenders", func(t *testing.T) {
		// Give Fed team very few planets (< 20% of 40 = less than 8)
		for i := range gs.Planets {
			gs.Planets[i].Owner = game.TeamRom
		}
		// Give Fed just 2 planets (5%)
		gs.Planets[0].Owner = game.TeamFed
		gs.Planets[1].Owner = game.TeamFed

		// No other bots defending
		for i := 1; i < len(gs.Players); i++ {
			gs.Players[i].Status = game.StatusDead
		}

		server.cachedTeamPlanetsFrame = 0 // Clear cache
		role := server.SelectBotBehavior(bot)
		if role != BotRoleDefender {
			t.Errorf("got %q, want %q when losing badly with few defenders", role, BotRoleDefender)
		}
	})

	t.Run("Losing badly returns raider when enough defenders", func(t *testing.T) {
		// Keep Fed at < 20%
		for i := range gs.Planets {
			gs.Planets[i].Owner = game.TeamRom
		}
		gs.Planets[0].Owner = game.TeamFed
		gs.Planets[1].Owner = game.TeamFed

		// Add 2 defending bots
		gs.Players[1].Status = game.StatusAlive
		gs.Players[1].Team = game.TeamFed
		gs.Players[1].IsBot = true
		gs.Players[1].BotDefenseTarget = 5

		gs.Players[2].Status = game.StatusAlive
		gs.Players[2].Team = game.TeamFed
		gs.Players[2].IsBot = true
		gs.Players[2].BotDefenseTarget = 6

		server.cachedTeamPlanetsFrame = 0
		role := server.SelectBotBehavior(bot)
		if role != BotRoleRaider {
			t.Errorf("got %q, want %q when losing with enough defenders", role, BotRoleRaider)
		}

		// Cleanup
		gs.Players[1].Status = game.StatusDead
		gs.Players[2].Status = game.StatusDead
	})

	t.Run("Winning returns hunter", func(t *testing.T) {
		// Give Fed > 60% of planets
		for i := range gs.Planets {
			gs.Planets[i].Owner = game.TeamFed
		}
		// Give Rom just a few
		for i := 35; i < 40; i++ {
			gs.Planets[i].Owner = game.TeamRom
		}

		server.cachedTeamPlanetsFrame = 0
		role := server.SelectBotBehavior(bot)
		if role != BotRoleHunter {
			t.Errorf("got %q, want %q when winning", role, BotRoleHunter)
		}
	})

	t.Run("Balanced with kill streak returns raider", func(t *testing.T) {
		// 50% planets each (balanced)
		for i := range gs.Planets {
			if i < 20 {
				gs.Planets[i].Owner = game.TeamFed
			} else {
				gs.Planets[i].Owner = game.TeamRom
			}
		}

		// Bot has enough kills to carry armies
		bot.KillsStreak = game.ArmyKillRequirement

		server.cachedTeamPlanetsFrame = 0
		role := server.SelectBotBehavior(bot)
		if role != BotRoleRaider {
			t.Errorf("got %q, want %q when balanced with kill streak", role, BotRoleRaider)
		}
		bot.KillsStreak = 0
	})

	t.Run("Balanced with too many hunters returns defender", func(t *testing.T) {
		// 50% planets
		for i := range gs.Planets {
			if i < 20 {
				gs.Planets[i].Owner = game.TeamFed
			} else {
				gs.Planets[i].Owner = game.TeamRom
			}
		}

		// Add hunters with no defenders
		gs.Players[1].Status = game.StatusAlive
		gs.Players[1].Team = game.TeamFed
		gs.Players[1].IsBot = true
		gs.Players[1].BotTarget = 10
		gs.Players[1].BotDefenseTarget = -1

		gs.Players[2].Status = game.StatusAlive
		gs.Players[2].Team = game.TeamFed
		gs.Players[2].IsBot = true
		gs.Players[2].BotTarget = 11
		gs.Players[2].BotDefenseTarget = -1

		bot.BotTarget = -1
		bot.BotDefenseTarget = -1
		bot.KillsStreak = 0

		server.cachedTeamPlanetsFrame = 0
		role := server.SelectBotBehavior(bot)
		if role != BotRoleDefender {
			t.Errorf("got %q, want %q when too many hunters vs defenders", role, BotRoleDefender)
		}

		gs.Players[1].Status = game.StatusDead
		gs.Players[2].Status = game.StatusDead
	})
}

func TestBroadcastTargetToAllies(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	bot := gs.Players[0]
	bot.Status = game.StatusAlive
	bot.Team = game.TeamFed
	bot.IsBot = true
	bot.X, bot.Y = 50000, 50000

	target := gs.Players[5]
	target.Status = game.StatusAlive
	target.Team = game.TeamRom

	ally := gs.Players[1]
	ally.Status = game.StatusAlive
	ally.Team = game.TeamFed
	ally.IsBot = true
	ally.BotTarget = -1

	t.Run("High-value target broadcasts to nearby untargeted ally", func(t *testing.T) {
		ally.X, ally.Y = 55000, 50000 // 5000 units away (within BroadcastTargetRange)
		ally.BotTarget = -1

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, BroadcastTargetMinValue+1000)

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 1 {
			t.Fatalf("got %d suggestions, want 1", len(suggestions))
		}
		if suggestions[0].allyID != ally.ID {
			t.Errorf("suggestion allyID = %d, want %d", suggestions[0].allyID, ally.ID)
		}
		if suggestions[0].targetID != target.ID {
			t.Errorf("suggestion targetID = %d, want %d", suggestions[0].targetID, target.ID)
		}
	})

	t.Run("Low-value non-carrier does not broadcast", func(t *testing.T) {
		ally.BotTarget = -1
		target.Armies = 0

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, BroadcastTargetMinValue-1000)

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 0 {
			t.Errorf("got %d suggestions for low-value non-carrier, want 0", len(suggestions))
		}
	})

	t.Run("Carrier always broadcasts regardless of value", func(t *testing.T) {
		ally.BotTarget = -1
		target.Armies = 3

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, 1000) // Low value but carrier

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 1 {
			t.Fatalf("got %d suggestions for carrier, want 1", len(suggestions))
		}
		target.Armies = 0
	})

	t.Run("Does not broadcast to ally with existing target", func(t *testing.T) {
		ally.BotTarget = 10 // Already has a target

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, BroadcastTargetMinValue+1000)

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 0 {
			t.Errorf("got %d suggestions for busy ally, want 0", len(suggestions))
		}
		ally.BotTarget = -1
	})

	t.Run("Does not broadcast to distant ally", func(t *testing.T) {
		ally.X, ally.Y = 70000, 50000 // 20000 units away (beyond BroadcastTargetRange)
		ally.BotTarget = -1

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, BroadcastTargetMinValue+1000)

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 0 {
			t.Errorf("got %d suggestions for distant ally, want 0", len(suggestions))
		}
	})

	t.Run("Does not broadcast to enemy players", func(t *testing.T) {
		ally.X, ally.Y = 55000, 50000
		ally.Team = game.TeamRom // Wrong team

		server.pendingSuggestions = nil
		server.BroadcastTargetToAllies(bot, target, BroadcastTargetMinValue+1000)

		suggestions := server.GetPendingSuggestions()
		if len(suggestions) != 0 {
			t.Errorf("got %d suggestions for enemy, want 0", len(suggestions))
		}
		ally.Team = game.TeamFed
	})
}

func TestApplyPendingTargetSuggestions(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	ally := gs.Players[1]
	ally.Status = game.StatusAlive
	ally.Team = game.TeamFed
	ally.IsBot = true
	ally.BotTarget = -1

	t.Run("Applies suggestion to untargeted ally", func(t *testing.T) {
		ally.BotTarget = -1
		server.pendingSuggestions = []targetSuggestion{
			{allyID: ally.ID, targetID: 5, lockTime: 20, value: 5000},
		}

		server.ApplyPendingTargetSuggestions()

		if ally.BotTarget != 5 {
			t.Errorf("ally BotTarget = %d, want 5", ally.BotTarget)
		}
		if ally.BotTargetLockTime != 20 {
			t.Errorf("ally BotTargetLockTime = %d, want 20", ally.BotTargetLockTime)
		}
		if len(server.pendingSuggestions) != 0 {
			t.Errorf("pendingSuggestions not cleared, got %d", len(server.pendingSuggestions))
		}
	})

	t.Run("Does not overwrite ally that acquired target during tick", func(t *testing.T) {
		ally.BotTarget = 7 // Acquired a target during this tick
		server.pendingSuggestions = []targetSuggestion{
			{allyID: ally.ID, targetID: 5, lockTime: 20, value: 5000},
		}

		server.ApplyPendingTargetSuggestions()

		if ally.BotTarget != 7 {
			t.Errorf("ally BotTarget = %d, want 7 (should not be overwritten)", ally.BotTarget)
		}
	})
}

func TestIsPlayerIsolated(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	player := gs.Players[0]
	player.Status = game.StatusAlive
	player.Team = game.TeamFed
	player.X, player.Y = 50000, 50000

	t.Run("Player with no allies is isolated", func(t *testing.T) {
		// All other players dead
		for i := 1; i < len(gs.Players); i++ {
			gs.Players[i].Status = game.StatusDead
		}
		server.cachedIsolationFrame = 0
		gs.Frame = 1

		if !server.IsPlayerIsolated(player.ID) {
			t.Error("player with no living allies should be isolated")
		}
	})

	t.Run("Player with nearby ally is not isolated", func(t *testing.T) {
		ally := gs.Players[1]
		ally.Status = game.StatusAlive
		ally.Team = game.TeamFed
		ally.X, ally.Y = 53000, 50000 // 3000 units away (within IsolationRange)

		server.cachedIsolationFrame = 0
		gs.Frame = 2

		if server.IsPlayerIsolated(player.ID) {
			t.Error("player with nearby ally should not be isolated")
		}
		ally.Status = game.StatusDead
	})

	t.Run("Player with distant ally is isolated", func(t *testing.T) {
		ally := gs.Players[1]
		ally.Status = game.StatusAlive
		ally.Team = game.TeamFed
		ally.X, ally.Y = 60000, 50000 // 10000 units away (beyond IsolationRange)

		server.cachedIsolationFrame = 0
		gs.Frame = 3

		if !server.IsPlayerIsolated(player.ID) {
			t.Error("player with only distant ally should be isolated")
		}
		ally.Status = game.StatusDead
	})

	t.Run("Enemy nearby does not prevent isolation", func(t *testing.T) {
		enemy := gs.Players[1]
		enemy.Status = game.StatusAlive
		enemy.Team = game.TeamRom
		enemy.X, enemy.Y = 51000, 50000 // Very close but enemy

		server.cachedIsolationFrame = 0
		gs.Frame = 4

		if !server.IsPlayerIsolated(player.ID) {
			t.Error("nearby enemy should not prevent isolation")
		}
		enemy.Status = game.StatusDead
	})

	t.Run("Cache is reused within same frame", func(t *testing.T) {
		// Set up isolation state
		for i := 1; i < len(gs.Players); i++ {
			gs.Players[i].Status = game.StatusDead
		}
		server.cachedIsolationFrame = 0
		gs.Frame = 5

		result1 := server.IsPlayerIsolated(player.ID)

		// Now add a nearby ally but don't clear cache (same frame)
		ally := gs.Players[1]
		ally.Status = game.StatusAlive
		ally.Team = game.TeamFed
		ally.X, ally.Y = 51000, 50000

		result2 := server.IsPlayerIsolated(player.ID)

		if result1 != result2 {
			t.Error("cache should return same result within same frame")
		}
		ally.Status = game.StatusDead
	})
}
