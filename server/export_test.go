package server

import "github.com/lab1702/netrek-web/game"

// Test helpers to expose private methods for testing purposes
// This file should only be used for testing and not in production

// SetGameState allows tests to set the game state directly
func (s *Server) SetGameState(gs *game.GameState) {
	s.gameState = gs
}

// GetGameState allows tests to get the current game state
func (s *Server) GetGameState() *game.GameState {
	return s.gameState
}

// FireBotTorpedo exposes the private fireBotTorpedo method for testing
func (s *Server) FireBotTorpedo(shooter, target *game.Player) {
	s.fireBotTorpedo(shooter, target)
}

// CalculateEnhancedInterceptCourse exposes the private calculateEnhancedInterceptCourse method for testing
func (s *Server) CalculateEnhancedInterceptCourse(shooter, target *game.Player) float64 {
	return s.calculateEnhancedInterceptCourse(shooter, target)
}

// SelectBestCombatTarget exposes the private selectBestCombatTarget method for testing
func (s *Server) SelectBestCombatTarget(p *game.Player) *game.Player {
	return s.selectBestCombatTarget(p)
}

// CalculateTargetScore exposes the private calculateTargetScore method for testing
func (s *Server) CalculateTargetScore(p, target *game.Player, dist float64) float64 {
	return s.calculateTargetScore(p, target, dist)
}

// SelectCombatManeuver exposes the private selectCombatManeuver for testing
func (s *Server) SelectCombatManeuver(p, target *game.Player, dist float64, interceptDir float64) CombatManeuver {
	return s.selectCombatManeuver(p, target, dist, interceptDir)
}

// IsTorpedoThreatening exposes the private isTorpedoThreatening for testing
func (s *Server) IsTorpedoThreatening(p *game.Player, torp *game.Torpedo) bool {
	return s.isTorpedoThreatening(p, torp)
}

// CoordinateTeamAttack exposes the private coordinateTeamAttack for testing
func (s *Server) CoordinateTeamAttack(p *game.Player, target *game.Player) int {
	return s.coordinateTeamAttack(p, target)
}

// DetonatePassingTorpedoes exposes the private detonatePassingTorpedoes for testing
func (s *Server) DetonatePassingTorpedoes(p *game.Player) {
	s.detonatePassingTorpedoes(p)
}

// SelectBotBehavior exposes the private selectBotBehavior for testing
func (s *Server) SelectBotBehavior(p *game.Player) string {
	return s.selectBotBehavior(p)
}

// BroadcastTargetToAllies exposes the private broadcastTargetToAllies for testing
func (s *Server) BroadcastTargetToAllies(p *game.Player, target *game.Player, targetValue float64) {
	s.broadcastTargetToAllies(p, target, targetValue)
}

// GetPendingSuggestions returns the current pending suggestions for testing
func (s *Server) GetPendingSuggestions() []targetSuggestion {
	return s.pendingSuggestions
}

// IsPlayerIsolated exposes the private isPlayerIsolated for testing
func (s *Server) IsPlayerIsolated(playerID int) bool {
	return s.isPlayerIsolated(playerID)
}
