package server

import (
	"flag"
	"fmt"
	"math"
	"os"
	"testing"
	"time"

	"github.com/lab1702/netrek-web/game"
)

// Test configuration flags
var (
	iterations    = flag.Int("iterations", 100, "Number of test iterations to run")
	targetSpeed   = flag.Float64("target-speed", 8.0, "Target ship speed")
	targetPattern = flag.String("target-pattern", "straight", "Target movement pattern: straight, circle, zigzag")
	verbose       = flag.Bool("verbose", false, "Enable verbose logging of each torpedo launch")
	baseline      = flag.Bool("baseline", false, "Capture baseline accuracy metrics")
)

// TorpedoLaunchData captures data about each torpedo launch for analysis
type TorpedoLaunchData struct {
	LaunchPos         Point   `json:"launch_pos"`
	TargetPos         Point   `json:"target_pos"`
	TargetVelocity    Vector  `json:"target_velocity"`
	PredictedHitPos   Point   `json:"predicted_hit_pos"`
	FireDirection     float64 `json:"fire_direction"`
	TorpedoSpeed      float64 `json:"torpedo_speed"`
	ActualClosestDist float64 `json:"actual_closest_dist"`
	TimeToClosest     float64 `json:"time_to_closest"`
	Hit               bool    `json:"hit"`
}

// Point represents a 2D coordinate
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Vector represents a 2D velocity vector
type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// AccuracyMetrics holds statistics about torpedo accuracy
type AccuracyMetrics struct {
	TotalShots         int                 `json:"total_shots"`
	Hits               int                 `json:"hits"`
	HitRate            float64             `json:"hit_rate"`
	AverageClosestDist float64             `json:"average_closest_dist"`
	MedianClosestDist  float64             `json:"median_closest_dist"`
	MaxClosestDist     float64             `json:"max_closest_dist"`
	MinClosestDist     float64             `json:"min_closest_dist"`
	LaunchData         []TorpedoLaunchData `json:"launch_data"`
}

// TestTorpedoAccuracyBaseline creates a controlled environment to measure
// current torpedo accuracy and establish a baseline
func TestTorpedoAccuracyBaseline(t *testing.T) {
	if !*baseline {
		t.Skip("Baseline test skipped - use -baseline flag to run")
	}

	fmt.Printf("=== Torpedo Accuracy Baseline Test ===\n")
	fmt.Printf("Iterations: %d, Target Speed: %.1f, Pattern: %s\n\n",
		*iterations, *targetSpeed, *targetPattern)

	metrics := runAccuracyTest(t, *iterations, *targetSpeed, *targetPattern)

	fmt.Printf("=== BASELINE RESULTS ===\n")
	fmt.Printf("Total Shots:         %d\n", metrics.TotalShots)
	fmt.Printf("Hits:                %d\n", metrics.Hits)
	fmt.Printf("Hit Rate:            %.2f%%\n", metrics.HitRate*100)
	fmt.Printf("Avg Closest Dist:    %.1f units\n", metrics.AverageClosestDist)
	fmt.Printf("Median Closest Dist: %.1f units\n", metrics.MedianClosestDist)
	fmt.Printf("Min Closest Dist:    %.1f units\n", metrics.MinClosestDist)
	fmt.Printf("Max Closest Dist:    %.1f units\n", metrics.MaxClosestDist)

	// Save baseline results to file
	saveBaselineResults(metrics)
}

