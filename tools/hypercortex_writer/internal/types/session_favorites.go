package types

type SessionFavorites struct {
	Folders            []SessionFavoriteFolder             `json:"folders"`
	ChatRefsByFolderID map[string][]SessionFavoriteChatRef `json:"chatRefsByFolderId"`
}

type SessionFavoriteFolder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parentId"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

type SessionFavoriteChatRef struct {
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	ChatID     string `json:"chatId"`
	AddedAt    int64  `json:"addedAt"`
}
