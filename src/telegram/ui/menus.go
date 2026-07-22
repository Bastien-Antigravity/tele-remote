package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"

	tb "gopkg.in/telebot.v3"
)

// OnComponentConnected is triggered when a client connects via gRPC or NATS
func OnComponentConnected(bot *core.Bot, clientID, componentName, menuJSON string, pub interfaces.IPublisher) {
	bot.Log.Info("Component connection handshake starting", "id", clientID, "name", componentName)
	if menuJSON == "" {
		bot.Log.Warning("Component connected with empty menu", "id", clientID)
		return
	}

	bot.Mu.Lock()
	// Clean up old sessions with the same name to prevent duplicates
	for id, m := range bot.DynamicMenus {
		if m.Name == componentName {
			delete(bot.DynamicMenus, id)
			delete(bot.Publishers, id)
		}
	}
	bot.Publishers[clientID] = pub
	bot.Mu.Unlock()

	var rawItems []map[string]interface{}
	if err := json.Unmarshal([]byte(menuJSON), &rawItems); err != nil {
		bot.Log.Error("Failed to parse component menu JSON", "err", err, "json", menuJSON)
		return
	}

	root := &models.CommandMenu{
		Title: fmt.Sprintf("📦 %s", componentName),
		Rows:  []models.CommandRow{},
	}

	for _, item := range rawItems {
		row := parseMenuRow(bot, item, clientID)
		if len(row.Buttons) > 0 {
			root.Rows = append(root.Rows, row)
		}
	}

	bot.Mu.Lock()
	bot.DynamicMenus[clientID] = &models.ComponentMenu{
		Name:     componentName,
		ClientID: clientID,
		Root:     root,
	}
	bot.Mu.Unlock()

	bot.Log.Info("Dynamic menu registered", "client", clientID, "rows", len(root.Rows))
}

func parseMenuRow(bot *core.Bot, data map[string]interface{}, clientID string) models.CommandRow {
	row := models.CommandRow{Buttons: []models.CommandButton{}}
	if btns, ok := data["buttons"].([]interface{}); ok {
		for _, b := range btns {
			if bMap, ok := b.(map[string]interface{}); ok {
				row.Buttons = append(row.Buttons, parseButton(bot, bMap, clientID))
			}
		}
	} else {
		row.Buttons = append(row.Buttons, parseButton(bot, data, clientID))
	}
	return row
}

func parseButton(bot *core.Bot, data map[string]interface{}, clientID string) models.CommandButton {
	label, _ := data["label"].(string)

	if sub, ok := data["menu"].([]interface{}); ok {
		subMenu := &models.CommandMenu{Title: label, Rows: []models.CommandRow{}}
		for _, s := range sub {
			if sMap, ok := s.(map[string]interface{}); ok {
				subMenu.Rows = append(subMenu.Rows, parseMenuRow(bot, sMap, clientID))
			}
		}
		return models.CommandButton{Label: label, NextMenu: subMenu}
	}

	cmdType := int32(0)
	if val, ok := data["cmd_type"].(float64); ok {
		cmdType = int32(val)
	}
	payload := ""
	if p, ok := data["payload"].(string); ok {
		payload = p
	}
	inputPrompt := ""
	if ip, ok := data["input_prompt"].(string); ok {
		inputPrompt = ip
	}

	uniqueID := registerAction(bot, createCommandAction(bot, clientID, cmdType, payload, label))

	return models.CommandButton{
		Label:        label,
		CallbackData: uniqueID,
		CommandType:  cmdType,
		Payload:      payload,
		InputPrompt:  inputPrompt,
	}
}

func createCommandAction(bot *core.Bot, clientID string, cmdType int32, payload, label string) models.CallbackAction {
	return func(ctx tb.Context) error {
		bot.Mu.RLock()
		pub, ok := bot.Publishers[clientID]
		bot.Mu.RUnlock()

		if !ok {
			return bot.Send(ctx, "❌ Component disconnected.")
		}

		bot.Mu.RLock()
		var btn *models.CommandButton
		comp, ok := bot.DynamicMenus[clientID]
		if ok {
			btn = findButtonByLabel(comp.Root, label)
		}
		bot.Mu.RUnlock()

		if btn != nil && btn.InputPrompt != "" {
			bot.Mu.Lock()
			bot.PendingInputs[ctx.Chat().ID] = btn
			bot.Mu.Unlock()
			return bot.Send(ctx, btn.InputPrompt, &tb.ReplyMarkup{RemoveKeyboard: true})
		}

		bot.Log.Info("Executing component command", "client", clientID, "type", cmdType)

		dispatchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := pub.PublishCommand(dispatchCtx, cmdType, payload, ""); err != nil {
			return bot.Send(ctx, fmt.Sprintf("⚠️ Failed to send command: %v", err))
		}

		return bot.Send(ctx, fmt.Sprintf("✅ Sent: %s", label))
	}
}

