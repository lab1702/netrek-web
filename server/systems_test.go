package server

import (
	"math"
	"testing"

	"github.com/lab1702/netrek-web/game"
)

func TestFuelShortageSlowsTravelAndRefuelingRestoresSpeed(t *testing.T) {
	for _, shields := range []bool{false, true} {
		s, _, p := newTestClientAndPlayer(game.TeamFed, game.ShipCruiser)
		p.X, p.Y, p.Dir, p.DesDir = 50000, 50000, 0, 0
		p.Speed, p.DesSpeed = 9, 9
		p.Fuel, p.Shields_up = 0, shields
		for tick := 0; tick < 100; tick++ {
			s.updatePlayerSystems(p, p.ID)
			s.updatePlayerPhysics(p, p.ID)
		}
		if distance := p.X - 50000; distance >= 15000 {
			t.Fatalf("shields=%v: fuel-starved cruiser traveled %v units; full-speed travel is 18000", shields, distance)
		}
		if p.DesSpeed != 9 {
			t.Fatal("temporary fuel shortage changed the pilot's speed command")
		}
		p.Fuel = game.ShipData[p.Ship].MaxFuel
		for tick := 0; tick < 100; tick++ {
			s.updatePlayerSystems(p, p.ID)
			s.updatePlayerPhysics(p, p.ID)
		}
		if p.FuelStarved || math.Abs(p.Speed-9) > 0.001 {
			t.Fatalf("refueled cruiser failed to recover requested speed: %v", p.Speed)
		}
	}
}

// TestRepairStartMessageIsPrivate verifies that the "is repairing damage"
// notice sent when a ship begins repairing is addressed only to the repairing
// player (a "to" field), matching the repair-completion message. Without it the
// message is broadcast to every connected client, spamming and leaking each
// ship's repair status.
func TestRepairStartMessageIsPrivate(t *testing.T) {
	gs := game.NewGameState()
	s := &Server{
		gameState: gs,
		broadcast: make(chan ServerMessage, 10),
	}

	const idx = 3
	p := gs.Players[idx]
	p.Status = game.StatusAlive
	p.Ship = game.ShipDestroyer
	p.RepairRequest = true
	p.Speed = 0
	p.Orbiting = -1

	s.updatePlayerSystems(p, idx)

	select {
	case msg := <-s.broadcast:
		data, ok := msg.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected message data type %T", msg.Data)
		}
		to, ok := data["to"]
		if !ok {
			t.Fatal("repair-start message has no 'to' field; it is broadcast to all clients")
		}
		if to != idx {
			t.Errorf("repair-start message addressed to %v, want %d", to, idx)
		}
	default:
		t.Fatal("expected a repair-start broadcast message")
	}
}
