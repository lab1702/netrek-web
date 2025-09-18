package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestOrbitalVelocity(t *testing.T) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	// Create a planet at origin
	planet := &game.Planet{
		ID:    0,
		Name:  "TestPlanet",
		X:     0,
		Y:     0,
		Owner: game.TeamFed,
	}
	gs.Planets[0] = planet

	// Create a player orbiting the planet
	player := gs.Players[0]
	player.Status = game.StatusAlive
	player.Orbiting = 0
	player.X = 800 // OrbitDist from game/types.go
	player.Y = 0
	player.Dir = math.Pi / 2 // Pointing North (tangent to circle)

	// Test orbital velocity calculation
	vx, vy, ok := server.OrbitalVelocity(player)
	if !ok {
		t.Fatal("Expected orbital velocity calculation to succeed")
	}

	// Expected angular velocity is π/64 rad/tick (from physics.go)
	// Expected tangential speed = (π/64) * 800 ≈ 39.27
	expectedSpeed := (math.Pi / 64.0) * 800
	actualSpeed := math.Sqrt(vx*vx + vy*vy)

	tolerance := 0.1
	if math.Abs(actualSpeed-expectedSpeed) > tolerance {
		t.Errorf("Expected orbital speed ~%.2f, got %.2f", expectedSpeed, actualSpeed)
	}

	// Check velocity direction - should be perpendicular to radius vector
	// Radius vector is (800, 0), so velocity should be (0, positive)
	if math.Abs(vx) > tolerance {
		t.Errorf("Expected vx ~0, got %.2f", vx)
	}
	if vy < 0 {
		t.Errorf("Expected positive vy for counter-clockwise orbit, got %.2f", vy)
	}

	t.Logf("Orbital velocity: vx=%.2f, vy=%.2f, speed=%.2f", vx, vy, actualSpeed)
}

func TestOrbitalVelocityNotOrbiting(t *testing.T) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	// Create a player not orbiting
	player := gs.Players[0]
	player.Status = game.StatusAlive
	player.Orbiting = -1
	player.X = 1000
	player.Y = 1000

	// Test that non-orbiting players return false
	_, _, ok := server.OrbitalVelocity(player)
	if ok {
		t.Error("Expected orbital velocity calculation to fail for non-orbiting player")
	}
}

func TestTargetVelocity(t *testing.T) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	// Create a planet
	planet := &game.Planet{ID: 0, X: 0, Y: 0}
	gs.Planets[0] = planet

	t.Run("NonOrbiting", func(t *testing.T) {
		// Test non-orbiting player
		player := gs.Players[0]
		player.Status = game.StatusAlive
		player.Orbiting = -1
		player.Speed = 5.0
		player.Dir = math.Pi / 4 // 45 degrees

		vel := server.targetVelocity(player)
		expectedVx := player.Speed * math.Cos(player.Dir) * 20
		expectedVy := player.Speed * math.Sin(player.Dir) * 20

		tolerance := 0.01
		if math.Abs(vel.X-expectedVx) > tolerance || math.Abs(vel.Y-expectedVy) > tolerance {
			t.Errorf("Expected velocity (%.2f, %.2f), got (%.2f, %.2f)",
				expectedVx, expectedVy, vel.X, vel.Y)
		}
	})

	t.Run("Orbiting", func(t *testing.T) {
		// Test orbiting player
		player := gs.Players[1]
		player.Status = game.StatusAlive
		player.Orbiting = 0
		player.X = 800
		player.Y = 0
		player.Speed = 0 // Should be 0 when orbiting
		player.Dir = math.Pi / 2

		vel := server.targetVelocity(player)

		// Should use orbital velocity, not straight-line velocity
		expectedSpeed := (math.Pi / 64.0) * 800
		actualSpeed := math.Sqrt(vel.X*vel.X + vel.Y*vel.Y)

		tolerance := 0.1
		if math.Abs(actualSpeed-expectedSpeed) > tolerance {
			t.Errorf("Expected orbital velocity speed ~%.2f, got %.2f", expectedSpeed, actualSpeed)
		}
	})
}

