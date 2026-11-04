package types

type WorkspaceDirectory struct {
	Path        string `json:"path"`
	Alias       string `json:"alias"`
	Description string `json:"description,omitempty"`
}