func registerAction(bot *core.Bot, fn models.CallbackAction) string {
	bot.Mu.Lock()
	defer bot.Mu.Unlock()
	bot.CbCounter++
	id := fmt.Sprintf("dyn_%d", bot.CbCounter)
	bot.ActionMap[id] = fn
	return id
}

func getMenuByPath(root *models.CommandMenu, path []string) *models.CommandMenu {
	if root == nil {
		return nil
	}
	current := root
	for _, step := range path {
		var next *models.CommandMenu
		for _, row := range current.Rows {
			for _, b := range row.Buttons {
				if b.Label == step && b.NextMenu != nil {
					next = b.NextMenu
					break
				}
			}
			if next != nil {
				break
			}
		}
		if next == nil {
			return current // path broke, return what we have so far
		}
		current = next
	}
	return current
}

func findButtonInMenu(m *models.CommandMenu, label string) *models.CommandButton {
	if m == nil {
		return nil
	}
	for _, row := range m.Rows {
		for _, b := range row.Buttons {
			if b.Label == label {
				return &b
			}
		}
	}
	return nil
}

func findButtonByLabel(m *models.CommandMenu, label string) *models.CommandButton {
	if m == nil {
		return nil
	}
	for _, row := range m.Rows {
		for _, b := range row.Buttons {
			if b.Label == label {
				return &b
			}
			if b.NextMenu != nil {
				if sub := findButtonByLabel(b.NextMenu, label); sub != nil {
					return sub
				}
			}
		}
	}
	return nil
}

// HandleDynamicText routes text messages from Reply buttons back to actions
func HandleDynamicText(bot *core.Bot, c tb.Context) error {
	text := c.Text()

	// 0. Intercept Input Mode
	bot.Mu.Lock()
	if pendingBtn, ok := bot.PendingInputs[c.Chat().ID]; ok {
		delete(bot.PendingInputs, c.Chat().ID)
		bot.Mu.Unlock()

		bot.Mu.RLock()
		clientID := bot.UserStates[c.Chat().ID]
		pub := bot.Publishers[clientID]
		bot.Mu.RUnlock()

		if pub != nil {
			_ = pub.PublishCommand(context.Background(), pendingBtn.CommandType, pendingBtn.Payload, text)
			// Return to current menu level after input
			bot.Mu.RLock()
			path := bot.UserPaths[c.Chat().ID]
			comp := bot.DynamicMenus[clientID]
			bot.Mu.RUnlock()
			if comp != nil {
				return RenderMenu(bot, c, getMenuByPath(comp.Root, path))
			}
			return ShowMainMenu(bot, c)
		}
		return bot.Send(c, "❌ Error: Service disconnected.")
	}
	bot.Mu.Unlock()

	// 1. Navigation: Main Menu
	if text == "📱 main menu" {
		bot.Mu.Lock()
		delete(bot.UserStates, c.Chat().ID)
		delete(bot.UserPaths, c.Chat().ID)
		bot.Mu.Unlock()
		return ShowMainMenu(bot, c)
	}

	// 2. Navigation: Back
	if strings.HasPrefix(text, "🔙 ") {
		bot.Mu.Lock()
		path := bot.UserPaths[c.Chat().ID]
		if len(path) > 0 {
			newPath := path[:len(path)-1]
			bot.UserPaths[c.Chat().ID] = newPath
			bot.Mu.Unlock()

			bot.Mu.RLock()
			clientID := bot.UserStates[c.Chat().ID]
			comp := bot.DynamicMenus[clientID]
			bot.Mu.RUnlock()
			if comp != nil {
				return RenderMenu(bot, c, getMenuByPath(comp.Root, newPath))
			}
		} else {
			delete(bot.UserStates, c.Chat().ID)
			delete(bot.UserPaths, c.Chat().ID)
			bot.Mu.Unlock()
		}
		return ShowMainMenu(bot, c)
	}

	// 3. Navigation: Manual Refresh
	if text == "🔄 refresh" {
		bot.Mu.RLock()
		clientID, inComp := bot.UserStates[c.Chat().ID]
		path := bot.UserPaths[c.Chat().ID]
		pub := bot.Publishers[clientID]
		bot.Mu.RUnlock()

		if inComp && pub != nil {
			// Component Level Refresh
			_ = pub.RequestRefresh(context.Background())
			time.Sleep(200 * time.Millisecond)

			bot.Mu.RLock()
			updatedComp := bot.DynamicMenus[clientID]
			bot.Mu.RUnlock()

			if updatedComp != nil {
				return RenderMenu(bot, c, getMenuByPath(updatedComp.Root, path))
			}
		} else {
			// Main Menu Level Refresh
			return ShowMainMenu(bot, c)
		}
		return nil
	}

	// 4. Navigation: Entry Points (Microservices)
	if strings.HasPrefix(text, "📂 ") || strings.HasPrefix(text, "⚙️ ") {
		parts := strings.SplitN(text, " ", 2)
		if len(parts) < 2 {
			return nil
		}
		compName := parts[1]

		bot.Mu.RLock()
		var found *models.ComponentMenu
		var targetID string
		for id, m := range bot.DynamicMenus {
			if m.Name == compName {
				found = m
				targetID = id
				break
			}
		}
		bot.Mu.RUnlock()

		if found != nil {
			bot.Mu.Lock()
			bot.UserStates[c.Chat().ID] = targetID
			bot.UserPaths[c.Chat().ID] = []string{}
			bot.Mu.Unlock()
			return RenderMenu(bot, c, found.Root)
		}
	}

	// 5. Navigation: In-Menu Buttons
	bot.Mu.RLock()
	clientID, inComp := bot.UserStates[c.Chat().ID]
	path := bot.UserPaths[c.Chat().ID]
	comp, ok := bot.DynamicMenus[clientID]
	pub := bot.Publishers[clientID]
	bot.Mu.RUnlock()

	if inComp && ok {
		currentMenu := getMenuByPath(comp.Root, path)
		if currentMenu != nil {
			btn := findButtonInMenu(currentMenu, text)
			if btn != nil {
				if btn.NextMenu != nil {
					// Enter Sub-menu
					bot.Mu.Lock()
					bot.UserPaths[c.Chat().ID] = append(bot.UserPaths[c.Chat().ID], text)
					newPath := bot.UserPaths[c.Chat().ID]
					bot.Mu.Unlock()

					return RenderMenu(bot, c, getMenuByPath(comp.Root, newPath))
				}

				bot.Mu.RLock()
				fn, exists := bot.ActionMap[btn.CallbackData]
				bot.Mu.RUnlock()
				if exists {
					err := fn(c)
					if pub != nil {
						_ = pub.RequestRefresh(context.Background())
					}
					return err
				}
			}
		}
	}

	return nil
}