func TestInterceptAccuracyWithOrbitalVelocity(t *testing.T) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	// Create a planet at origin
	planet := &game.Planet{ID: 0, X: 0, Y: 0}
	gs.Planets[0] = planet

	// Create shooter
	shooter := gs.Players[0]
	shooter.X = 3000
	shooter.Y = 0
	shooter.Ship = game.ShipDestroyer

	// Create orbiting target
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Orbiting = 0
	target.X = 800 // At orbit distance
	target.Y = 0
	target.Speed = 0
	target.Dir = math.Pi / 2

	// Calculate intercept using old method (straight-line)
	shooterPos := Point2D{X: shooter.X, Y: shooter.Y}
	targetPos := Point2D{X: target.X, Y: target.Y}

	oldTargetVel := Vector2D{
		X: target.Speed * math.Cos(target.Dir) * 20,
		Y: target.Speed * math.Sin(target.Dir) * 20,
	}

	shipStats := game.ShipData[shooter.Ship]
	projSpeed := float64(shipStats.TorpSpeed * 20)

_, oldOk := InterceptDirectionSimple(shooterPos, targetPos, oldTargetVel, projSpeed)
if !oldOk {
	t.Fatal("Old intercept calculation should succeed")
}

	// Calculate intercept using new method (orbital velocity)
	newTargetVel := server.targetVelocity(target)
_, newOk := InterceptDirectionSimple(shooterPos, targetPos, newTargetVel, projSpeed)
if !newOk {
	t.Fatal("New intercept calculation should succeed")
}

// Simulate target movement for a few ticks to see which prediction is better
ticks := 10.0

	// Future target position using orbital mechanics
	angularVel := math.Pi / 64.0
	futureAngle := math.Atan2(target.Y, target.X) + angularVel*ticks
	radius := math.Sqrt(target.X*target.X + target.Y*target.Y)
	actualFutureX := radius * math.Cos(futureAngle)
	actualFutureY := radius * math.Sin(futureAngle)

	// Predicted positions
	oldPredictedX := target.X + oldTargetVel.X*ticks
	oldPredictedY := target.Y + oldTargetVel.Y*ticks

	newPredictedX := target.X + newTargetVel.X*ticks
	newPredictedY := target.Y + newTargetVel.Y*ticks

	// Calculate prediction errors
	oldError := math.Sqrt((oldPredictedX-actualFutureX)*(oldPredictedX-actualFutureX) +
		(oldPredictedY-actualFutureY)*(oldPredictedY-actualFutureY))

	newError := math.Sqrt((newPredictedX-actualFutureX)*(newPredictedX-actualFutureX) +
		(newPredictedY-actualFutureY)*(newPredictedY-actualFutureY))

	t.Logf("Old prediction error: %.2f", oldError)
	t.Logf("New prediction error: %.2f", newError)
	t.Logf("Improvement: %.1fx better", oldError/newError)

	// New method should be significantly more accurate
	if newError >= oldError*0.5 {
		t.Errorf("Expected new method to be at least 2x better, but old_error=%.2f, new_error=%.2f",
			oldError, newError)
	}
}

func BenchmarkTargetVelocityNonOrbiting(b *testing.B) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	player := gs.Players[0]
	player.Status = game.StatusAlive
	player.Orbiting = -1
	player.Speed = 5.0
	player.Dir = math.Pi / 4

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.targetVelocity(player)
	}
}

func BenchmarkTargetVelocityOrbiting(b *testing.B) {
	server := &Server{}
	gs := game.NewGameState()
	server.gameState = gs

	planet := &game.Planet{ID: 0, X: 0, Y: 0}
	gs.Planets[0] = planet

	player := gs.Players[0]
	player.Status = game.StatusAlive
	player.Orbiting = 0
	player.X = 800
	player.Y = 0

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = server.targetVelocity(player)
	}
}

func BenchmarkOldTargetVelocityCalculation(b *testing.B) {
	player := &game.Player{}
	player.Speed = 5.0
	player.Dir = math.Pi / 4

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Vector2D{
			X: player.Speed * math.Cos(player.Dir) * 20,
			Y: player.Speed * math.Sin(player.Dir) * 20,
		}
	}
}
