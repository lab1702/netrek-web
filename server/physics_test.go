package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

// TestPlayerTurning tests the turning mechanics with fractional accumulator
func TestPlayerTurning(t *testing.T) {
	tests := []struct {
		name          string
		speed         float64
		ship          game.ShipType
		initialDir    float64
		desiredDir    float64
		ticksToRun    int
		expectedClose bool // Whether we expect to reach desired direction
		description   string
	}{
		{
			name:          "Fast turn at speed 0",
			speed:         0,
			ship:          game.ShipDestroyer,
			initialDir:    0,
			desiredDir:    math.Pi / 2,
			ticksToRun:    5,
			expectedClose: true,
			description:   "At speed 0, ships should turn very fast",
		},
		{
			name:          "Slow turn at high speed",
			speed:         10,
			ship:          game.ShipDestroyer,
			initialDir:    0,
			desiredDir:    math.Pi / 2,
			ticksToRun:    50,
			expectedClose: false,
			description:   "At high speed, ships should turn slowly",
		},
		{
			name:          "No turn when already at desired direction",
			speed:         5,
			ship:          game.ShipCruiser,
			initialDir:    math.Pi,
			desiredDir:    math.Pi,
			ticksToRun:    10,
			expectedClose: true,
			description:   "Should remain at same direction if already there",
		},
		{
			name:          "Turn shortest path (clockwise)",
			speed:         2,
			ship:          game.ShipDestroyer,
			initialDir:    0.1,
			desiredDir:    2 * math.Pi,
			ticksToRun:    20,
			expectedClose: true,
			description:   "Should turn shortest path clockwise",
		},
		{
			name:          "Turn shortest path (counter-clockwise)",
			speed:         2,
			ship:          game.ShipDestroyer,
			initialDir:    2*math.Pi - 0.1,
			desiredDir:    0,
			ticksToRun:    20,
			expectedClose: true,
			description:   "Should turn shortest path counter-clockwise",
		},
		{
			name:          "Medium speed turning",
			speed:         5,
			ship:          game.ShipDestroyer,
			initialDir:    0,
			desiredDir:    math.Pi / 4,
			ticksToRun:    30,
			expectedClose: true,
			description:   "Medium speed should allow turning within reasonable time",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := game.NewGameState()
			server := &Server{gameState: gs}

			// Setup player
			p := gs.Players[0]
			p.Status = game.StatusAlive
			p.Ship = tt.ship
			p.Dir = tt.initialDir
			p.DesDir = tt.desiredDir
			p.Speed = tt.speed
			p.DesSpeed = tt.speed
			p.SubDir = 0
			p.X = 50000
			p.Y = 50000

			// Run physics updates
			for tick := 0; tick < tt.ticksToRun; tick++ {
				server.updatePlayerPhysics(p, 0)
			}

			// Check if we're close to desired direction (within 0.1 radians)
			angleDiff := math.Abs(p.Dir - tt.desiredDir)
			// Normalize angle difference
			if angleDiff > math.Pi {
				angleDiff = 2*math.Pi - angleDiff
			}

			isClose := angleDiff < 0.1

			if tt.expectedClose && !isClose {
				t.Errorf("Expected to reach desired direction, but diff is %.4f radians (%.1f degrees)",
					angleDiff, angleDiff*180/math.Pi)
			} else if !tt.expectedClose && isClose {
				t.Errorf("Expected NOT to reach desired direction in %d ticks, but did", tt.ticksToRun)
			}
		})
	}
}

