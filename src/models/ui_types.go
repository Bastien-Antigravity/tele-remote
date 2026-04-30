package models

import tb "gopkg.in/telebot.v3"

// -----------------------------------------------------------------------------

// ComponentMenu holds the structured tree for a single registered component
type ComponentMenu struct {
	Name     string
	ClientID string
	Root     *CommandMenu
}

// -----------------------------------------------------------------------------

// CallbackAction defines the function signature for an inline button trigger
type CallbackAction func(ctx tb.Context) error

// -----------------------------------------------------------------------------

// CommandButton represents a single button in a row
type CommandButton struct {
	Label        string
	CallbackData string       // id in the actionMap
	NextMenu     *CommandMenu // for sub-menus
}

// -----------------------------------------------------------------------------

// CommandRow represents a single row of buttons in a menu
type CommandRow struct {
	Buttons []CommandButton
}

// -----------------------------------------------------------------------------

// CommandMenu represents a structured Telegram keyboard mapping
type CommandMenu struct {
	Title   string // displayed as header
	Rows    []CommandRow
	Caption string
	Markup  *tb.ReplyMarkup
}

// Standard Command Types
const (
	CmdPowerOff int32 = 1
	CmdStop     int32 = 2
)
