package models

import "time"

type ServiceStatus struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Healthy   bool                   `json:"healthy"`
	LastCheck time.Time              `json:"last_check"`
	Details   map[string]interface{} `json:"details,omitempty"`
	Error     string                 `json:"error,omitempty"`
}