// TestSpeedAcceleration tests the acceleration mechanics with fractional accumulator
func TestSpeedAcceleration(t *testing.T) {
	tests := []struct {
		name            string
		ship            game.ShipType
		initialSpeed    float64
		desiredSpeed    float64
		ticksToRun      int
		expectedMinimum float64 // Minimum speed we expect to reach
		description     string
	}{
		{
			name:            "Accelerate from 0 to max",
			ship:            game.ShipDestroyer,
			initialSpeed:    0,
			desiredSpeed:    10,
			ticksToRun:      100,
			expectedMinimum: 9.0,
			description:     "Ship should accelerate from 0 to near max speed",
		},
		{
			name:            "Decelerate from max to 0",
			ship:            game.ShipDestroyer,
			initialSpeed:    10,
			desiredSpeed:    0,
			ticksToRun:      100,
			expectedMinimum: -1, // Expect to reach 0 (less than minimum means we check exact value)
			description:     "Ship should decelerate from max to 0",
		},
		{
			name:            "Slow acceleration for heavy ship",
			ship:            game.ShipStarbase,
			initialSpeed:    0,
			desiredSpeed:    4,
			ticksToRun:      50,
			expectedMinimum: 2.0,
			description:     "Heavy ships should accelerate more slowly",
		},
		{
			name:            "Fast acceleration for scout",
			ship:            game.ShipScout,
			initialSpeed:    0,
			desiredSpeed:    12,
			ticksToRun:      80,
			expectedMinimum: 10.0,
			description:     "Scout should accelerate quickly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := game.NewGameState()
			server := &Server{gameState: gs}

			// Setup player
			p := gs.Players[0]
			p.Status = game.StatusAlive
			p.Ship = tt.ship
			p.Speed = tt.initialSpeed
			p.DesSpeed = tt.desiredSpeed
			p.AccFrac = 0
			p.X = 50000
			p.Y = 50000
			p.Dir = 0
			p.DesDir = 0

			// Run physics updates
			for tick := 0; tick < tt.ticksToRun; tick++ {
				server.updatePlayerPhysics(p, 0)
			}

			if tt.expectedMinimum >= 0 {
				// Check we reached at least minimum speed
				if p.Speed < tt.expectedMinimum {
					t.Errorf("Expected speed >= %.1f, got %.1f", tt.expectedMinimum, p.Speed)
				}
			} else {
				// Check we reached exact target (for deceleration to 0)
				if p.Speed != tt.desiredSpeed {
					t.Errorf("Expected speed %.1f, got %.1f", tt.desiredSpeed, p.Speed)
				}
			}
		})
	}
}

// TestSpeedWithDamage tests that damaged ships have reduced max speed
func TestSpeedWithDamage(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.Speed = 0     // Start from 0 to trigger acceleration
	p.DesSpeed = 10 // Try to reach max speed
	p.Damage = 0
	p.X = 50000
	p.Y = 50000
	p.Dir = 0
	p.DesDir = 0

	shipStats := game.ShipData[p.Ship]

	// Apply 50% damage
	p.Damage = shipStats.MaxDamage / 2

	// Try to accelerate to max speed - should be capped at reduced max
	for tick := 0; tick < 150; tick++ {
		server.updatePlayerPhysics(p, 0)
	}

	// With 50% damage, max speed should be reduced
	// Formula: maxspeed = (max + 2) - (max + 1) * (damage / maxdamage)
	// = (10 + 2) - (10 + 1) * 0.5 = 12 - 5.5 = 6.5
	expectedMaxSpeed := 6.5

	if p.Speed > expectedMaxSpeed+0.1 {
		t.Errorf("Damaged ship speed %.1f exceeds expected max %.1f", p.Speed, expectedMaxSpeed)
	}

	// Also verify that we reached close to the expected max
	if p.Speed < expectedMaxSpeed-0.5 {
		t.Errorf("Damaged ship should reach close to max speed, got %.1f, expected ~%.1f", p.Speed, expectedMaxSpeed)
	}
}

// TestEngineOverheat tests that engine overheat limits speed to 1
func TestEngineOverheat(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.Speed = 10
	p.DesSpeed = 5 // Set to different value to trigger speed update logic
	p.X = 50000
	p.Y = 50000
	p.Dir = 0
	p.DesDir = 0

	// Trigger engine overheat while moving
	p.EngineOverheat = true

	// Run physics updates - ship should decelerate to speed 1
	for tick := 0; tick < 200; tick++ {
		server.updatePlayerPhysics(p, 0)
	}

	if p.Speed > 1 {
		t.Errorf("Engine overheat should limit speed to 1, got %.1f", p.Speed)
	}

	if p.DesSpeed > 1 {
		t.Errorf("Engine overheat should clamp DesSpeed to 1, got %.1f", p.DesSpeed)
	}
}

