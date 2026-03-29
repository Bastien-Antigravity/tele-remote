package telegram

import tb "gopkg.in/telebot.v3"

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
