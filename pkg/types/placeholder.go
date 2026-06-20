package types

import "time"

const (
	PlaceholderProblemCycleReference = "cycle_reference"
	PlaceholderProblemDuplicateName  = "duplicate_name"
	PlaceholderProblemPluginFailed   = "plugin_failed"

	PlaceholderSourceSystemPlugin = "system_plugin"
)

type PlaceholderSource struct {
	Kind        string `json:"kind"`
	PluginID    string `json:"pluginId,omitempty"`
	InterfaceID string `json:"interfaceId,omitempty"`
}

type PlaceholderItem struct {
	Name        string             `json:"name"`
	Value       string             `json:"value"`
	Description string             `json:"description,omitempty"`
	Source      *PlaceholderSource `json:"source,omitempty"`
	CreatedAt   time.Time          `json:"createdAt"`
}

type PlaceholderFolder struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	ParentID         string    `json:"parentId,omitempty"`
	PlaceholderNames []string  `json:"placeholderNames,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type PlaceholderLibrary struct {
	Placeholders []PlaceholderItem   `json:"placeholders"`
	Folders      []PlaceholderFolder `json:"folders,omitempty"`
}

type PlaceholderProblem struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type PlaceholderResolveResult struct {
	Text     string               `json:"text"`
	Problems []PlaceholderProblem `json:"problems,omitempty"`
}

type PlaceholderPreviewRequest struct {
	Text string `json:"text"`
}

type PlaceholderDependencyNode struct {
	Name     string                      `json:"name"`
	Missing  bool                        `json:"missing,omitempty"`
	Cycle    bool                        `json:"cycle,omitempty"`
	Children []PlaceholderDependencyNode `json:"children,omitempty"`
}
