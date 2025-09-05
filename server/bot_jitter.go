package server

import (
	"math"
	"math/rand"
	"time"
)

// maxJitterDeg is the maximum random angle deviation in degrees for bot torpedo firing
const maxJitterDeg = 5.0

func init() {
	// Seed the random number generator once at startup
	rand.Seed(time.Now().UnixNano())
}

// randomJitterRad returns a random angle in radians within ±maxJitterDeg
// This adds unpredictability to bot torpedo firing to make them harder to dodge
func randomJitterRad() float64 {
	// Generate uniform random value between -1 and 1, then scale by maxJitterDeg
	deg := (rand.Float64()*2 - 1) * maxJitterDeg
	// Convert degrees to radians
	return deg * math.Pi / 180
}