// TestPositionUpdate tests that ships move correctly based on speed and direction
func TestPositionUpdate(t *testing.T) {
	tests := []struct {
		name      string
		speed     float64
		direction float64
		ticks     int
		expectX   func(initialX, finalX float64) bool
		expectY   func(initialY, finalY float64) bool
	}{
		{
			name:      "Move east",
			speed:     5,
			direction: 0,
			ticks:     10,
			expectX:   func(initialX, finalX float64) bool { return finalX > initialX+900 },
			expectY:   func(initialY, finalY float64) bool { return math.Abs(finalY-initialY) < 10 },
		},
		{
			name:      "Move south",
			speed:     5,
			direction: math.Pi / 2,
			ticks:     10,
			expectX:   func(initialX, finalX float64) bool { return math.Abs(finalX-initialX) < 10 },
			expectY:   func(initialY, finalY float64) bool { return finalY > initialY+900 },
		},
		{
			name:      "Move west",
			speed:     5,
			direction: math.Pi,
			ticks:     10,
			expectX:   func(initialX, finalX float64) bool { return finalX < initialX-900 },
			expectY:   func(initialY, finalY float64) bool { return math.Abs(finalY-initialY) < 10 },
		},
		{
			name:      "Move north",
			speed:     5,
			direction: 3 * math.Pi / 2,
			ticks:     10,
			expectX:   func(initialX, finalX float64) bool { return math.Abs(finalX-initialX) < 10 },
			expectY:   func(initialY, finalY float64) bool { return finalY < initialY-900 },
		},
		{
			name:      "Speed 0 - no movement",
			speed:     0,
			direction: 0,
			ticks:     10,
			expectX:   func(initialX, finalX float64) bool { return math.Abs(finalX-initialX) < 0.1 },
			expectY:   func(initialY, finalY float64) bool { return math.Abs(finalY-initialY) < 0.1 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := game.NewGameState()
			server := &Server{gameState: gs}

			p := gs.Players[0]
			p.Status = game.StatusAlive
			p.Ship = game.ShipDestroyer
			p.Speed = tt.speed
			p.DesSpeed = tt.speed
			p.Dir = tt.direction
			p.DesDir = tt.direction
			p.X = 50000
			p.Y = 50000

			initialX := p.X
			initialY := p.Y

			// Run physics updates
			for tick := 0; tick < tt.ticks; tick++ {
				server.updatePlayerPhysics(p, 0)
			}

			if !tt.expectX(initialX, p.X) {
				t.Errorf("X position expectation failed: initial=%.1f, final=%.1f", initialX, p.X)
			}
			if !tt.expectY(initialY, p.Y) {
				t.Errorf("Y position expectation failed: initial=%.1f, final=%.1f", initialY, p.Y)
			}
		})
	}
}

// TestGalaxyEdgeBounce tests that ships bounce off galaxy edges
func TestGalaxyEdgeBounce(t *testing.T) {
	tests := []struct {
		name        string
		initialX    float64
		initialY    float64
		direction   float64
		speed       float64
		expectX     func(x float64) bool
		expectY     func(y float64) bool
		expectDir   func(dir float64) bool
		description string
	}{
		{
			name:        "Bounce off left edge",
			initialX:    100,
			initialY:    50000,
			direction:   math.Pi, // Moving west
			speed:       10,
			expectX:     func(x float64) bool { return x >= 0 },
			expectY:     func(y float64) bool { return true },
			expectDir:   func(dir float64) bool { return dir != math.Pi }, // Direction should change
			description: "Ship should bounce off left edge and reverse X direction",
		},
		{
			name:        "Bounce off right edge",
			initialX:    game.GalaxyWidth - 100,
			initialY:    50000,
			direction:   0, // Moving east
			speed:       10,
			expectX:     func(x float64) bool { return x <= game.GalaxyWidth },
			expectY:     func(y float64) bool { return true },
			expectDir:   func(dir float64) bool { return dir != 0 }, // Direction should change
			description: "Ship should bounce off right edge and reverse X direction",
		},
		{
			name:        "Bounce off top edge",
			initialX:    50000,
			initialY:    100,
			direction:   3 * math.Pi / 2, // Moving north
			speed:       10,
			expectX:     func(x float64) bool { return true },
			expectY:     func(y float64) bool { return y >= 0 },
			expectDir:   func(dir float64) bool { return dir != 3*math.Pi/2 }, // Direction should change
			description: "Ship should bounce off top edge and reverse Y direction",
		},
		{
			name:        "Bounce off bottom edge",
			initialX:    50000,
			initialY:    game.GalaxyHeight - 100,
			direction:   math.Pi / 2, // Moving south
			speed:       10,
			expectX:     func(x float64) bool { return true },
			expectY:     func(y float64) bool { return y <= game.GalaxyHeight },
			expectDir:   func(dir float64) bool { return dir != math.Pi/2 }, // Direction should change
			description: "Ship should bounce off bottom edge and reverse Y direction",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := game.NewGameState()
			server := &Server{gameState: gs}

			p := gs.Players[0]
			p.Status = game.StatusAlive
			p.Ship = game.ShipDestroyer
			p.Speed = tt.speed
			p.DesSpeed = tt.speed
			p.Dir = tt.direction
			p.DesDir = tt.direction
			p.X = tt.initialX
			p.Y = tt.initialY

			// Run physics updates until we hit edge
			for tick := 0; tick < 20; tick++ {
				server.updatePlayerPhysics(p, 0)
			}

			if !tt.expectX(p.X) {
				t.Errorf("X position failed: got %.1f", p.X)
			}
			if !tt.expectY(p.Y) {
				t.Errorf("Y position failed: got %.1f", p.Y)
			}
			if !tt.expectDir(p.Dir) {
				t.Errorf("Direction should have changed from %.4f, got %.4f", tt.direction, p.Dir)
			}
		})
	}
}

