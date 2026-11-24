package ui

/*
ESSENTIAL PROCESS:
Handles atomic dynamic menu registration, Reply Keyboard rendering, 2-step emergency
command confirmations, scoped pending-input routing, and direct button action dispatch.

DATA FLOW:
1. OnComponentConnected validates and parses menuJSON into a CommandMenu tree before acquiring bot.Mu.Lock() to atomically swap the component session.
2. RequestEmergencyConfirmation records a 60s PendingConfirmation per ConversationKey and renders a confirmation Reply Keyboard.
3. HandleDynamicText resolves confirmations, pending text inputs (routed strictly to pending.ClientID), navigation controls, and direct menu button execution.

KEY PARAMETERS:
- convKey: Scoped ConversationKey{ChatID, UserID} isolating multi-user state within a chat.
- menuJSON: Raw JSON array defining a component's interactive menu tree.
*/

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Bastien-Antigravity/tele-remote/src/interfaces"
	"github.com/Bastien-Antigravity/tele-remote/src/models"
	"github.com/Bastien-Antigravity/tele-remote/src/telegram/core"

	tb "gopkg.in/telebot.v3"
)

// -----------------------------------------------------------------------------

const (
	emergencyConfirmTTL = 60 * time.Second
	pendingInputTTL     = 5 * time.Minute
	interactiveTimeout  = 3 * time.Second
	emergencyTimeout    = 10 * time.Second
	maxMenuJSONBytes    = 65536
	maxMenuDepth        = 8
)

// -----------------------------------------------------------------------------

type broadcastTarget struct {
	clientID string
	name     string
	pub      interfaces.IPublisher
}

// -----------------------------------------------------------------------------

func getConvKey(c tb.Context) core.ConversationKey {
	return core.GetConvKey(c)
}

// -----------------------------------------------------------------------------

func clonePath(path []string) []string {
	if len(path) == 0 {
		return []string{}
	}
	cp := make([]string, len(path))
	copy(cp, path)
	return cp
}

// -----------------------------------------------------------------------------

func defaultComponentPriority(name string) int {
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "web"):
		return 5
	case strings.Contains(n, "squad"):
		return 10
	case strings.Contains(n, "watchdog"):
		return 15
	case strings.Contains(n, "config"):
		return 20
	case strings.Contains(n, "log"):
		return 30
	case strings.Contains(n, "notif"):
		return 35
	case strings.Contains(n, "tele"):
		return 40
	case strings.Contains(n, "rag"):
		return 45
	default:
		return 100
	}
}

// -----------------------------------------------------------------------------

func extractNumericOrder(data map[string]interface{}) int {
	if data == nil {
		return 0
	}
	for _, key := range []string{"priority", "order", "id"} {
		raw, exists := data[key]
		if !exists || raw == nil {
			continue
		}
		switch v := raw.(type) {
		case float64:
			if int(v) > 0 {
				return int(v)
			}
		case int:
			if v > 0 {
				return v
			}
		case string:
			if parsed, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && parsed > 0 {
				return parsed
			}
		}
	}
	return 0
}

// -----------------------------------------------------------------------------

func normalizeOrder(order int) int {
	if order <= 0 {
		return 1000
	}
	return order
}

// -----------------------------------------------------------------------------

func sortByOrder[T any](items []T, getOrder func(T) int) {
	hasOrder := false
	for _, item := range items {
		if getOrder(item) > 0 {
			hasOrder = true
			break
		}
	}
	if !hasOrder {
		return
	}
	sort.SliceStable(items, func(i, j int) bool {
		return normalizeOrder(getOrder(items[i])) < normalizeOrder(getOrder(items[j]))
	})
}

// -----------------------------------------------------------------------------

