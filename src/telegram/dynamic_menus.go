package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	
	"tele-remote/src/interfaces"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------
// OnComponentConnected is triggered when a client connects via gRPC
func (bot *Bot) OnComponentConnected(clientID, componentName, menuJSON string, pub interfaces.Publisher) {
	if menuJSON == "" {
		return
	}
	var compMenu ComponentMenu
	if err := json.Unmarshal([]byte(menuJSON), &compMenu); err != nil {
		bot.log.Error("Failed to parse menu JSON", "err", err, "client", clientID)
		return
	}

	bot.mu.Lock()
	if compMenu.Name == "" {
		compMenu.Name = componentName
	}
	bot.dynamicMenus[clientID] = &compMenu
	bot.publishers[clientID] = pub
	bot.mu.Unlock()

	bot.log.Info("Registered dynamic menu for client", "client", clientID, "name", compMenu.Name)
	bot.Broadcast(fmt.Sprintf("🔌 Connected node: %s", compMenu.Name))
}

// -----------------------------------------------------------------------------
// OnComponentDisconnected is triggered when a client drops the gRPC connection
func (bot *Bot) OnComponentDisconnected(clientID string) {
	bot.mu.Lock()
	if pub, exists := bot.publishers[clientID]; exists {
		pub.Close()
		delete(bot.publishers, clientID)
	}
	if compMenu, exists := bot.dynamicMenus[clientID]; exists {
		bot.log.Info("Removing dynamic menu for client", "client", clientID)
		bot.Broadcast(fmt.Sprintf("🔌 Disconnected node: %s", compMenu.Name))
		delete(bot.dynamicMenus, clientID)
	}
	bot.mu.Unlock()
}

// -----------------------------------------------------------------------------
// registerAction generates and stores a unique callback action ID
func (bot *Bot) registerAction(action CallbackAction) string {
	bot.mu.Lock()
	defer bot.mu.Unlock()
	bot.cbCounter++
	id := fmt.Sprintf("dyn_%d", bot.cbCounter)
	bot.actionMap[id] = action
	return id
}

// -----------------------------------------------------------------------------
// showNodesMenu renders the list of connected nodes with dynamic menus
func (bot *Bot) showNodesMenu(c tb.Context) error {
	bot.mu.RLock()
	defer bot.mu.RUnlock()

	if len(bot.dynamicMenus) == 0 {
		return c.Send("No connected nodes available at the moment.")
	}

	menu := &tb.ReplyMarkup{}
	var rows []tb.Row
	for clientID, compMenu := range bot.dynamicMenus {
		actionID := bot.registerAction(CallbackAction{
			Type:     "node_main",
			ClientID: clientID,
			Path:     []int{},
		})
		btn := menu.Data(compMenu.Name, actionID)
		rows = append(rows, menu.Row(btn))
	}
	menu.Inline(rows...)
	return c.Send("Select a Node:", menu)
}

// -----------------------------------------------------------------------------
// handleDynamicCallback routes inline callbacks to requested submenus or custom commands
func (bot *Bot) handleDynamicCallback(c tb.Context) error {
	cbData := strings.TrimSpace(c.Callback().Data)

	idx := strings.LastIndex(cbData, "|")
	if idx >= 0 {
		cbData = cbData[idx+1:]
	} else {
		cbData = strings.TrimLeft(cbData, "\f")
	}

	bot.mu.RLock()
	action, ok := bot.actionMap[cbData]
	bot.mu.RUnlock()

	if !ok {
		return c.Respond(&tb.CallbackResponse{Text: "Expired or invalid menu action."})
	}

	switch action.Type {
	case "node_main", "node_sub":
		return bot.renderSubMenu(c, action)
	case "execute":
		bot.log.Info("Triggering dynamic command", "cmd", action.Command, "client", action.ClientID)
		
		bot.mu.RLock()
		pub, exists := bot.publishers[action.ClientID]
		bot.mu.RUnlock()
		
		if !exists {
			return c.Respond(&tb.CallbackResponse{Text: "Publisher not found or node disconnected."})
		}
		
		err := pub.PublishCommand(context.Background(), 99, action.Command)
		if err != nil {
			return c.Respond(&tb.CallbackResponse{Text: "Error communicating with node."})
		}
		c.Send("✅ Command sent to node.")
		return c.Respond()
	}

	return c.Respond()
}

// -----------------------------------------------------------------------------
// renderSubMenu fetches the target nested menu and renders it
func (bot *Bot) renderSubMenu(c tb.Context, action CallbackAction) error {
	bot.mu.RLock()
	compMenu, exists := bot.dynamicMenus[action.ClientID]
	bot.mu.RUnlock()

	if !exists {
		return c.Respond(&tb.CallbackResponse{Text: "Node is no longer connected."})
	}

	menu := &tb.ReplyMarkup{}
	var rows []tb.Row

	currentItems := compMenu.Menu
	navName := compMenu.Name
	for _, p := range action.Path {
		if p >= 0 && p < len(currentItems) {
			navName = currentItems[p].Label
			currentItems = currentItems[p].SubMenus
		} else {
			return c.Respond(&tb.CallbackResponse{Text: "Menu path invalid."})
		}
	}

	for i, item := range currentItems {
		newPath := append([]int(nil), action.Path...)
		newPath = append(newPath, i)

		if item.Command != "" {
			actionID := bot.registerAction(CallbackAction{
				Type:     "execute",
				ClientID: action.ClientID,
				Command:  item.Command,
			})
			btn := menu.Data(item.Label, actionID)
			rows = append(rows, menu.Row(btn))
		} else if len(item.SubMenus) > 0 {
			actionID := bot.registerAction(CallbackAction{
				Type:     "node_sub",
				ClientID: action.ClientID,
				Path:     newPath,
			})
			btn := menu.Data("📁 "+item.Label, actionID)
			rows = append(rows, menu.Row(btn))
		}
	}

	if len(action.Path) > 0 {
		backPath := action.Path[:len(action.Path)-1]
		backActionID := bot.registerAction(CallbackAction{
			Type:     "node_sub",
			ClientID: action.ClientID,
			Path:     backPath,
		})
		backBtn := menu.Data("🔙 Back", backActionID)
		rows = append(rows, menu.Row(backBtn))
	}

	menu.Inline(rows...)
	_, err := bot.b.Edit(c.Message(), "Menu: "+navName, menu)
	if err != nil {
		bot.b.Send(c.Sender(), "Menu: "+navName, menu)
	}
	return c.Respond()
}
