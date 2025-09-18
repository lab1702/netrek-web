package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// OrbitalVelocity returns the instantaneous tangential velocity of a ship that is
// currently orbiting a planet. If the ship is not orbiting, ok will be false.
//
// The velocity is returned in world units per tick, which is the same unit that
// InterceptDirectionSimple expects for target velocity calculations.
func (s *Server) OrbitalVelocity(p *game.Player) (vx, vy float64, ok bool) {
	// Check if player is orbiting a planet
	if p.Orbiting < 0 || p.Orbiting >= len(s.gameState.Planets) {
		return 0, 0, false
	}

	planet := s.gameState.Planets[p.Orbiting]
	if planet == nil {
		return 0, 0, false
	}

	// Calculate radius vector from planet to ship
	dx := p.X - planet.X
	dy := p.Y - planet.Y
	radius := math.Sqrt(dx*dx + dy*dy)

	// Avoid division by zero
	if radius < 1e-9 {
		return 0, 0, false
	}

	// From physics.go, we know that orbiting ships have their direction
	// incremented by π/64 radians per tick, and the ship direction is
	// tangent to the orbit circle.
	//
	// The angular velocity is ω = π/64 rad/tick
	// The tangential speed is v = ω * r = (π/64) * radius
	angularVelocity := math.Pi / 64.0
	tangentialSpeed := angularVelocity * radius

	// Calculate unit tangent vector
	// Since ship direction is tangent to orbit, we can use it directly
	// or calculate it as perpendicular to radius vector
	//
	// The direction of orbital motion (from physics.go updatePlayerOrbit):
	// Ship moves counter-clockwise (positive angular velocity)
	// Tangent vector is radius rotated 90° counter-clockwise
	unitTangentX := -dy / radius // perpendicular to radius, counter-clockwise
	unitTangentY := dx / radius

	// Calculate velocity components
	vx = tangentialSpeed * unitTangentX
	vy = tangentialSpeed * unitTangentY

	return vx, vy, true
}
