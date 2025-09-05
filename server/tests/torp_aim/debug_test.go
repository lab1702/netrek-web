package torp_aim

import (
	"fmt"
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
	"github.com/lab1702/netrek-web/server"
)

func TestDebugTorpedoParameters(t *testing.T) {
	// Create fresh game state
	gs := game.NewGameState()
	testServer := &server.Server{}
	testServer.SetGameState(gs)

	// Set up bot shooter
	shooter := gs.Players[0]
	shooter.Status = game.StatusAlive
	shooter.Team = game.TeamFed
	shooter.Ship = game.ShipDestroyer
	shooter.X = 50000
	shooter.Y = 50000
	shooter.Dir = 0
	shooter.Speed = 0
	shooter.Fuel = 10000
	shooter.WTemp = 0
	shooter.NumTorps = 0
	shooter.IsBot = true

	// Set up stationary target directly to the east
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipCruiser
	target.X = 55000 // 5000 units east
	target.Y = 50000 // Same Y
	target.Speed = 0 // Stationary
	target.Dir = 0

	fmt.Printf("=== TORPEDO LAUNCH DEBUG ===\n")
	fmt.Printf("Shooter at (%.1f, %.1f)\n", shooter.X, shooter.Y)
	fmt.Printf("Target at (%.1f, %.1f)\n", target.X, target.Y)
	fmt.Printf("Target speed: %.1f, direction: %.3f\n", target.Speed, target.Dir)

	distance := game.Distance(shooter.X, shooter.Y, target.X, target.Y)
	fmt.Printf("Distance: %.1f\n", distance)

	shipStats := game.ShipData[shooter.Ship]
	fmt.Printf("Ship torpedo speed: %d\n", shipStats.TorpSpeed)
	fmt.Printf("Ship torpedo speed * 20: %.1f units/tick\n", float64(shipStats.TorpSpeed*20))

	// Fire torpedo
	initialTorpCount := len(gs.Torps)
	testServer.FireBotTorpedoWithLead(shooter, target)

	if len(gs.Torps) <= initialTorpCount {
		t.Fatal("No torpedo was fired")
	}

	torpedo := gs.Torps[len(gs.Torps)-1]
	fmt.Printf("Torpedo fired:\n")
	fmt.Printf("  Direction: %.3f rad (%.1f degrees)\n", torpedo.Dir, torpedo.Dir*180/math.Pi)
	fmt.Printf("  Speed: %.1f units/tick\n", torpedo.Speed)
	fmt.Printf("  Fuse: %d ticks\n", torpedo.Fuse)

	// Expected direction for stationary target should be 0 (due east)
	expectedDir := 0.0
	actualDir := torpedo.Dir
	angleDiff := math.Abs(actualDir - expectedDir)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	fmt.Printf("Expected direction: %.3f, Actual: %.3f, Difference: %.3f rad (%.1f degrees)\n",
		expectedDir, actualDir, angleDiff, angleDiff*180/math.Pi)

	// Simulate where torpedo will be after various times
	fmt.Printf("\n=== TRAJECTORY SIMULATION ===\n")
	for tick := 0; tick <= 10; tick++ {
		torpX := torpedo.X + torpedo.Speed*math.Cos(torpedo.Dir)*float64(tick)
		torpY := torpedo.Y + torpedo.Speed*math.Sin(torpedo.Dir)*float64(tick)
		targX := target.X + target.Speed*math.Cos(target.Dir)*float64(tick)*20
		targY := target.Y + target.Speed*math.Sin(target.Dir)*float64(tick)*20

		dist := math.Sqrt((torpX-targX)*(torpX-targX) + (torpY-targY)*(torpY-targY))
		fmt.Printf("Tick %2d: Torp(%.1f, %.1f) Target(%.1f, %.1f) Dist: %.1f\n",
			tick, torpX, torpY, targX, targY, dist)
	}

	// Expected hit time
	expectedHitTime := distance / torpedo.Speed
	fmt.Printf("\nExpected hit time: %.2f ticks\n", expectedHitTime)
}
