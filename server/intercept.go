package server

import (
	"math"
)

// Point2D represents a 2D position
type Point2D struct {
	X, Y float64
}

// Vector2D represents a 2D velocity vector
type Vector2D struct {
	X, Y float64
}

// InterceptSolution contains the result of an intercept calculation
type InterceptSolution struct {
	Direction       float64 // Direction to fire in radians
	TimeToIntercept float64 // Time until projectile reaches target
	InterceptPoint  Point2D // Where the intercept will occur
}

// InterceptDirection calculates the direction to fire a projectile to intercept a moving target.
// This is a pure mathematical function using the standard 2D intercept formula.
//
// Parameters:
//
//	shooterPos: Position of the shooter (world units)
//	targetPos: Position of the target (world units)
//	targetVel: Velocity of target (world units per tick)
//	projSpeed: Speed of projectile (world units per tick)
//
// Returns:
//
//	solution: The intercept solution, or nil if no solution exists
//	ok: true if a valid intercept solution was found
func InterceptDirection(shooterPos, targetPos Point2D, targetVel Vector2D, projSpeed float64) (*InterceptSolution, bool) {
	// Check for invalid projectile speed
	if projSpeed <= 0 {
		return nil, false
	}

	// Relative position (target relative to shooter)
	relX := targetPos.X - shooterPos.X
	relY := targetPos.Y - shooterPos.Y

	// Check for zero distance (target at shooter position)
	distSq := relX*relX + relY*relY
	if distSq < 1e-9 {
		// Target is essentially at shooter position - time should be near-zero but positive
		return &InterceptSolution{
			Direction:       0.0,
			TimeToIntercept: 1e-6, // Very small positive time instead of zero
			InterceptPoint:  shooterPos,
		}, true
	}

	// Relative velocity (target velocity relative to shooter, since shooter is stationary)
	relVelX := targetVel.X
	relVelY := targetVel.Y

	// Check for stationary target
	velSq := relVelX*relVelX + relVelY*relVelY
	if velSq < 1e-9 {
		// Stationary target - fire directly at it
		direction := math.Atan2(relY, relX)
		distance := math.Sqrt(distSq)
		timeToIntercept := distance / projSpeed

		return &InterceptSolution{
			Direction:       direction,
			TimeToIntercept: timeToIntercept,
			InterceptPoint:  targetPos,
		}, true
	}

	// Solve the quadratic equation for intercept
	// We want to find time t such that:
	// |targetPos + targetVel*t - shooterPos| = projSpeed*t
	//
	// This expands to the quadratic: a*t² + b*t + c = 0
	// where:
	// a = velSq - projSpeed²
	// b = 2 * (relX*relVelX + relY*relVelY)
	// c = distSq
	//
	a := velSq - projSpeed*projSpeed
	b := 2.0 * (relX*relVelX + relY*relVelY)
	c := distSq

	// Check for linear case (projectile and target have same speed)
	if math.Abs(a) < 1e-9 {
		if math.Abs(b) < 1e-9 {
			// Both a and b are zero - degenerate case
			return nil, false
		}
		// Linear case: b*t + c = 0
		t := -c / b
		if t < 0 {
			return nil, false // Solution is in the past
		}

		// Calculate intercept point and direction
		interceptX := targetPos.X + targetVel.X*t
		interceptY := targetPos.Y + targetVel.Y*t
		direction := math.Atan2(interceptY-shooterPos.Y, interceptX-shooterPos.X)

		return &InterceptSolution{
			Direction:       direction,
			TimeToIntercept: t,
			InterceptPoint:  Point2D{X: interceptX, Y: interceptY},
		}, true
	}

	// Quadratic case - check discriminant
	discriminant := b*b - 4*a*c
	if discriminant < 0 {
		// No real solution - target is too fast to intercept
		return nil, false
	}

	// Calculate both roots
	sqrtDiscriminant := math.Sqrt(discriminant)
	t1 := (-b + sqrtDiscriminant) / (2 * a)
	t2 := (-b - sqrtDiscriminant) / (2 * a)

	// Choose the smallest positive time
	var t float64
	if t1 > 0 && t2 > 0 {
		t = math.Min(t1, t2)
	} else if t1 > 0 {
		t = t1
	} else if t2 > 0 {
		t = t2
	} else {
		// Both solutions are in the past
		return nil, false
	}

	// Calculate intercept point and direction
	interceptX := targetPos.X + targetVel.X*t
	interceptY := targetPos.Y + targetVel.Y*t
	direction := math.Atan2(interceptY-shooterPos.Y, interceptX-shooterPos.X)

	return &InterceptSolution{
		Direction:       direction,
		TimeToIntercept: t,
		InterceptPoint:  Point2D{X: interceptX, Y: interceptY},
	}, true
}

// InterceptDirectionSimple is a simplified version that only returns direction and success
// This is the interface that the existing bot code will use
func InterceptDirectionSimple(shooterPos, targetPos Point2D, targetVel Vector2D, projSpeed float64) (float64, bool) {
	solution, ok := InterceptDirection(shooterPos, targetPos, targetVel, projSpeed)
	if !ok {
		// Fallback to direct shot
		direction := math.Atan2(targetPos.Y-shooterPos.Y, targetPos.X-shooterPos.X)
		return direction, false
	}
	return solution.Direction, true
}

// NormalizeAngleSigned normalizes an angle to the range [-π, π].
// This differs from game.NormalizeAngle which normalizes to [0, 2π].
func NormalizeAngleSigned(angle float64) float64 {
	if math.IsNaN(angle) || math.IsInf(angle, 0) {
		return 0
	}
	angle = math.Mod(angle, 2*math.Pi)
	if angle > math.Pi {
		angle -= 2 * math.Pi
	} else if angle <= -math.Pi {
		angle += 2 * math.Pi
	}
	return angle
}

// AngleDifference calculates the smallest angle difference between two angles
func AngleDifference(a1, a2 float64) float64 {
	diff := NormalizeAngleSigned(a1 - a2)
	return math.Abs(diff)
}
