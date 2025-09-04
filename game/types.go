package game

import (
	"math"
	"sync"
	"time"
)

// Constants from original Netrek
const (
	MaxPlayers = 64
	MaxPlanets = 40
	MaxTorps   = 8
	MaxPlasma  = 1

	// Galaxy dimensions
	GalaxyWidth  = 100000
	GalaxyHeight = 100000

	// Movement constants
	WARP1 = 60 // Units per tick at warp 1 (from original Netrek)

	// Distance constants
	ExplosionDist = 350
	DamageDist    = 2000
	PhaserDist    = 6000 // Base phaser range constant from original Netrek
	TractorDist   = 6000
	EntOrbitDist  = 900 // Maximum distance to enter orbit
	OrbitDist     = 800 // Actual orbit radius
	DockDist      = 600
	ORBSPEED      = 2 // Maximum speed to enter orbit (from original Netrek)

	// Ship explosion constants (from original Netrek)
	ShipExplosionDist    = 350  // Ships within this distance take full damage
	ShipExplosionMaxDist = 3000 // Maximum explosion damage radius
	ShipExplosionRange   = 2650 // Used in damage falloff calculation (MaxDist - ExplosionDist)

	// Phaser hit detection constants (from original Netrek)
	ZAPPLAYERDIST = 390 // Phaser will hit player if line is this close
	ZAPPLASMADIST = 270 // Phaser will hit plasma if line is this close

	// Game timing
	FPS            = 10
	UpdateInterval = time.Millisecond * 100 // 10 FPS (10 ticks per second)
)

// Team IDs
const (
	TeamNone = 0
	TeamFed  = 1 << 0
	TeamRom  = 1 << 1
	TeamKli  = 1 << 2
	TeamOri  = 1 << 3
)

// Team home positions for respawn
var TeamHomeX = map[int]int{
	TeamFed: 20000,
	TeamRom: 20000,
	TeamKli: 80000,
	TeamOri: 80000,
}

var TeamHomeY = map[int]int{
	TeamFed: 80000,
	TeamRom: 20000,
	TeamKli: 20000,
	TeamOri: 80000,
}

// Player Status
const (
	StatusFree    = 0
	StatusOutfit  = 1
	StatusAlive   = 2
	StatusExplode = 3
	StatusDead    = 4
	StatusObserve = 6
)

// Death reasons (why player died)
const (
	KillTorp      = 0 // Killed by torpedo
	KillPhaser    = 1 // Killed by phaser
	KillPlanet    = 2 // Killed by planet
	KillExplosion = 3 // Killed by explosion
	KillQuit      = 4 // Player quit
	KillDaemon    = 5 // Server killed player
)

// Planet combat constants
const (
	PlanetFireDist = 1500 // Distance at which planets fire at enemy ships
)

// Army constants
const (
	ArmyKillRequirement = 2.0 // Kills required to pick up armies (classic Netrek rule)
)

// Ship Types
type ShipType int

const (
	ShipScout ShipType = iota
	ShipDestroyer
	ShipCruiser
	ShipBattleship
	ShipAssault
	ShipStarbase
	ShipGalaxy
)