// TestPlayerOrbit tests orbital mechanics
func TestPlayerOrbit(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	// Setup planet
	planet := gs.Planets[0]
	planet.X = 50000
	planet.Y = 50000

	// Setup player orbiting planet
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.Orbiting = 0
	p.Dir = 0
	p.DesDir = 0
	p.X = planet.X + float64(game.OrbitDist)
	p.Y = planet.Y
	p.Speed = 0

	initialDir := p.Dir

	// Run orbit updates
	for tick := 0; tick < 100; tick++ {
		server.updatePlayerOrbit(p)
	}

	// Check that direction has incremented
	if p.Dir <= initialDir {
		t.Errorf("Orbit direction should increase, initial=%.4f, final=%.4f", initialDir, p.Dir)
	}

	// Check that player maintains orbit distance
	dist := game.Distance(p.X, p.Y, planet.X, planet.Y)
	expectedDist := float64(game.OrbitDist)
	if math.Abs(dist-expectedDist) > 1 {
		t.Errorf("Orbit distance should be %.1f, got %.1f", expectedDist, dist)
	}

	// Check that speed is 0 during orbit
	if p.Speed != 0 {
		t.Errorf("Speed should be 0 during orbit, got %.1f", p.Speed)
	}

	// Check that DesDir matches Dir (should be synchronized)
	if p.DesDir != p.Dir {
		t.Errorf("DesDir should match Dir during orbit")
	}
}

// TestOrbitNotUpdatedWhenNotOrbiting tests that orbit update is skipped when not orbiting
func TestOrbitNotUpdatedWhenNotOrbiting(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.Orbiting = -1 // Not orbiting
	p.Dir = 0
	p.X = 50000
	p.Y = 50000

	initialDir := p.Dir
	initialX := p.X
	initialY := p.Y

	// Try to update orbit (should do nothing)
	server.updatePlayerOrbit(p)

	if p.Dir != initialDir || p.X != initialX || p.Y != initialY {
		t.Errorf("Orbit update should do nothing when not orbiting")
	}
}

// TestPlayerLockOnPlanet tests planet lock-on navigation
func TestPlayerLockOnPlanet(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	// Setup planet
	planet := gs.Planets[0]
	planet.X = 60000
	planet.Y = 60000
	planet.Owner = game.TeamFed

	// Setup player far from planet
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipDestroyer
	p.LockType = "planet"
	p.LockTarget = 0
	p.X = 50000
	p.Y = 50000
	p.Dir = 0
	p.DesDir = 0
	p.Speed = 5
	p.DesSpeed = 5

	// Run lock-on updates
	for tick := 0; tick < 10; tick++ {
		server.updatePlayerLockOn(p)
	}

	// Check that DesDir points toward planet
	expectedDir := math.Atan2(planet.Y-p.Y, planet.X-p.X)
	angleDiff := math.Abs(p.DesDir - expectedDir)
	if angleDiff > math.Pi {
		angleDiff = 2*math.Pi - angleDiff
	}

	if angleDiff > 0.1 {
		t.Errorf("DesDir should point toward planet, expected %.4f, got %.4f", expectedDir, p.DesDir)
	}

	// Check that DesSpeed is set to max (far from planet)
	maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)
	if p.DesSpeed != maxSpeed {
		t.Errorf("DesSpeed should be max when far from planet, got %.1f", p.DesSpeed)
	}
}