// OnComponentConnected is triggered when a client connects via gRPC or NATS.
// It unmarshals and validates menuJSON BEFORE locking bot.Mu so malformed payloads
// never wipe out an existing valid component session.
func OnComponentConnected(bot *core.Bot, clientID, componentName, menuJSON string, pub interfaces.IPublisher) {
	bot.Log.Info("Component connection handshake starting", "id", clientID, "name", componentName)
	if strings.TrimSpace(menuJSON) == "" {
		bot.Log.Warning("Component connected with empty menu", "id", clientID)
		return
	}
	if len(menuJSON) > maxMenuJSONBytes {
		bot.Log.Error("Component menu JSON exceeds maximum allowed size", "id", clientID, "bytes", len(menuJSON))
		return
	}

	var rawItems []map[string]interface{}
	var wrapperMap map[string]interface{}
	if err := json.Unmarshal([]byte(menuJSON), &rawItems); err != nil {
		if objErr := json.Unmarshal([]byte(menuJSON), &wrapperMap); objErr != nil {
			bot.Log.Error("Failed to parse component menu JSON", "err", err, "json", menuJSON)
			return
		}
		for _, key := range []string{"rows", "menu", "buttons"} {
			if arr, ok := wrapperMap[key].([]interface{}); ok {
				for _, elem := range arr {
					if itemMap, ok := elem.(map[string]interface{}); ok {
						rawItems = append(rawItems, itemMap)
					}
				}
				break
			}
		}
	}

	compPriority := 0
	if wrapperMap != nil {
		compPriority = extractNumericOrder(wrapperMap)
	}

	startIdx := 0
	if compPriority <= 0 && len(rawItems) > 0 {
		first := rawItems[0]
		labelVal, _ := first["label"].(string)
		btnsVal, hasBtns := first["buttons"].([]interface{})
		menuVal, hasMenu := first["menu"].([]interface{})
		isMetadataOnly := strings.TrimSpace(labelVal) == "" && (!hasBtns || len(btnsVal) == 0) && (!hasMenu || len(menuVal) == 0)

		if rawCP, hasCP := first["component_priority"]; hasCP {
			compPriority = extractNumericOrder(map[string]interface{}{"priority": rawCP})
		}
		if isMetadataOnly {
			if compPriority <= 0 {
				compPriority = extractNumericOrder(first)
			}
			startIdx = 1
		}
	}
	if compPriority <= 0 {
		compPriority = defaultComponentPriority(componentName)
	}

	root := &models.CommandMenu{
		Title: fmt.Sprintf("📦 %s", componentName),
		Rows:  []models.CommandRow{},
	}

	for _, item := range rawItems[startIdx:] {
		row, err := parseMenuRow(item, 0)
		if err != nil {
			bot.Log.Error("Failed to parse component menu tree", "id", clientID, "err", err)
			return
		}
		if len(row.Buttons) > 0 {
			root.Rows = append(root.Rows, row)
		}
	}
	sortByOrder(root.Rows, func(r models.CommandRow) int { return r.Order })

	now := time.Now().UTC()
	compMenu := &models.ComponentMenu{
		Name:         componentName,
		ClientID:     clientID,
		Priority:     compPriority,
		RegisteredAt: now,
		Root:         root,
	}

	bot.Mu.Lock()
	nextRev := uint64(1)
	registeredAt := now
	if existing, ok := bot.DynamicMenus[clientID]; ok && existing != nil {
		nextRev = existing.Revision + 1
		if !existing.RegisteredAt.IsZero() {
			registeredAt = existing.RegisteredAt
		}
	}

	// Clean up old sessions with the same name to prevent duplicates
	for id, m := range bot.DynamicMenus {
		if m != nil && m.Name == componentName && id != clientID {
			if m.Revision >= nextRev {
				nextRev = m.Revision + 1
			}
			if registeredAt.Equal(now) && !m.RegisteredAt.IsZero() {
				registeredAt = m.RegisteredAt
			}
			delete(bot.DynamicMenus, id)
			delete(bot.Publishers, id)
			// Migrate or clean up user states pointing to the old session ID
			for key, activeID := range bot.UserStates {
				if activeID == id {
					bot.UserStates[key] = clientID
					bot.UserPaths[key] = []string{}
				}
			}
			for key, pending := range bot.PendingInputs {
				if pending.ClientID == id {
					delete(bot.PendingInputs, key)
				}
			}
		}
	}

	// Clean up stale state for the same clientID on re-registration or menu refresh
	for key, pending := range bot.PendingInputs {
		if pending.ClientID == clientID {
			delete(bot.PendingInputs, key)
		}
	}
	for key, activeID := range bot.UserStates {
		if activeID == clientID {
			if getMenuByPath(root, bot.UserPaths[key]) == nil {
				bot.UserPaths[key] = []string{}
			}
		}
	}

	compMenu.Revision = nextRev
	compMenu.RegisteredAt = registeredAt
	bot.Publishers[clientID] = pub
	bot.DynamicMenus[clientID] = compMenu
	bot.Mu.Unlock()

	bot.Log.Info("Dynamic menu registered", "client", clientID, "rows", len(root.Rows), "revision", nextRev, "priority", compPriority)
}

