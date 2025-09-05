# Netrek Bot Torpedo Aiming Analysis

## Overview
This document provides a comprehensive audit of the three different torpedo prediction systems currently used in the Netrek bot AI.

## Current Baseline Performance
From testing 100 shots with straight-line targets at speed 8.0:
- **Hit Rate:** 76%
- **Average Miss Distance:** 433.7 units
- **Median Miss Distance:** 400.0 units
- **Miss Range:** 11.2 - 1085.0 units

## Prediction Systems Analysis

### 1. `fireBotTorpedo` - Simple Linear Prediction
**Location:** `server/bot_weapons.go:18-58`

**Approach:** Basic linear prediction using time-to-target calculation

**Algorithm:**
```go
timeToTarget := dist / float64(shipStats.TorpSpeed*20)
predictX := target.X + target.Speed*math.Cos(target.Dir)*timeToTarget
predictY := target.Y + target.Speed*math.Sin(target.Dir)*timeToTarget
fireDir := math.Atan2(predictY-p.Y, predictX-p.X) + randomJitterRad()
```

**Characteristics:**
- **Units:** Torpedo speed uses `TorpSpeed * 20` (units/tick)
- **Target velocity:** Uses `target.Speed * math.Cos/Sin(target.Dir)` (world units/tick)
- **Assumptions:** Constant target velocity, no acceleration
- **Jitter:** Applied after calculation
- **Edge cases:** None handled explicitly

**Issues:**
- Assumes torpedo and target have same time units
- No handling of impossible intercept scenarios
- Linear prediction may be inaccurate for maneuvering targets

### 2. `fireBotTorpedoWithLead` - Quadratic Intercept Solver
**Location:** `server/bot_weapons.go:158-251`

**Approach:** Advanced quadratic intercept calculation based on borgmove.c

**Algorithm:**
```go
// Relative position and velocity setup
vxa := target.X - p.X / l  // Normalized
vya := target.Y - p.Y / l
vxs := target.Speed * math.Cos(target.Dir) * 20  // units/tick
vys := target.Speed * math.Sin(target.Dir) * 20

// Quadratic solver
a := vs*vs - torpSpeed*torpSpeed
b := 2 * l * dp
c := l * l
// Solve for intercept time t
```

**Characteristics:**
- **Units:** Consistent `* 20` conversion for both torpedo and target
- **Solver:** Full quadratic equation with discriminant check
- **Fallback:** Direct shot if no solution (t ≤ 0)
- **Edge cases:** Handles linear case when a=0
- **Jitter:** Applied after calculation

**Issues:**
- Complex code with potential numerical instability
- No validation of intercept point reasonableness
- Unit conversion in intercept calculation (vxs*t/20) may cause precision loss

### 3. `calculateEnhancedInterceptCourse` - Acceleration Prediction
**Location:** `server/bot_navigation.go:56-100`

**Approach:** Enhanced prediction including acceleration and obstacle avoidance

**Algorithm:**
```go
// Track acceleration
targetAccel := 0.0
if target.Speed != target.DesSpeed {
    if target.DesSpeed > target.Speed {
        targetAccel = 1.0  // Accelerating
    } else {
        targetAccel = -1.0  // Decelerating
    }
}

// Predict with acceleration
timeToIntercept := dist / torpSpeed
futureSpeed := target.Speed + targetAccel*timeToIntercept*0.5
predictX := target.X + futureSpeed*math.Cos(target.Dir)*timeToIntercept*20
```

**Characteristics:**
- **Units:** Torpedo speed uses `TorpSpeed * 20`
- **Acceleration:** Simple +1/-1 acceleration model
- **Obstacle prediction:** Adds turn prediction near walls/planets
- **Fallback:** Direct shot for distant/cloaked targets
- **Jitter:** Applied in calling functions

**Issues:**
- Oversimplified acceleration model (constant ±1)
- No actual intercept solving - still linear with acceleration estimate
- Unit mixing: timeToIntercept vs world coordinates

## Code Analysis Comparison Table

