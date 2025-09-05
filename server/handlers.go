package server

// This file previously contained all handler functions but has been refactored
// into separate, more focused files for better maintainability:
//
//   handler_utils.go          - Shared utilities, constants, and data structures
//   game_state_handlers.go    - Login and quit handlers
//   movement_handlers.go      - Movement, orbit, and lock handlers
//   combat_handlers.go        - Weapons and combat systems
//   ship_management_handlers.go - Repair, beam, and bomb handlers
//   communication_handlers.go - All messaging systems
//   bot_handlers.go          - Bot management commands
//
// All handler functions are now organized by their functional area,
// making the codebase easier to navigate and maintain.
