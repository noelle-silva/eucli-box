package types

import "time"

type WorkspaceDirectory struct {
	Path        string `json:"path"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"`
}

type Workspace struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Directories []WorkspaceDirectory `json:"directories"`
	Prompt      string               `json:"prompt,omitempty"`
	CreatedAt   time.Time            `json:"createdAt"`
	UpdatedAt   time.Time            `json:"updatedAt"`
}

type WorkspaceSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}