// runAccuracyTest executes the torpedo accuracy test with given parameters
func runAccuracyTest(t *testing.T, iterations int, speed float64, pattern string) AccuracyMetrics {
	var launchData []TorpedoLaunchData
	totalClosestDist := 0.0

	for i := 0; i < iterations; i++ {
		// Create fresh game state for each test
		gs := game.NewGameState()
		testServer := &Server{}
		testServer.SetGameState(gs) // Assume this method exists or we'll create it

		// Set up bot shooter
		shooter := gs.Players[0]
		shooter.Status = game.StatusAlive
		shooter.Team = game.TeamFed
		shooter.Ship = game.ShipDestroyer // Standard ship for consistency
		shooter.X = 50000
		shooter.Y = 50000
		shooter.Dir = 0
		shooter.Speed = 0
		shooter.Fuel = 10000
		shooter.WTemp = 0
		shooter.NumTorps = 0
		shooter.IsBot = true

		// Set up target with specific movement pattern
		target := gs.Players[1]
		target.Status = game.StatusAlive
		target.Team = game.TeamKli
		target.Ship = game.ShipCruiser

		// Position target at varying distances and angles
		angle := float64(i) * 2 * math.Pi / float64(iterations)
		distance := 5000.0 + float64(i%3)*2000 // Vary distance: 5k, 7k, 9k
		target.X = shooter.X + distance*math.Cos(angle)
		target.Y = shooter.Y + distance*math.Sin(angle)

		// Set target movement based on pattern
		setTargetMovement(target, speed, pattern, float64(i))

		// Record initial state
		launchPos := Point{X: shooter.X, Y: shooter.Y}
		targetPos := Point{X: target.X, Y: target.Y}
		targetVel := Vector{
			X: target.Speed * math.Cos(target.Dir), // Already in world units per game tick
			Y: target.Speed * math.Sin(target.Dir),
		}

		// Fire torpedo using current bot logic
		initialTorpCount := len(gs.Torps)
		testServer.FireBotTorpedoWithLead(shooter, target) // We'll need to expose this method

		if len(gs.Torps) <= initialTorpCount {
			t.Errorf("Iteration %d: No torpedo was fired", i)
			continue
		}

		// Get the fired torpedo
		torpedo := gs.Torps[len(gs.Torps)-1]

		// Calculate predicted hit position (where we aimed)
		shipStats := game.ShipData[shooter.Ship]
		torpSpeed := float64(shipStats.TorpSpeed * 20)
		timeToIntercept := distance / torpSpeed
		predictedHitPos := Point{
			X: target.X + targetVel.X*timeToIntercept*20, // targetVel in units per game tick
			Y: target.Y + targetVel.Y*timeToIntercept*20,
		}

		// Simulate torpedo flight and find closest approach
		closestDist, timeToClosest := simulateClosestApproach(torpedo, target, targetVel)
		hit := closestDist <= 600 // Standard torpedo hit radius

		// Record launch data
		launch := TorpedoLaunchData{
			LaunchPos:         launchPos,
			TargetPos:         targetPos,
			TargetVelocity:    targetVel,
			PredictedHitPos:   predictedHitPos,
			FireDirection:     torpedo.Dir,
			TorpedoSpeed:      torpedo.Speed,
			ActualClosestDist: closestDist,
			TimeToClosest:     timeToClosest,
			Hit:               hit,
		}

		launchData = append(launchData, launch)
		totalClosestDist += closestDist

		if *verbose {
			fmt.Printf("Shot %3d: Closest dist %.1f, Hit: %v\n", i+1, closestDist, hit)
		}
	}

	// Calculate metrics
	hits := 0
	var closestDists []float64
	for _, launch := range launchData {
		if launch.Hit {
			hits++
		}
		closestDists = append(closestDists, launch.ActualClosestDist)
	}

	// Sort for median calculation
	sortFloat64Slice(closestDists)

	metrics := AccuracyMetrics{
		TotalShots:         len(launchData),
		Hits:               hits,
		HitRate:            float64(hits) / float64(len(launchData)),
		AverageClosestDist: totalClosestDist / float64(len(launchData)),
		MedianClosestDist:  closestDists[len(closestDists)/2],
		MinClosestDist:     closestDists[0],
		MaxClosestDist:     closestDists[len(closestDists)-1],
		LaunchData:         launchData,
	}

	return metrics
}