// -----------------------------------------------------------------------------

func parseMenuRow(data map[string]interface{}, depth int) (models.CommandRow, error) {
	if depth > maxMenuDepth {
		return models.CommandRow{}, fmt.Errorf("menu recursion depth exceeds limit of %d", maxMenuDepth)
	}
	row := models.CommandRow{
		Order:   extractNumericOrder(data),
		Buttons: []models.CommandButton{},
	}
	if btns, ok := data["buttons"].([]interface{}); ok {
		for _, b := range btns {
			if bMap, ok := b.(map[string]interface{}); ok {
				btn, err := parseButton(bMap, depth)
				if err != nil {
					return models.CommandRow{}, err
				}
				row.Buttons = append(row.Buttons, btn)
			}
		}
	} else {
		btn, err := parseButton(data, depth)
		if err != nil {
			return models.CommandRow{}, err
		}
		row.Buttons = append(row.Buttons, btn)
	}
	sortByOrder(row.Buttons, func(b models.CommandButton) int { return b.Order })
	return row, nil
}

// -----------------------------------------------------------------------------

func parseButton(data map[string]interface{}, depth int) (models.CommandButton, error) {
	if depth > maxMenuDepth {
		return models.CommandButton{}, fmt.Errorf("menu recursion depth exceeds limit of %d", maxMenuDepth)
	}
	label, _ := data["label"].(string)
	order := extractNumericOrder(data)

	if sub, ok := data["menu"].([]interface{}); ok {
		subMenu := &models.CommandMenu{Title: label, Rows: []models.CommandRow{}}
		for _, s := range sub {
			if sMap, ok := s.(map[string]interface{}); ok {
				subRow, err := parseMenuRow(sMap, depth+1)
				if err != nil {
					return models.CommandButton{}, err
				}
				if len(subRow.Buttons) > 0 {
					subMenu.Rows = append(subMenu.Rows, subRow)
				}
			}
		}
		sortByOrder(subMenu.Rows, func(r models.CommandRow) int { return r.Order })
		return models.CommandButton{Label: label, Order: order, NextMenu: subMenu}, nil
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

	return models.CommandButton{
		Label:       label,
		Order:       order,
		CommandType: cmdType,
		Payload:     payload,
		InputPrompt: inputPrompt,
	}, nil
}

// -----------------------------------------------------------------------------

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
			return nil
		}
		current = next
	}
	return current
}

// -----------------------------------------------------------------------------

func renderResolvedMenu(bot *core.Bot, c tb.Context, convKey core.ConversationKey, root *models.CommandMenu, path []string) error {
	target := getMenuByPath(root, path)
	if target == nil {
		bot.Mu.Lock()
		bot.UserPaths[convKey] = []string{}
		bot.Mu.Unlock()
		target = root
	}
	if target == nil {
		return ShowMainMenu(bot, c)
	}
	return RenderMenu(bot, c, target)
}

// -----------------------------------------------------------------------------

func findButtonInMenu(m *models.CommandMenu, label string) *models.CommandButton {
	if m == nil {
		return nil
	}
	for _, row := range m.Rows {
		for i := range row.Buttons {
			if row.Buttons[i].Label == label {
				btnCopy := row.Buttons[i]
				return &btnCopy
			}
		}
	}
	return nil
}

// -----------------------------------------------------------------------------

// RequestEmergencyConfirmation initiates a 2-step confirmation workflow for CmdPowerOff or CmdStop.
func RequestEmergencyConfirmation(bot *core.Bot, c tb.Context, cmdType int32) error {
	convKey := getConvKey(c)

	var confirmLabel, promptText string
	switch cmdType {
	case models.CmdPowerOff:
		confirmLabel = models.BtnConfirmPowerOff
		promptText = "⚠️ Are you sure you want to POWER OFF all connected components? Please confirm within 60s."
	default:
		return fmt.Errorf("unsupported emergency command type: %d", cmdType)
	}

	bot.Mu.Lock()
	bot.PendingConfirmations[convKey] = core.PendingConfirmation{
		CommandType: cmdType,
		ExpiresAt:   time.Now().Add(emergencyConfirmTTL),
	}
	bot.Mu.Unlock()

	markup := &tb.ReplyMarkup{ResizeKeyboard: true}
	markup.Reply(
		markup.Row(markup.Text(confirmLabel)),
		markup.Row(markup.Text(models.BtnCancel)),
	)

	return bot.Send(c, promptText, markup)
}