// RenderMenu builds and displays a CommandMenu as a Reply Keyboard
func RenderMenu(bot *core.Bot, c tb.Context, m *models.CommandMenu) error {
	if m == nil {
		return bot.Send(c, "❌ Menu data unavailable. Please return to the main menu.")
	}

	menuMarkup := &tb.ReplyMarkup{ResizeKeyboard: true}
	var finalRows []tb.Row

	// Create ONE button per row (vertical stack)
	for _, row := range m.Rows {
		for _, b := range row.Buttons {
			// Append each button as an entirely new row slice
			finalRows = append(finalRows, menuMarkup.Row(menuMarkup.Text(b.Label)))
		}
	}

	// Calculate Dynamic Back Label
	backLabel := "🔙 config"
	bot.Mu.RLock()
	if clientID, ok := bot.UserStates[c.Chat().ID]; ok {
		if comp, exists := bot.DynamicMenus[clientID]; exists {
			if comp.Name != "Config Server" {
				backLabel = fmt.Sprintf("🔙 %s", comp.Name)
			}
		}
	}
	bot.Mu.RUnlock()

	// Add Navigation Footer (grouped horizontally on ONE line)
	finalRows = append(finalRows, menuMarkup.Row(
		menuMarkup.Text(backLabel),
		menuMarkup.Text("🔄 refresh"),
		menuMarkup.Text("📱 main menu"),
	))

	menuMarkup.Reply(finalRows...)
	return bot.Send(c, fmt.Sprintf("📍 %s", m.Title), menuMarkup)
}

// ShowMainMenu displays the dynamic top-level menu
func ShowMainMenu(bot *core.Bot, c tb.Context) error {
	menuStart := &tb.ReplyMarkup{ResizeKeyboard: true}
	var finalRows []tb.Row

	// A. Microservices (stacked vertically)
	bot.Mu.RLock()
	for _, m := range bot.DynamicMenus {
		icon := "📂 "
		if strings.Contains(m.Name, "Config") {
			icon = "⚙️ "
		}
		finalRows = append(finalRows, menuStart.Row(menuStart.Text(fmt.Sprintf("%s%s", icon, m.Name))))
	}
	bot.Mu.RUnlock()

	// B. Emergency Controls + refresh (grouped horizontally on ONE line)
	finalRows = append(finalRows, menuStart.Row(
		menuStart.Text("🆘 power off !"),
		menuStart.Text("🔄 refresh"),
		menuStart.Text("⏏️ close all positions"),
	))

	menuStart.Reply(finalRows...)
	return bot.Send(c, "Welcome to Antigravity Remote.", menuStart)
}
