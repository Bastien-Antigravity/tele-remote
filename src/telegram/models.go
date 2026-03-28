package telegram

import tb "gopkg.in/telebot.v3"

// -----------------------------------------------------------------------------
// MenuItem represents a single executable button or sub-folder
type MenuItem struct {
	Label    string     `json:"label"`
	Command  string     `json:"command_id"` // empty if it's a folder
	SubMenus []MenuItem `json:"sub_menus"`
}

// -----------------------------------------------------------------------------
// ComponentMenu represents the full dynamic menu structure sent by a component
type ComponentMenu struct {
	Name string     `json:"name"`
	Menu []MenuItem `json:"menu"`
}

// -----------------------------------------------------------------------------
// CallbackAction stores contextual state for an inline callback button
type CallbackAction struct {
	Type     string // "node_main", "node_sub", "execute"
	ClientID string
	Path     []int  // path of indices into SubMenus
	Command  string // if execute
}

// -----------------------------------------------------------------------------
// CommandMenu represents a structured command menu concept for future use
type CommandMenu struct {
	Name    string
	Caption string
	Markup  *tb.ReplyMarkup
}