// ShipStats holds the specifications for each ship type
type ShipStats struct {
	Name         string
	MaxSpeed     int
	MaxFuel      int
	MaxShields   int
	MaxDamage    int
	MaxArmies    int
	TorpDamage   int
	TorpSpeed    int
	TorpFuse     int
	PhaserDamage int
	PlasmaDamage int
	PlasmaSpeed  int
	PlasmaFuse   int // Plasma torpedo fuse time (in ticks)
	TurnRate     int // Turn rate in original units (higher = faster turning)
	Mass         int
	TractorStr   int
	HasPlasma    bool
	// Temperature limits
	MaxWpnTemp int // Maximum weapon temperature
	MaxEngTemp int // Maximum engine temperature
	// Fuel cost multipliers (from original Netrek)
	TorpFuelMult   int // Multiplier for torpedo fuel cost (damage * mult)
	PhaserFuelMult int // Multiplier for phaser fuel cost (damage * mult)
	PlasmaFuelMult int // Multiplier for plasma fuel cost (damage * mult)
	// Tractor/Pressor range multiplier
	TractorRange float64 // Multiplier for tractor/pressor range
	// Movement physics
	AccInt int // Acceleration integer (higher = faster acceleration)
	DecInt int // Deceleration integer (higher = faster deceleration)
	// Ship systems
	RepairRate   int // Repair rate (damage points per tick when repairing)
	FuelRecharge int // Fuel recharge rate (fuel points per tick)
	WpnCool      int // Weapon cooling rate (temp units per tick)
	EngCool      int // Engine cooling rate (temp units per tick)
	CloakCost    int // Fuel cost per tick when cloaked
	DetCost      int // Fuel cost for detonating enemy torpedoes
}