// TestAutoOrbitOnLockOn tests that ships auto-orbit when close and slow
func TestAutoOrbitOnLockOn(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs, broadcast: make(chan ServerMessage, 256)}

	// Setup planet
	planet := gs.Planets[0]
	planet.X = 50000
	planet.Y = 50000
	planet.Owner = game.TeamFed
	planet.Info = 0

	// Setup player close to planet and slow
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipDestroyer
	p.LockType = "planet"
	p.LockTarget = 0
	p.X = planet.X + float64(game.EntOrbitDist) - 100 // Close enough (< EntOrbitDist)
	p.Y = planet.Y
	p.Dir = 0
	p.DesDir = 0
	p.Speed = float64(game.ORBSPEED) // At orbit speed
	p.DesSpeed = float64(game.ORBSPEED)
	p.Orbiting = -1

	// This should trigger auto-orbit
	server.updatePlayerLockOn(p)

	if p.Orbiting != 0 {
		t.Errorf("Should auto-orbit planet 0 when close and slow, got Orbiting=%d", p.Orbiting)
	}

	if p.Speed != 0 {
		t.Errorf("Speed should be set to 0 on orbit entry, got %.1f", p.Speed)
	}

	if p.LockType != "none" {
		t.Errorf("Lock should be cleared on orbit entry, got %s", p.LockType)
	}

	// Check that planet Info was updated
	if planet.Info&p.Team == 0 {
		t.Errorf("Planet Info should be updated with team scout info")
	}
}

// TestLockOnSpeedAdjustment tests that ships slow down when approaching planet
func TestLockOnSpeedAdjustment(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	// Setup planet
	planet := gs.Planets[0]
	planet.X = 50000
	planet.Y = 50000

	// Setup player at medium distance
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.LockType = "planet"
	p.LockTarget = 0
	p.X = planet.X + 4000 // Between 3000 and 5000
	p.Y = planet.Y
	p.Dir = 0
	p.DesDir = 0
	p.Speed = 10
	p.DesSpeed = 10

	maxSpeed := float64(game.ShipData[p.Ship].MaxSpeed)

	server.updatePlayerLockOn(p)

	// DesSpeed should be adjusted to slow down
	if p.DesSpeed >= maxSpeed {
		t.Errorf("DesSpeed should be reduced when approaching planet, got %.1f", p.DesSpeed)
	}

	if p.DesSpeed < 3 {
		t.Errorf("DesSpeed should not be less than 3, got %.1f", p.DesSpeed)
	}
}

// TestTractorBeamPhysics tests tractor beam force application
func TestTractorBeamPhysics(t *testing.T) {
	server := NewServer()
	gs := server.gameState

	// Setup source ship (tractoring)
	source := gs.Players[0]
	source.Status = game.StatusAlive
	source.Team = game.TeamFed
	source.Ship = game.ShipDestroyer
	source.X = 50000
	source.Y = 50000
	source.Tractoring = 1
	source.Pressoring = -1
	source.Fuel = 10000
	source.ETemp = 0
	source.EngineOverheat = false
	source.Orbiting = -1

	// Setup target ship
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipDestroyer
	target.X = 52000
	target.Y = 50000
	target.Orbiting = -1

	initialSourceX := source.X
	initialTargetX := target.X
	initialFuel := source.Fuel

	// Run tractor beam update
	server.updateTractorBeams()

	// Source should move toward target
	if source.X <= initialSourceX {
		t.Errorf("Source ship should move toward target, initial=%.1f, final=%.1f", initialSourceX, source.X)
	}

	// Target should move toward source
	if target.X >= initialTargetX {
		t.Errorf("Target ship should move toward source, initial=%.1f, final=%.1f", initialTargetX, target.X)
	}

	// Fuel should be consumed
	if source.Fuel >= initialFuel {
		t.Errorf("Fuel should be consumed, initial=%d, final=%d", initialFuel, source.Fuel)
	}

	// Engine temp should increase
	if source.ETemp == 0 {
		t.Errorf("Engine temp should increase from tractor beam")
	}
}

