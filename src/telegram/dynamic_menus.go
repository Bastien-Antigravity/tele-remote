package telegram

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

// OnComponentConnected is triggered when a client connects via gRPC or NATS
func (bot *Bot) OnComponentConnected(clientID, componentName, menuJSON string, pub interfaces.Publisher) {
	if menuJSON == "" {
		return
	}

	bot.mu.Lock()
	bot.publishers[clientID] = pub
	bot.mu.Unlock()

	var rawItems []map[string]interface{}
	if err := json.Unmarshal([]byte(menuJSON), &rawItems); err != nil {
		bot.log.Error("Failed to parse component menu JSON", "err", err, "json", menuJSON)
		return
	}

	root := &models.CommandMenu{
		Title: fmt.Sprintf("📦 %s", componentName),
		Rows:  []models.CommandRow{},
	}

	for _, item := range rawItems {
		row := bot.parseMenuRow(item, clientID)
		if len(row.Buttons) > 0 {
			root.Rows = append(root.Rows, row)
		}
	}

	bot.mu.Lock()
	bot.dynamicMenus[clientID] = &models.ComponentMenu{
		Name:     componentName,
		ClientID: clientID,
		Root:     root,
	}
	bot.mu.Unlock()

	// Persistence: Save state after each successful registration
	if err := bot.SaveState(); err != nil {
		bot.log.Warning("Failed to save component state", "err", err)
	}

	bot.log.Info("Dynamic menu registered", "client", clientID, "rows", len(root.Rows))
}

// -----------------------------------------------------------------------------

// OnComponentDisconnected cleans up memory and publishers
func (bot *Bot) OnComponentDisconnected(clientID string) {
	bot.mu.Lock()
	defer bot.mu.Unlock()

	delete(bot.dynamicMenus, clientID)
	delete(bot.publishers, clientID)
	bot.log.Info("Component disconnected", "client", clientID)
}

// -----------------------------------------------------------------------------

func (bot *Bot) parseMenuRow(data map[string]interface{}, clientID string) models.CommandRow {
	row := models.CommandRow{Buttons: []models.CommandButton{}}

	// If it's a list (row), iterate
	if btns, ok := data["buttons"].([]interface{}); ok {
		for _, b := range btns {
			if bMap, ok := b.(map[string]interface{}); ok {
				row.Buttons = append(row.Buttons, bot.parseButton(bMap, clientID))
			}
		}
	} else {
		// Single button row
		row.Buttons = append(row.Buttons, bot.parseButton(data, clientID))
	}

	return row
}

// -----------------------------------------------------------------------------

func (bot *Bot) parseButton(data map[string]interface{}, clientID string) models.CommandButton {
	label := data["label"].(string)

	// If it has sub-buttons, it's a sub-menu
	if sub, ok := data["menu"].([]interface{}); ok {
		subMenu := &models.CommandMenu{Title: label, Rows: []models.CommandRow{}}
		for _, s := range sub {
			if sMap, ok := s.(map[string]interface{}); ok {
				subMenu.Rows = append(subMenu.Rows, bot.parseMenuRow(sMap, clientID))
			}
		}
		return models.CommandButton{Label: label, NextMenu: subMenu}
	}

	// Otherwise, it's a command
	cmdType := int32(0)
	if val, ok := data["cmd_type"].(float64); ok {
		cmdType = int32(val)
	}
	payload := ""
	if p, ok := data["payload"].(string); ok {
		payload = p
	}

	uniqueID := bot.registerAction(func(ctx tb.Context) error {
		bot.mu.RLock()
		pub, ok := bot.publishers[clientID]
		bot.mu.RUnlock()

		if !ok {
			return ctx.Send("❌ Component disconnected.")
		}

		bot.log.Info("Executing component command", "client", clientID, "type", cmdType)
		if err := pub.PublishCommand(context.Background(), cmdType, payload); err != nil {
			return ctx.Send(fmt.Sprintf("⚠️ Failed to send command: %v", err))
		}
		return ctx.Send(fmt.Sprintf("✅ Sent: %s", label))
	})

	return models.CommandButton{Label: label, CallbackData: uniqueID}
}

// -----------------------------------------------------------------------------

func (bot *Bot) registerAction(fn models.CallbackAction) string {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.cbCounter++
	id := fmt.Sprintf("dyn_%d", bot.cbCounter)
	bot.actionMap[id] = fn
	return id
}

// -----------------------------------------------------------------------------

// showNodesMenu displays the list of currently connected components
func (bot *Bot) showNodesMenu(c tb.Context) error {
	bot.mu.RLock()
	defer bot.mu.RUnlock()

	if len(bot.dynamicMenus) == 0 {
		return c.Send("📭 No components currently connected.")
	}

	menu := &tb.ReplyMarkup{}
	var rows []tb.Row
	for _, m := range bot.dynamicMenus {
		// Create a unique action for showing this component's root menu
		btnID := bot.registerAction(func(ctx tb.Context) error {
			return bot.renderMenu(ctx, m.Root)
		})
		rows = append(rows, menu.Row(menu.Data(m.Name, btnID)))
	}

	menu.Inline(rows...)
	return c.Send("🔌 Connected Nodes:", menu)
}

// -----------------------------------------------------------------------------

// handleDynamicCallback routes any non-static inline buttons to the actionMap
func (bot *Bot) handleDynamicCallback(c tb.Context) error {
	data := c.Callback().Data
	bot.mu.RLock()
	fn, ok := bot.actionMap[data]
	bot.mu.RUnlock()

	if !ok {
		// Silent ignore or error
		return c.Respond()
	}

	return fn(c)
}

// -----------------------------------------------------------------------------

// renderMenu recursively builds and displays a CommandMenu
func (bot *Bot) renderMenu(c tb.Context, m *models.CommandMenu) error {
	menuMarkup := &tb.ReplyMarkup{}
	var rows []tb.Row

	for _, row := range m.Rows {
		var btns []tb.Btn
		for _, b := range row.Buttons {
			var btn tb.Btn
			if b.NextMenu != nil {
				// Submenu button
				btnID := bot.registerAction(func(ctx tb.Context) error {
					return bot.renderMenu(ctx, b.NextMenu)
				})
				btn = menuMarkup.Data(b.Label, btnID)
			} else {
				// Command button
				btn = menuMarkup.Data(b.Label, b.CallbackData)
			}
			btns = append(btns, btn)
		}
		rows = append(rows, menuMarkup.Row(btns...))
	}

	menuMarkup.Inline(rows...)
	return c.Edit(m.Title, menuMarkup)
}
