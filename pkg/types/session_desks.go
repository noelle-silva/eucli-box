package types

type SessionDesks struct {
	Folders            []SessionDeskFolder             `json:"folders"`
	ChatRefsByFolderID map[string][]SessionDeskChatRef `json:"chatRefsByFolderId"`
}

type SessionDeskFolder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parentId"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type SessionDeskChatRef struct {
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	RoleID     string `json:"roleId,omitempty"`
	ChatID     string `json:"chatId"`
	AddedAt    int64  `json:"addedAt"`
}