// TestPressorBeamPhysics tests pressor beam force application
func TestPressorBeamPhysics(t *testing.T) {
	server := NewServer()
	gs := server.gameState

	// Setup source ship (pressoring)
	source := gs.Players[0]
	source.Status = game.StatusAlive
	source.Team = game.TeamFed
	source.Ship = game.ShipDestroyer
	source.X = 50000
	source.Y = 50000
	source.Tractoring = -1
	source.Pressoring = 1
	source.Fuel = 10000
	source.EngineOverheat = false
	source.Orbiting = -1

	// Setup target ship
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipDestroyer
	target.X = 52000
	target.Y = 50000
	target.Orbiting = -1

	initialSourceX := source.X
	initialTargetX := target.X

	// Run pressor beam update
	server.updateTractorBeams()

	// Source should move away from target
	if source.X >= initialSourceX {
		t.Errorf("Source ship should move away from target, initial=%.1f, final=%.1f", initialSourceX, source.X)
	}

	// Target should move away from source
	if target.X <= initialTargetX {
		t.Errorf("Target ship should move away from source, initial=%.1f, final=%.1f", initialTargetX, target.X)
	}
}

// TestTractorBeamBreaksOrbit tests that tractor beam pulls ships out of orbit
func TestTractorBeamBreaksOrbit(t *testing.T) {
	server := NewServer()
	gs := server.gameState

	// Setup planet
	planet := gs.Planets[0]
	planet.X = 50000
	planet.Y = 50000

	// Setup source ship (tractoring)
	source := gs.Players[0]
	source.Status = game.StatusAlive
	source.Team = game.TeamFed
	source.Ship = game.ShipDestroyer
	source.X = 55000
	source.Y = 50000
	source.Tractoring = 1
	source.Pressoring = -1
	source.Fuel = 10000
	source.EngineOverheat = false
	source.Orbiting = -1

	// Setup target ship orbiting planet
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipDestroyer
	target.X = planet.X + 1000
	target.Y = planet.Y
	target.Orbiting = 0
	target.Bombing = true
	target.Beaming = true

	// Run tractor beam update
	server.updateTractorBeams()

	// Target should no longer be orbiting
	if target.Orbiting != -1 {
		t.Errorf("Tractor beam should break orbit, got Orbiting=%d", target.Orbiting)
	}

	// Bombing and beaming should be stopped
	if target.Bombing {
		t.Errorf("Bombing should stop when orbit is broken")
	}
	if target.Beaming {
		t.Errorf("Beaming should stop when orbit is broken")
	}
}

// TestTractorBeamRangeLimit tests that tractor beam breaks at max range
func TestTractorBeamRangeLimit(t *testing.T) {
	server := NewServer()
	gs := server.gameState

	shipStats := game.ShipData[game.ShipDestroyer]
	tractorRange := float64(game.TractorDist) * shipStats.TractorRange

	// Setup source ship (tractoring)
	source := gs.Players[0]
	source.Status = game.StatusAlive
	source.Team = game.TeamFed
	source.Ship = game.ShipDestroyer
	source.X = 50000
	source.Y = 50000
	source.Tractoring = 1
	source.Pressoring = -1
	source.Fuel = 10000
	source.EngineOverheat = false
	source.Orbiting = -1

	// Setup target ship out of range
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipDestroyer
	target.X = source.X + tractorRange + 100 // Just out of range
	target.Y = source.Y
	target.Orbiting = -1

	// Run tractor beam update
	server.updateTractorBeams()

	// Beam should be broken
	if source.Tractoring != -1 {
		t.Errorf("Tractor beam should break at max range, got Tractoring=%d", source.Tractoring)
	}
}