// -----------------------------------------------------------------------------

func executeEmergencyBroadcast(bot *core.Bot, c tb.Context, cmdType int32) error {
	bot.Mu.RLock()
	targets := make([]broadcastTarget, 0, len(bot.Publishers))
	for clientID, pub := range bot.Publishers {
		if pub == nil {
			continue
		}
		name := clientID
		if comp, exists := bot.DynamicMenus[clientID]; exists && comp != nil && comp.Name != "" {
			name = comp.Name
		}
		targets = append(targets, broadcastTarget{
			clientID: clientID,
			name:     name,
			pub:      pub,
		})
	}
	bot.Mu.RUnlock()

	if len(targets) == 0 {
		_ = bot.Send(c, "ℹ️ No components currently connected.")
		return ShowMainMenu(bot, c)
	}

	dispatchCtx, cancel := context.WithTimeout(context.Background(), emergencyTimeout)
	defer cancel()

	var wg sync.WaitGroup
	var mu sync.Mutex
	succeeded := 0
	var failed []string

	for _, t := range targets {
		wg.Add(1)
		go func(target broadcastTarget) {
			defer wg.Done()
			if err := target.pub.PublishCommand(dispatchCtx, cmdType, "", ""); err != nil {
				mu.Lock()
				failed = append(failed, fmt.Sprintf("%s (%v)", target.name, err))
				mu.Unlock()
				return
			}
			mu.Lock()
			succeeded++
			mu.Unlock()
		}(t)
	}
	wg.Wait()
	sort.Strings(failed)

	if len(failed) == 0 {
		_ = bot.Send(c, fmt.Sprintf("🆘 Power-off command delivered to all %d connected components.", succeeded))
	} else {
		_ = bot.Send(c, fmt.Sprintf("⚠️ Power-off delivered to %d/%d components. Failed: %s", succeeded, len(targets), strings.Join(failed, ", ")))
	}

	return ShowMainMenu(bot, c)
}

// -----------------------------------------------------------------------------

func executeButtonAction(bot *core.Bot, c tb.Context, convKey core.ConversationKey, clientID string, currentPath []string, btn *models.CommandButton) error {
	bot.Mu.RLock()
	pub, ok := bot.Publishers[clientID]
	var compRev uint64
	if comp, exists := bot.DynamicMenus[clientID]; exists && comp != nil {
		compRev = comp.Revision
	}
	bot.Mu.RUnlock()

	if !ok || pub == nil {
		return bot.Send(c, "❌ Component disconnected.")
	}

	if btn.InputPrompt != "" {
		bot.Mu.Lock()
		bot.PendingInputs[convKey] = core.PendingInput{
			ClientID:     clientID,
			MenuRevision: compRev,
			Button:       *btn,
			Path:         clonePath(currentPath),
			ExpiresAt:    time.Now().Add(pendingInputTTL),
		}
		bot.Mu.Unlock()
		return bot.Send(c, btn.InputPrompt, &tb.ReplyMarkup{RemoveKeyboard: true})
	}

	bot.Log.Info("Executing component command", "client", clientID, "type", btn.CommandType, "label", btn.Label)

	dispatchCtx, cancel := context.WithTimeout(context.Background(), interactiveTimeout)
	defer cancel()

	if err := pub.PublishCommand(dispatchCtx, btn.CommandType, btn.Payload, ""); err != nil {
		return bot.Send(c, fmt.Sprintf("⚠️ Failed to send command: %v", err))
	}

	err := bot.Send(c, fmt.Sprintf("✅ Sent: %s", btn.Label))
	_ = pub.RequestRefresh(dispatchCtx)
	return err
}

// -----------------------------------------------------------------------------