| Feature | fireBotTorpedo | fireBotTorpedoWithLead | calculateEnhancedInterceptCourse |
|---------|----------------|------------------------|--------------------------------|
| **Complexity** | Simple | Complex | Medium |
| **Prediction Type** | Linear | Quadratic Intercept | Linear + Acceleration |
| **Torpedo Speed** | `TorpSpeed * 20` | `TorpSpeed * 20` | `TorpSpeed * 20` |
| **Target Velocity Units** | `Speed * cos/sin` | `Speed * cos/sin * 20` | `Speed * cos/sin` |
| **Edge Case Handling** | None | Linear fallback, no-solution fallback | Distance/cloak fallback |
| **Jitter Injection** | After calculation | After calculation | In caller |
| **Unit Consistency** | ⚠️ Mixed | ✅ Consistent | ⚠️ Mixed |
| **Intercept Solving** | Approximate | Exact (quadratic) | Approximate |
| **Acceleration Model** | None | None | Simple ±1 |
| **Obstacle Avoidance** | None | None | Basic turn prediction |

## Unit Consistency Issues Found

### 1. Time Units Inconsistency
- **Issue:** Some functions use world units/second vs units/tick
- **Impact:** Incorrect time-to-intercept calculations
- **Examples:**
  - `fireBotTorpedo`: Mixed time units between torpedo and target
  - `calculateEnhancedInterceptCourse`: `*20` applied inconsistently

### 2. Velocity Conversion Inconsistency  
- **Issue:** Target velocity calculated differently across functions
- **Impact:** Different prediction accuracy
- **Examples:**
  - Method 1: `target.Speed * cos/sin` (units/tick)
  - Method 2: `target.Speed * cos/sin * 20` (units/tick)

### 3. Distance vs Time Calculations
- **Issue:** Intercept point calculations mix distance and time units
- **Examples:**
  - Line 223: `interceptX := target.X + vxs*t/20` (why divide by 20?)

## Range Logic Consistency

All weapon functions use consistent torpedo range calculations:
- `EffectiveTorpRangeDefault(shipStats)` in `bot_combat.go`
- Formula: `(TorpSpeed * 20) * TorpFuse * DefaultTorpSafety`
- Safety margin: `DefaultTorpSafety = 0.85`

**Verified consistent across:**
- `engageCombat` (line 106)
- `planetDefenseWeaponLogic` (line 333)
- `starbaseDefenseWeaponLogic` (line 367)

## Call Graph Analysis

```
engageCombat (bot_combat.go:9)
├── fireTorpedoSpread (line 110)
│   └── calculateEnhancedInterceptCourse (bot_navigation.go:56)
├── fireEnhancedTorpedo (line 113)
│   └── calculateEnhancedInterceptCourse (bot_navigation.go:56)
└── fireBotTorpedoWithLead (line 126)
    └── [inline quadratic math]

planetDefenseWeaponLogic (bot_weapons.go:326)
└── fireBotTorpedoWithLead (line 335)

starbaseDefensiveCombat (bots.go:742)
└── fireBotTorpedoWithLead (line 753)
```

## Key Findings

### Critical Issues
1. **Unit Inconsistency:** Mixed time/distance units causing prediction errors
2. **Multiple Algorithms:** Three different approaches with no clear best choice
3. **No Validation:** No checks for reasonable intercept solutions
4. **Numerical Instability:** Quadratic solver may have edge cases

### Performance Issues  
1. **Redundant Calculations:** Multiple prediction methods doing similar work
2. **Complex Conditionals:** Enhanced method has many branches
3. **Memory Allocation:** String operations in jitter functions

### Accuracy Issues
1. **Jitter Timing:** Random error applied after calculation may mask real accuracy
2. **Acceleration Model:** Oversimplified ±1 model doesn't match ship physics  
3. **No Intercept Validation:** Solutions not checked for reasonableness

## Recommendations

1. **Unify Prediction Logic:** Replace all three methods with single, proven algorithm
2. **Fix Unit Consistency:** Standardize on units/tick throughout
3. **Add Solution Validation:** Check intercept solutions for reasonableness
4. **Improve Edge Case Handling:** Handle impossible intercepts gracefully
5. **Optimize Performance:** Reduce redundant calculations
6. **Better Testing:** Add comprehensive unit tests for edge cases

## Next Steps

Based on this audit, the next phase should focus on implementing a unified, mathematically sound intercept solver that addresses the unit consistency and accuracy issues identified above.
