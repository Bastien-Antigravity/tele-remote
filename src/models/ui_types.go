package models

/*
ESSENTIAL PROCESS:
Defines the in-memory UI menu tree models, conversation scoping keys, and pending
interactive state structures for Tele-Remote.

DATA FLOW:
1. Microservices register JSON menu trees that deserialize into ComponentMenu, CommandMenu, CommandRow, and CommandButton.
2. User interactions are scoped by ConversationKey{ChatID, UserID}.
3. Multi-step input prompts and emergency command confirmations are tracked with expiration timestamps.

KEY PARAMETERS:
- ConversationKey: Composite key isolating per-user and per-chat interactive state.
- PendingInput: Stores originating ClientID, target CommandButton, menu Path, and ExpiresAt.
- PendingConfirmation: Tracks 2-step confirmation state and 60s expiration for critical commands.
*/

import (
	"time"
)

// -----------------------------------------------------------------------------

// ComponentMenu holds the structured tree for a single registered component
type ComponentMenu struct {
	Name         string       `json:"name"`
	ClientID     string       `json:"client_id"`
	Priority     int          `json:"priority"`
	Revision     uint64       `json:"revision"`
	RegisteredAt time.Time    `json:"registered_at"`
	Root         *CommandMenu `json:"root"`
}

// -----------------------------------------------------------------------------

// CommandButton represents a single button in a row
type CommandButton struct {
	Label       string       `json:"label"`
	Order       int          `json:"order,omitempty"`
	NextMenu    *CommandMenu `json:"next_menu"`    // for sub-menus
	CommandType int32        `json:"command_type"` // command identifier dispatched to publisher
	Payload     string       `json:"payload"`      // command argument payload
	InputPrompt string       `json:"input_prompt"` // if set, ask user for text input
}

// -----------------------------------------------------------------------------

// CommandRow represents a single row of buttons in a menu
type CommandRow struct {
	Order   int             `json:"order,omitempty"`
	Buttons []CommandButton `json:"buttons"`
}

// -----------------------------------------------------------------------------

// CommandMenu represents a structured Telegram keyboard mapping
type CommandMenu struct {
	Title   string       `json:"title"` // displayed as header
	Rows    []CommandRow `json:"rows"`
	Caption string       `json:"caption"`
}

// -----------------------------------------------------------------------------

// ConversationKey scopes user navigation, pending inputs, and confirmations by chat and sender
type ConversationKey struct {
	ChatID int64
	UserID int64
}

// -----------------------------------------------------------------------------

// PendingInput tracks an in-flight text prompt bound to the originating component
type PendingInput struct {
	ClientID     string
	MenuRevision uint64
	Button       CommandButton
	Path         []string
	ExpiresAt    time.Time
}

// -----------------------------------------------------------------------------

// PendingConfirmation tracks a 2-step confirmation gate for critical fleet commands
type PendingConfirmation struct {
	CommandType int32
	ExpiresAt   time.Time
}

// -----------------------------------------------------------------------------

// Standard Command Types and Emergency Reply Keyboard Labels
const (
	CmdPowerOff int32 = 1

	BtnPowerOff        = "🆘 power off !"
	BtnConfirmPowerOff = "✅ CONFIRM POWER OFF"
	BtnCancel          = "❌ Cancel"
)