// TestTractorBeamFuelDepletion tests that tractor beam stops when out of fuel
func TestTractorBeamFuelDepletion(t *testing.T) {
	server := NewServer()
	gs := server.gameState

	// Setup source ship with low fuel
	source := gs.Players[0]
	source.Status = game.StatusAlive
	source.Team = game.TeamFed
	source.Ship = game.ShipDestroyer
	source.X = 50000
	source.Y = 50000
	source.Tractoring = 1
	source.Pressoring = -1
	source.Fuel = 10 // Very low fuel
	source.EngineOverheat = false
	source.Orbiting = -1

	// Setup target ship
	target := gs.Players[1]
	target.Status = game.StatusAlive
	target.Team = game.TeamKli
	target.Ship = game.ShipDestroyer
	target.X = 52000
	target.Y = 50000
	target.Orbiting = -1

	// Run tractor beam update
	server.updateTractorBeams()

	// Beam should be broken when fuel runs out
	if source.Tractoring != -1 {
		t.Errorf("Tractor beam should stop when out of fuel, got Tractoring=%d", source.Tractoring)
	}
}

// TestAlertLevels tests alert level calculations
func TestAlertLevels(t *testing.T) {
	tests := []struct {
		name          string
		enemyDistance float64
		expectedAlert string
	}{
		{
			name:          "Red alert - enemy close",
			enemyDistance: 8000,
			expectedAlert: "red",
		},
		{
			name:          "Yellow alert - enemy moderate distance",
			enemyDistance: 12000,
			expectedAlert: "yellow",
		},
		{
			name:          "Green alert - enemy far",
			enemyDistance: 20000,
			expectedAlert: "green",
		},
		{
			name:          "Red alert - enemy very close",
			enemyDistance: 5000,
			expectedAlert: "red",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs := game.NewGameState()
			server := &Server{gameState: gs}

			// Setup player
			p := gs.Players[0]
			p.Status = game.StatusAlive
			p.Team = game.TeamFed
			p.Ship = game.ShipDestroyer
			p.X = 50000
			p.Y = 50000

			// Setup enemy
			enemy := gs.Players[1]
			enemy.Status = game.StatusAlive
			enemy.Team = game.TeamKli
			enemy.Ship = game.ShipDestroyer
			enemy.X = p.X + tt.enemyDistance
			enemy.Y = p.Y

			// Calculate alert levels
			server.updateAlertLevels()

			if p.AlertLevel != tt.expectedAlert {
				t.Errorf("Expected alert level %s, got %s (distance: %.1f)",
					tt.expectedAlert, p.AlertLevel, tt.enemyDistance)
			}
		})
	}
}

// TestAlertLevelIgnoresTeammates tests that alert level ignores same-team players
func TestAlertLevelIgnoresTeammates(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	// Setup player
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipDestroyer
	p.X = 50000
	p.Y = 50000

	// Setup teammate very close
	teammate := gs.Players[1]
	teammate.Status = game.StatusAlive
	teammate.Team = game.TeamFed // Same team
	teammate.Ship = game.ShipDestroyer
	teammate.X = p.X + 100 // Very close
	teammate.Y = p.Y

	// Calculate alert levels
	server.updateAlertLevels()

	// Should remain green despite close teammate
	if p.AlertLevel != "green" {
		t.Errorf("Alert level should ignore teammates, got %s", p.AlertLevel)
	}
}

// TestAlertLevelRedOverridesYellow tests that red alert takes priority
func TestAlertLevelRedOverridesYellow(t *testing.T) {
	gs := game.NewGameState()
	server := &Server{gameState: gs}

	// Setup player
	p := gs.Players[0]
	p.Status = game.StatusAlive
	p.Team = game.TeamFed
	p.Ship = game.ShipDestroyer
	p.X = 50000
	p.Y = 50000

	// Setup one enemy at yellow range
	enemy1 := gs.Players[1]
	enemy1.Status = game.StatusAlive
	enemy1.Team = game.TeamKli
	enemy1.Ship = game.ShipDestroyer
	enemy1.X = p.X + 12000 // Yellow range
	enemy1.Y = p.Y

	// Setup another enemy at red range
	enemy2 := gs.Players[2]
	enemy2.Status = game.StatusAlive
	enemy2.Team = game.TeamRom
	enemy2.Ship = game.ShipDestroyer
	enemy2.X = p.X + 8000 // Red range
	enemy2.Y = p.Y

	// Calculate alert levels
	server.updateAlertLevels()

	// Should be red (not yellow)
	if p.AlertLevel != "red" {
		t.Errorf("Red alert should override yellow, got %s", p.AlertLevel)
	}
}
