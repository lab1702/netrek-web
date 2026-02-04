package server

import (
	"math"

	"github.com/lab1702/netrek-web/game"
)

// SpatialGrid provides O(1) average case lookup for nearby entities
// using a grid-based spatial hash. This reduces collision detection
// from O(n*m) to O(n) average case.
type SpatialGrid struct {
	cellSize float64
	cols     int
	rows     int
	cells    [][]int // Each cell contains player IDs
}

// GridCellSize is the size of each grid cell in game units.
// Should be at least as large as the maximum collision detection distance.
// Using 3000 to cover plasma explosion radius (1500) with some margin.
const GridCellSize = 3000.0

// NewSpatialGrid creates a new spatial grid for the galaxy
func NewSpatialGrid() *SpatialGrid {
	cols := int(math.Ceil(float64(game.GalaxyWidth) / GridCellSize))
	rows := int(math.Ceil(float64(game.GalaxyHeight) / GridCellSize))

	cells := make([][]int, cols*rows)
	for i := range cells {
		cells[i] = make([]int, 0, 4) // Pre-allocate small capacity
	}

	return &SpatialGrid{
		cellSize: GridCellSize,
		cols:     cols,
		rows:     rows,
		cells:    cells,
	}
}

// Clear resets the grid for a new frame
func (g *SpatialGrid) Clear() {
	for i := range g.cells {
		g.cells[i] = g.cells[i][:0] // Reuse underlying array
	}
}

// cellIndex returns the cell index for a position
func (g *SpatialGrid) cellIndex(x, y float64) int {
	col := int(x / g.cellSize)
	row := int(y / g.cellSize)

	// Clamp to valid range
	if col < 0 {
		col = 0
	} else if col >= g.cols {
		col = g.cols - 1
	}
	if row < 0 {
		row = 0
	} else if row >= g.rows {
		row = g.rows - 1
	}

	return row*g.cols + col
}

// Insert adds a player to the grid
func (g *SpatialGrid) Insert(playerID int, x, y float64) {
	idx := g.cellIndex(x, y)
	g.cells[idx] = append(g.cells[idx], playerID)
}

// GetNearby returns player IDs that might be within range of the given position.
// The caller must still perform exact distance checks.
func (g *SpatialGrid) GetNearby(x, y float64) []int {
	col := int(x / g.cellSize)
	row := int(y / g.cellSize)

	// Collect players from the current cell and all 8 adjacent cells
	var result []int

	for dr := -1; dr <= 1; dr++ {
		for dc := -1; dc <= 1; dc++ {
			c := col + dc
			r := row + dr

			// Skip out-of-bounds cells
			if c < 0 || c >= g.cols || r < 0 || r >= g.rows {
				continue
			}

			idx := r*g.cols + c
			result = append(result, g.cells[idx]...)
		}
	}

	return result
}

// IndexPlayers populates the grid with all alive players
func (g *SpatialGrid) IndexPlayers(players []*game.Player) {
	g.Clear()
	for i, p := range players {
		if p.Status == game.StatusAlive {
			g.Insert(i, p.X, p.Y)
		}
	}
}