// HandleDynamicText routes text messages from Reply buttons and user prompts back to actions.
func HandleDynamicText(bot *core.Bot, c tb.Context) error {
	text := c.Text()
	convKey := getConvKey(c)

	// 0a. Emergency command triggers via text fallback (if routed through OnText)
	if text == models.BtnPowerOff {
		return RequestEmergencyConfirmation(bot, c, models.CmdPowerOff)
	}

	// 0b. Handle 2-Step Emergency Confirmation buttons & cancellation
	if text == models.BtnCancel {
		bot.Mu.Lock()
		_, hadConfirm := bot.PendingConfirmations[convKey]
		delete(bot.PendingConfirmations, convKey)
		delete(bot.PendingInputs, convKey)
		bot.Mu.Unlock()
		if hadConfirm {
			_ = bot.Send(c, "❌ Emergency command cancelled.")
		}
		return ShowMainMenu(bot, c)
	}

	if text == models.BtnConfirmPowerOff {
		expectedCmd := models.CmdPowerOff

		bot.Mu.Lock()
		pendingConf, hasConf := bot.PendingConfirmations[convKey]
		if hasConf {
			delete(bot.PendingConfirmations, convKey)
		}
		bot.Mu.Unlock()

		if !hasConf || pendingConf.CommandType != expectedCmd {
			_ = bot.Send(c, "⚠️ No matching pending confirmation found.")
			return ShowMainMenu(bot, c)
		}
		if time.Now().After(pendingConf.ExpiresAt) {
			_ = bot.Send(c, "⏰ Confirmation expired. Command aborted.")
			return ShowMainMenu(bot, c)
		}

		return executeEmergencyBroadcast(bot, c, expectedCmd)
	}

	// 0c. Atomically clear unconfirmed PendingConfirmation and check/extract PendingInput
	bot.Mu.Lock()
	delete(bot.PendingConfirmations, convKey)
	pending, hasPending := bot.PendingInputs[convKey]
	if hasPending {
		delete(bot.PendingInputs, convKey)
	}
	bot.Mu.Unlock()

	if hasPending {
		if !pending.ExpiresAt.IsZero() && time.Now().After(pending.ExpiresAt) {
			_ = bot.Send(c, "⏰ Input prompt expired. Please select the action again.")
			return ShowMainMenu(bot, c)
		}

		bot.Mu.RLock()
		pub := bot.Publishers[pending.ClientID]
		comp := bot.DynamicMenus[pending.ClientID]
		bot.Mu.RUnlock()

		if comp != nil && pending.MenuRevision != 0 && comp.Revision != pending.MenuRevision {
			_ = bot.Send(c, "⚠️ Component menu was updated; please select the action again.")
			return ShowMainMenu(bot, c)
		}

		if pub != nil {
			dispatchCtx, cancel := context.WithTimeout(context.Background(), interactiveTimeout)
			defer cancel()

			if err := pub.PublishCommand(dispatchCtx, pending.Button.CommandType, pending.Button.Payload, text); err != nil {
				return bot.Send(c, fmt.Sprintf("⚠️ Failed to send input: %v", err))
			}
			_ = pub.RequestRefresh(dispatchCtx)

			if comp != nil {
				return renderResolvedMenu(bot, c, convKey, comp.Root, pending.Path)
			}
			return ShowMainMenu(bot, c)
		}
		return bot.Send(c, "❌ Error: Service disconnected.")
	}

	// 1. Navigation: Main Menu
	if text == "📱 main menu" {
		bot.Mu.Lock()
		delete(bot.UserStates, convKey)
		delete(bot.UserPaths, convKey)
		bot.Mu.Unlock()
		return ShowMainMenu(bot, c)
	}

	// 2. Navigation: Back
	if strings.HasPrefix(text, "🔙 ") {
		bot.Mu.Lock()
		path := bot.UserPaths[convKey]
		if len(path) > 0 {
			newPath := clonePath(path[:len(path)-1])
			bot.UserPaths[convKey] = newPath
			clientID := bot.UserStates[convKey]
			comp := bot.DynamicMenus[clientID]
			bot.Mu.Unlock()

			if comp != nil {
				return renderResolvedMenu(bot, c, convKey, comp.Root, newPath)
			}
		} else {
			delete(bot.UserStates, convKey)
			delete(bot.UserPaths, convKey)
			bot.Mu.Unlock()
		}
		return ShowMainMenu(bot, c)
	}

	// 3. Navigation: Manual Refresh (no blocking time.Sleep)
	if text == "🔄 refresh" {
		bot.Mu.RLock()
		clientID, inComp := bot.UserStates[convKey]
		path := clonePath(bot.UserPaths[convKey])
		pub := bot.Publishers[clientID]
		updatedComp := bot.DynamicMenus[clientID]
		bot.Mu.RUnlock()

		if inComp && pub != nil {
			dispatchCtx, cancel := context.WithTimeout(context.Background(), interactiveTimeout)
			defer cancel()
			_ = pub.RequestRefresh(dispatchCtx)

			if updatedComp != nil {
				return renderResolvedMenu(bot, c, convKey, updatedComp.Root, path)
			}
		}
		return ShowMainMenu(bot, c)
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
			bot.UserStates[convKey] = targetID
			bot.UserPaths[convKey] = []string{}
			bot.Mu.Unlock()
			return RenderMenu(bot, c, found.Root)
		}
	}

	// 5. Navigation & Direct Action Dispatch: In-Menu Buttons
	bot.Mu.RLock()
	clientID, inComp := bot.UserStates[convKey]
	path := clonePath(bot.UserPaths[convKey])
	comp, ok := bot.DynamicMenus[clientID]
	bot.Mu.RUnlock()

	if inComp && ok && comp != nil {
		currentMenu := getMenuByPath(comp.Root, path)
		if currentMenu == nil {
			return renderResolvedMenu(bot, c, convKey, comp.Root, path)
		}
		btn := findButtonInMenu(currentMenu, text)
		if btn != nil {
			if btn.NextMenu != nil {
				newPath := append(clonePath(path), text)
				nextMenu := getMenuByPath(comp.Root, newPath)
				if nextMenu == nil {
					return renderResolvedMenu(bot, c, convKey, comp.Root, newPath)
				}
				bot.Mu.Lock()
				bot.UserPaths[convKey] = newPath
				bot.Mu.Unlock()

				return RenderMenu(bot, c, nextMenu)
			}

			return executeButtonAction(bot, c, convKey, clientID, path, btn)
		}
	}

	return nil
}

