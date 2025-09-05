package torp_aim

import (
	"fmt"
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
	"github.com/lab1702/netrek-web/server"
)

func TestDebugMovingTarget(t *testing.T) {
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

	// Set up moving target
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipCruiser
	target.X = 55000         // 5000 units east
	target.Y = 50000         // Same Y
	target.Speed = 8         // Moving at speed 8
	target.Dir = math.Pi / 2 // Moving north

	fmt.Printf("=== MOVING TARGET DEBUG ===\n")
	fmt.Printf("Shooter at (%.1f, %.1f)\n", shooter.X, shooter.Y)
	fmt.Printf("Target at (%.1f, %.1f)\n", target.X, target.Y)
	fmt.Printf("Target speed: %.1f, direction: %.3f rad (%.1f degrees)\n", target.Speed, target.Dir, target.Dir*180/math.Pi)

	distance := game.Distance(shooter.X, shooter.Y, target.X, target.Y)
	fmt.Printf("Distance: %.1f\n", distance)

	shipStats := game.ShipData[shooter.Ship]
	fmt.Printf("Ship torpedo speed: %d * 20 = %.1f units/tick\n", shipStats.TorpSpeed, float64(shipStats.TorpSpeed*20))

	// Show what the bot code thinks target velocity is
	targetVelX := target.Speed * math.Cos(target.Dir) * 20
	targetVelY := target.Speed * math.Sin(target.Dir) * 20
	fmt.Printf("Target velocity as calculated by bot: (%.1f, %.1f) units/tick\n", targetVelX, targetVelY)

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

	// For a target moving north, we should lead it - torpedo should fire northeast
	// Expected rough direction would be arctan(target_velocity_y / (torpedo_speed - target_velocity_x))
	// Since target is moving north (no x component) and torpedo is faster, we expect some northward angle
	expectedLeadAngle := math.Atan2(targetVelY, torpedo.Speed)
	fmt.Printf("Expected rough lead angle: %.3f rad (%.1f degrees)\n", expectedLeadAngle, expectedLeadAngle*180/math.Pi)

	// Simulate trajectory - check if they meet
	fmt.Printf("\n=== TRAJECTORY SIMULATION ===\n")
	minDist := math.Inf(1)
	minTick := -1.0

	for tick := 0.0; tick <= 30.0; tick += 0.5 {
		torpX := torpedo.X + torpedo.Speed*math.Cos(torpedo.Dir)*tick
		torpY := torpedo.Y + torpedo.Speed*math.Sin(torpedo.Dir)*tick

		// Target moves at game speed (not *20)
		targX := target.X + target.Speed*math.Cos(target.Dir)*tick*20
		targY := target.Y + target.Speed*math.Sin(target.Dir)*tick*20

		dist := math.Sqrt((torpX-targX)*(torpX-targX) + (torpY-targY)*(torpY-targY))

		if dist < minDist {
			minDist = dist
			minTick = tick
		}

		if int(tick*2)%4 == 0 { // Print every 2 ticks
			fmt.Printf("Tick %4.1f: Torp(%.1f, %.1f) Target(%.1f, %.1f) Dist: %.1f\n",
				tick, torpX, torpY, targX, targY, dist)
		}
	}

	fmt.Printf("\nClosest approach: %.1f units at tick %.1f\n", minDist, minTick)
	if minDist <= 600 {
		fmt.Printf("HIT! (within 600 unit radius)\n")
	} else {
		fmt.Printf("MISS by %.1f units\n", minDist)
	}
}
