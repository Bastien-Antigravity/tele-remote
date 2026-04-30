package store

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/Bastien-Antigravity/tele-remote/src/models"
	unilog_ifaces "github.com/Bastien-Antigravity/universal-logger/src/interfaces"
)

// -----------------------------------------------------------------------------
// Persistence Manager
// -----------------------------------------------------------------------------

// PersistenceManager handles saving and loading the component registry state
type PersistenceManager struct {
	filePath string
	log      unilog_ifaces.Logger
	mu       sync.Mutex
}

// NewPersistenceManager creates a new manager with a target file path
func NewPersistenceManager(path string, log unilog_ifaces.Logger) *PersistenceManager {
	return &PersistenceManager{
		filePath: path,
		log:      log,
	}
}

// -----------------------------------------------------------------------------
// IO Operations
// -----------------------------------------------------------------------------

// Load reads the state from disk
func (pm *PersistenceManager) Load() (map[string]*models.ComponentMenu, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]*models.ComponentMenu), nil
		}
		return nil, err
	}

	var state models.RegistryState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal registry state: %w", err)
	}

	if state.Components == nil {
		state.Components = make(map[string]*models.ComponentMenu)
	}

	return state.Components, nil
}

// Save writes the current state to disk
func (pm *PersistenceManager) Save(components map[string]*models.ComponentMenu) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	state := models.RegistryState{
		Components: components,
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(pm.filePath, data, 0644)
}