var ShipData = map[ShipType]ShipStats{
	ShipScout: {
		Name:           "Scout",
		MaxSpeed:       12,
		MaxFuel:        5000,
		MaxShields:     75,
		MaxDamage:      75,
		MaxArmies:      2,
		TorpDamage:     25,
		TorpSpeed:      16,
		TorpFuse:       16,
		PhaserDamage:   75,
		TurnRate:       570000, // Original Netrek turn rate
		Mass:           1500,
		TractorStr:     2000,
		HasPlasma:      false,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1000,
		TorpFuelMult:   7,
		PhaserFuelMult: 7,
		TractorRange:   0.7,
		AccInt:         200,
		DecInt:         270,
		RepairRate:     80,
		FuelRecharge:   8,
		WpnCool:        2,
		EngCool:        5,
		CloakCost:      17,
		DetCost:        100,
	},
	ShipDestroyer: {
		Name:           "Destroyer",
		MaxSpeed:       10,
		MaxFuel:        7000,
		MaxShields:     85,
		MaxDamage:      85,
		MaxArmies:      5,
		TorpDamage:     30,
		TorpSpeed:      14,
		TorpFuse:       30,
		PhaserDamage:   85,
		PlasmaDamage:   75,
		PlasmaSpeed:    15,
		PlasmaFuse:     30,     // Original Netrek value
		TurnRate:       310000, // Original Netrek turn rate
		Mass:           1800,
		TractorStr:     2500,
		HasPlasma:      true,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1000,
		TorpFuelMult:   7,
		PhaserFuelMult: 7,
		PlasmaFuelMult: 30,
		TractorRange:   0.9,
		AccInt:         200,
		DecInt:         300,
		RepairRate:     100,
		FuelRecharge:   11,
		WpnCool:        2,
		EngCool:        5,
		CloakCost:      21,
		DetCost:        100,
	},
	ShipCruiser: {
		Name:           "Cruiser",
		MaxSpeed:       9,
		MaxFuel:        10000,
		MaxShields:     100,
		MaxDamage:      100,
		MaxArmies:      10,
		TorpDamage:     40,
		TorpSpeed:      12,
		TorpFuse:       40,
		PhaserDamage:   100,
		PlasmaDamage:   100,
		PlasmaSpeed:    15,
		PlasmaFuse:     35,     // Original Netrek value
		TurnRate:       170000, // Original Netrek turn rate
		Mass:           2000,
		TractorStr:     3000,
		HasPlasma:      true,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1000,
		TorpFuelMult:   7,
		PhaserFuelMult: 7,
		PlasmaFuelMult: 30,
		TractorRange:   1.0,
		AccInt:         150,
		DecInt:         200,
		RepairRate:     110,
		FuelRecharge:   12,
		WpnCool:        2,
		EngCool:        5,
		CloakCost:      26,
		DetCost:        100,
	},
	ShipBattleship: {
		Name:           "Battleship",
		MaxSpeed:       8,
		MaxFuel:        14000,
		MaxShields:     130,
		MaxDamage:      130,
		MaxArmies:      6,
		TorpDamage:     40,
		TorpSpeed:      12,
		TorpFuse:       40,
		PhaserDamage:   105,
		PlasmaDamage:   130,
		PlasmaSpeed:    15,
		PlasmaFuse:     35,    // Original Netrek value
		TurnRate:       75000, // Original Netrek turn rate
		Mass:           2300,
		TractorStr:     3700,
		HasPlasma:      true,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1000,
		TorpFuelMult:   9,
		PhaserFuelMult: 10,
		PlasmaFuelMult: 30,
		TractorRange:   1.2,
		AccInt:         80,
		DecInt:         180,
		RepairRate:     125,
		FuelRecharge:   14,
		WpnCool:        2,
		EngCool:        5,
		CloakCost:      30,
		DetCost:        100,
	},
	ShipAssault: {
		Name:           "Assault",
		MaxSpeed:       8,
		MaxFuel:        6000,
		MaxShields:     80,
		MaxDamage:      200,
		MaxArmies:      20,
		TorpDamage:     30,
		TorpSpeed:      16,
		TorpFuse:       30, // Fixed: Was 20, should be 30
		PhaserDamage:   80,
		TurnRate:       120000, // Original Netrek turn rate
		Mass:           2300,
		TractorStr:     2500,
		HasPlasma:      false,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1200, // Assault has higher engine temp limit
		TorpFuelMult:   9,
		PhaserFuelMult: 7,
		TractorRange:   0.7,
		AccInt:         100,
		DecInt:         200,
		RepairRate:     120,
		FuelRecharge:   10,
		WpnCool:        2,
		EngCool:        7,
		CloakCost:      17,
		DetCost:        100,
	},
	ShipStarbase: {
		Name:           "Starbase",
		MaxSpeed:       2,
		MaxFuel:        60000,
		MaxShields:     500,
		MaxDamage:      600,
		MaxArmies:      25,
		TorpDamage:     30,
		TorpSpeed:      14,
		TorpFuse:       30,
		PhaserDamage:   120,
		PlasmaDamage:   150,
		PlasmaSpeed:    15,
		PlasmaFuse:     25,    // Original Netrek value
		TurnRate:       50000, // Original Netrek turn rate
		Mass:           5000,
		TractorStr:     8000,
		HasPlasma:      true,
		MaxWpnTemp:     1300, // Starbase has higher weapon temp limit
		MaxEngTemp:     1000,
		TorpFuelMult:   10,
		PhaserFuelMult: 8,
		PlasmaFuelMult: 25,
		TractorRange:   1.5,
		AccInt:         100,
		DecInt:         200,
		RepairRate:     140,
		FuelRecharge:   35,
		WpnCool:        3,
		EngCool:        5,
		CloakCost:      75,
		DetCost:        100,
	},
	ShipGalaxy: {
		Name:           "Galaxy",
		MaxSpeed:       9,
		MaxFuel:        12000,
		MaxShields:     140,
		MaxDamage:      120,
		MaxArmies:      5,
		TorpDamage:     40,
		TorpSpeed:      13,
		TorpFuse:       35, // Fixed: Was 30, should be 35
		PhaserDamage:   100,
		PlasmaDamage:   100,
		PlasmaSpeed:    15,
		PlasmaFuse:     33,     // Original Netrek value
		TurnRate:       192500, // Original Netrek turn rate
		Mass:           2050,
		TractorStr:     3000,
		HasPlasma:      true,
		MaxWpnTemp:     1000,
		MaxEngTemp:     1000,
		TorpFuelMult:   7,
		PhaserFuelMult: 7,
		PlasmaFuelMult: 30,
		TractorRange:   1.0,
		AccInt:         150,
		DecInt:         240,
		RepairRate:     112,
		FuelRecharge:   13,
		WpnCool:        2,
		EngCool:        5,
		CloakCost:      26,
		DetCost:        100,
	},
}

