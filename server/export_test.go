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

// FireBotTorpedoWithLead exposes the private fireBotTorpedoWithLead method for testing
func (s *Server) FireBotTorpedoWithLead(shooter, target *game.Player) {
	s.fireBotTorpedoWithLead(shooter, target)
}

// FireBotTorpedo exposes the private fireBotTorpedo method for testing
func (s *Server) FireBotTorpedo(shooter, target *game.Player) {
	s.fireBotTorpedo(shooter, target)
}

// FireEnhancedTorpedo exposes the private fireEnhancedTorpedo method for testing
func (s *Server) FireEnhancedTorpedo(shooter, target *game.Player) {
	s.fireEnhancedTorpedo(shooter, target)
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
