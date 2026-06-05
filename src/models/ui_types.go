package models

import tb "gopkg.in/telebot.v3"

// -----------------------------------------------------------------------------

// ComponentMenu holds the structured tree for a single registered component
type ComponentMenu struct {
	Name     string `json:"name"`
	ClientID string `json:"client_id"`
	Root     *CommandMenu `json:"root"`
}

// -----------------------------------------------------------------------------

// CallbackAction defines the function signature for an inline button trigger
type CallbackAction func(ctx tb.Context) error

// -----------------------------------------------------------------------------

// CommandButton represents a single button in a row
type CommandButton struct {
	Label        string       `json:"label"`
	CallbackData string       `json:"callback_data"` // id in the actionMap
	NextMenu     *CommandMenu `json:"next_menu"`     // for sub-menus
	CommandType  int32        `json:"command_type"`  // persisted for logic restore
	Payload      string       `json:"payload"`       // persisted for logic restore
}

// -----------------------------------------------------------------------------

// CommandRow represents a single row of buttons in a menu
type CommandRow struct {
	Buttons []CommandButton `json:"buttons"`
}

// -----------------------------------------------------------------------------

// CommandMenu represents a structured Telegram keyboard mapping
type CommandMenu struct {
	Title   string `json:"title"` // displayed as header
	Rows    []CommandRow `json:"rows"`
	Caption string `json:"caption"`
}

// Standard Command Types
const (
	CmdPowerOff int32 = 1
	CmdStop     int32 = 2
)