// setTargetMovement configures target ship movement based on pattern
func setTargetMovement(target *game.Player, speed float64, pattern string, iteration float64) {
	target.Speed = speed

	switch pattern {
	case "straight":
		// Move in random direction
		target.Dir = math.Mod(iteration*0.7, 2*math.Pi)
		target.DesDir = target.Dir
		target.DesSpeed = speed

	case "circle":
		// Circular motion around a center point
		center := Point{X: 55000, Y: 55000}
		radius := 3000.0
		angularSpeed := 0.1 // radians per iteration
		angle := iteration * angularSpeed

		target.X = center.X + radius*math.Cos(angle)
		target.Y = center.Y + radius*math.Sin(angle)
		target.Dir = angle + math.Pi/2 // Tangent to circle
		target.DesDir = target.Dir
		target.DesSpeed = speed

	case "zigzag":
		// Zigzag pattern
		if int(iteration)%10 < 5 {
			target.Dir = 0 // East
		} else {
			target.Dir = math.Pi // West
		}
		target.DesDir = target.Dir
		target.DesSpeed = speed

	default:
		// Default to straight
		target.Dir = 0
		target.DesDir = 0
		target.DesSpeed = speed
	}
}

// simulateClosestApproach simulates torpedo and target movement to find closest approach
func simulateClosestApproach(torpedo *game.Torpedo, target *game.Player, targetVel Vector) (float64, float64) {
	minDist := math.Inf(1)
	minTime := 0.0

	torpX, torpY := torpedo.X, torpedo.Y
	targX, targY := target.X, target.Y

	torpVelX := torpedo.Speed * math.Cos(torpedo.Dir)
	torpVelY := torpedo.Speed * math.Sin(torpedo.Dir)

	// Simulate for torpedo fuse time
	maxTicks := float64(game.ShipData[game.ShipDestroyer].TorpFuse)

	for tick := 0.0; tick <= maxTicks; tick += 0.1 {
		// Update positions
		currentTorpX := torpX + torpVelX*tick
		currentTorpY := torpY + torpVelY*tick
		currentTargX := targX + targetVel.X*tick*20 // targetVel is in units per game tick, tick is in game ticks
		currentTargY := targY + targetVel.Y*tick*20

		// Calculate distance
		dist := math.Sqrt(math.Pow(currentTorpX-currentTargX, 2) + math.Pow(currentTorpY-currentTargY, 2))

		if dist < minDist {
			minDist = dist
			minTime = tick
		}
	}

	return minDist, minTime
}

// saveBaselineResults saves the baseline metrics to a file
func saveBaselineResults(metrics AccuracyMetrics) {
	// Create results directory if it doesn't exist
	// Use a path relative to the project root (one level up from server/)
	docsDir := "../docs"
	os.MkdirAll(docsDir, 0755)

	filename := docsDir + "/torp_aim_baseline.txt"
	file, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Warning: Could not save baseline results: %v\n", err)
		return
	}
	defer file.Close()

	fmt.Fprintf(file, "NETREK BOT TORPEDO ACCURACY BASELINE\n")
	fmt.Fprintf(file, "Generated: %s\n\n", time.Now().Format("2006-01-02"))
	fmt.Fprintf(file, "Total Shots:         %d\n", metrics.TotalShots)
	fmt.Fprintf(file, "Hits:                %d\n", metrics.Hits)
	fmt.Fprintf(file, "Hit Rate:            %.2f%%\n", metrics.HitRate*100)
	fmt.Fprintf(file, "Avg Closest Dist:    %.1f units\n", metrics.AverageClosestDist)
	fmt.Fprintf(file, "Median Closest Dist: %.1f units\n", metrics.MedianClosestDist)
	fmt.Fprintf(file, "Min Closest Dist:    %.1f units\n", metrics.MinClosestDist)
	fmt.Fprintf(file, "Max Closest Dist:    %.1f units\n", metrics.MaxClosestDist)

	fmt.Printf("Baseline results saved to %s\n", filename)
}

// sortFloat64Slice sorts a float64 slice (simple bubble sort for small slices)
func sortFloat64Slice(slice []float64) {
	n := len(slice)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if slice[j] > slice[j+1] {
				slice[j], slice[j+1] = slice[j+1], slice[j]
			}
		}
	}
}