// Player represents a player in the game
type Player struct {
	mu sync.RWMutex

	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Team   int      `json:"team"`
	Ship   ShipType `json:"ship"`
	Status int      `json:"status"`

	// Position and movement
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Dir      float64 `json:"dir"` // Direction in radians
	Speed    float64 `json:"speed"`
	DesSpeed float64 `json:"desSpeed"`
	DesDir   float64 `json:"desDir"`
	SubDir   int     `json:"-"` // Fractional turn accumulator (not sent to client)
	AccFrac  int     `json:"-"` // Fractional acceleration accumulator (not sent to client)

	// Ship status
	Shields     int     `json:"shields"`
	Damage      int     `json:"damage"`
	Fuel        int     `json:"fuel"`
	Armies      int     `json:"armies"`
	Kills       float64 `json:"kills"`
	KillsStreak float64 `json:"killsStreak"` // Second kill counter that resets on death
	Deaths      int     `json:"deaths"`

	// Weapons
	WTemp     int `json:"wtemp"` // Weapon temperature
	ETemp     int `json:"etemp"` // Engine temperature
	NumTorps  int `json:"numTorps"`
	NumPlasma int `json:"numPlasma"`

	// Flags
	Shields_up     bool `json:"shields_up"`
	Cloaked        bool `json:"cloaked"`
	Repairing      bool `json:"repairing"`     // In repair mode
	RepairRequest  bool `json:"repairRequest"` // Slowing down to repair
	RepairCounter  int  `json:"-"`             // Counter for repair timing (not sent to client)
	Bombing        bool `json:"bombing"`
	Beaming        bool `json:"beaming"`
	BeamingUp      bool `json:"beamingUp"`      // True if beaming up, false if beaming down
	EngineOverheat bool `json:"engineOverheat"` // Engine temp exceeded max (PFENG in original)
	Tractoring     int  `json:"tractoring"`     // Player ID being tractored, -1 if none
	Pressoring     int  `json:"pressoring"`     // Player ID being pressored, -1 if none

	// Lock-on
	LockType   string `json:"lockType"`   // "none", "player", or "planet"
	LockTarget int    `json:"lockTarget"` // ID of locked target

	// Orbiting
	Orbiting int `json:"orbiting"` // Planet ID, -1 if not orbiting

	// Death tracking
	ExplodeTimer int `json:"explodeTimer"` // Frames left in explosion animation
	KilledBy     int `json:"killedBy"`     // Player ID who killed us
	WhyDead      int `json:"whyDead"`      // Reason for death (KillTorp, KillPhaser, etc)

	// Engine overheat tracking
	OverheatTimer int `json:"-"` // Frames left in overheat state (not sent to client)

	// Alert status
	AlertLevel string `json:"alertLevel"` // "green", "yellow", or "red"

	// Network
	Connected  bool      `json:"connected"`
	LastUpdate time.Time `json:"-"`

	// Bot fields
	IsBot       bool    `json:"isBot"`
	BotTarget   int     `json:"-"` // Current target player ID
	BotGoalX    float64 `json:"-"` // Navigation goal
	BotGoalY    float64 `json:"-"`
	BotCooldown int     `json:"-"` // Frames until next action

	// Refit system - ship type to use on next respawn (-1 means no pending refit)
	NextShipType int `json:"-"` // Ship type to use on next respawn
}

// Torpedo represents a torpedo in space
type Torpedo struct {
	ID     int     `json:"id"`
	Owner  int     `json:"owner"` // Player ID
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Dir    float64 `json:"dir"`
	Speed  float64 `json:"speed"`
	Damage int     `json:"damage"`
	Fuse   int     `json:"fuse"`   // Ticks until explosion
	Status int     `json:"status"` // Free, Move, Explode, Det
	Team   int     `json:"team"`
}

