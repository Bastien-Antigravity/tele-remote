package models

// RegistryState represents the persisted state of all connected microservices
type RegistryState struct {
	Components map[string]*ComponentMenu `json:"components"`
}