// -----------------------------------------------------------------------------

// RenderMenu builds and displays a CommandMenu as a Reply Keyboard.
func RenderMenu(bot *core.Bot, c tb.Context, m *models.CommandMenu) error {
	if m == nil {
		return bot.Send(c, "❌ Menu data unavailable. Please return to the main menu.")
	}

	convKey := getConvKey(c)
	menuMarkup := &tb.ReplyMarkup{ResizeKeyboard: true}
	var finalRows []tb.Row

	// Create ONE button per row (vertical stack)
	for _, row := range m.Rows {
		for _, b := range row.Buttons {
			finalRows = append(finalRows, menuMarkup.Row(menuMarkup.Text(b.Label)))
		}
	}

	// Calculate Dynamic Back Label
	backLabel := "🔙 config"
	bot.Mu.RLock()
	if clientID, ok := bot.UserStates[convKey]; ok {
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

// -----------------------------------------------------------------------------

// ShowMainMenu displays the dynamic top-level menu.
func ShowMainMenu(bot *core.Bot, c tb.Context) error {
	menuStart := &tb.ReplyMarkup{ResizeKeyboard: true}
	var finalRows []tb.Row

	// A. Microservices (stacked vertically in OpenMFE priority order with deterministic tie-breaking)
	bot.Mu.RLock()
	menus := make([]*models.ComponentMenu, 0, len(bot.DynamicMenus))
	for _, m := range bot.DynamicMenus {
		if m != nil {
			menus = append(menus, m)
		}
	}
	bot.Mu.RUnlock()

	sort.SliceStable(menus, func(i, j int) bool {
		pi := menus[i].Priority
		if pi <= 0 {
			pi = 100
		}
		pj := menus[j].Priority
		if pj <= 0 {
			pj = 100
		}
		if pi != pj {
			return pi < pj
		}
		nameI := strings.ToLower(strings.TrimSpace(menus[i].Name))
		nameJ := strings.ToLower(strings.TrimSpace(menus[j].Name))
		if nameI != nameJ {
			return nameI < nameJ
		}
		if !menus[i].RegisteredAt.Equal(menus[j].RegisteredAt) {
			return menus[i].RegisteredAt.Before(menus[j].RegisteredAt)
		}
		return menus[i].ClientID < menus[j].ClientID
	})

	for _, m := range menus {
		icon := "📂 "
		if strings.Contains(m.Name, "Config") {
			icon = "⚙️ "
		}
		finalRows = append(finalRows, menuStart.Row(menuStart.Text(fmt.Sprintf("%s%s", icon, m.Name))))
	}

	// B. Emergency Controls + refresh (grouped horizontally on ONE line)
	finalRows = append(finalRows, menuStart.Row(
		menuStart.Text(models.BtnPowerOff),
		menuStart.Text("🔄 refresh"),
	))

	menuStart.Reply(finalRows...)
	return bot.Send(c, "Welcome to Antigravity Remote.", menuStart)
}