// Plasma represents a plasma torpedo (reuses Torpedo struct)
type Plasma struct {
	ID     int     `json:"id"`
	Owner  int     `json:"owner"` // Player ID
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Dir    float64 `json:"dir"`
	Speed  float64 `json:"speed"`
	Damage int     `json:"damage"`
	Fuse   int     `json:"fuse"`   // Ticks until explosion
	Status int     `json:"status"` // Free, Move, Explode, Det
	Team   int     `json:"team"`
}

// Planet represents a planet
type Planet struct {
	mu sync.RWMutex

	ID     int     `json:"id"`
	Name   string  `json:"name"`
	Label  string  `json:"label"` // 3-letter label for display
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Owner  int     `json:"owner"` // Team that owns it
	Armies int     `json:"armies"`
	Info   int     `json:"info"`  // Information mask (who has scouted)
	Flags  int     `json:"flags"` // Planet flags (repair, fuel, agri, etc)
}

// PlanetFlags
const (
	PlanetRepair = 1 << 4
	PlanetFuel   = 1 << 5
	PlanetAgri   = 1 << 6
	PlanetHome   = 1 << 8
	PlanetCore   = 1 << 11
)

// TournamentPlayerStats tracks player performance in tournament mode
type TournamentPlayerStats struct {
	Kills        int
	Deaths       int
	PlanetsTaken int
	PlanetsLost  int
	TorpsFired   int
	PhasersFired int
	DamageDealt  int
	DamageTaken  int
}

// GameState holds the entire game state
type GameState struct {
	Mu sync.RWMutex // Made public for access from server package

	Players [MaxPlayers]*Player
	Planets [MaxPlanets]*Planet
	Torps   []*Torpedo
	Plasmas []*Plasma

	Frame     int64
	TickCount int   // Tick counter for periodic events
	T_mode    bool  // Tournament mode
	T_start   int64 // Tournament start time (frame)
	T_remain  int   // Tournament time remaining (seconds)
	GameOver  bool
	Winner    int    // Winning team (if GameOver)
	WinType   string // "genocide" or "conquest"

	// Team statistics
	TeamPlanets [4]int // Planet count per team
	TeamPlayers [4]int // Active player count per team

	// Tournament statistics
	TournamentStats map[int]*TournamentPlayerStats // Player ID -> stats
}

// NewGameState creates a new game state with INL mode enabled by default
func NewGameState() *GameState {
	return NewGameStateWithMode(true)
}

// NewGameStateWithMode creates a new game state with optional INL mode
func NewGameStateWithMode(inlMode bool) *GameState {
	gs := &GameState{
		Torps:           make([]*Torpedo, 0),
		Plasmas:         make([]*Plasma, 0),
		TournamentStats: make(map[int]*TournamentPlayerStats),
	}

	// Initialize players
	for i := 0; i < MaxPlayers; i++ {
		gs.Players[i] = &Player{
			ID:           i,
			Status:       StatusFree,
			Tractoring:   -1,
			Pressoring:   -1,
			Orbiting:     -1,
			LockType:     "none",
			LockTarget:   -1,
			NextShipType: -1, // No pending refit by default for fresh slots
		}
	}

	// Initialize planets
	InitPlanets(gs)

	// Apply INL mode flags if requested
	if inlMode {
		InitINLPlanetFlags(gs)
	}

	return gs
}

// Distance calculates distance between two points
func Distance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Sqrt(dx*dx + dy*dy)
}

// NormalizeAngle keeps angle between 0 and 2*PI
func NormalizeAngle(angle float64) float64 {
	for angle < 0 {
		angle += 2 * math.Pi
	}
	for angle >= 2*math.Pi {
		angle -= 2 * math.Pi
	}
	return angle
}

// Min returns the minimum of two integers
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the maximum of two integers
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// GetShipExplosionDamage returns the explosion damage for a ship type
// Based on original Netrek: SB=200, SC=75, all others=100
func GetShipExplosionDamage(shipType ShipType) int {
	switch shipType {
	case ShipStarbase:
		return 200
	case ShipScout:
		return 75
	default:
		return 100
	}
}
