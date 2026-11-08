package types

// PromptFolderLibrary 是一个作用域下的收藏夹索引库。
// 收藏夹只保存结构与索引的提示词 id，不保存提示词内容。
type PromptFolderLibrary struct {
	// RootFolderIDs 是根级收藏夹的显式顺序。
	RootFolderIDs []string       `json:"rootFolderIds"`
	// RootEntryOrder 是根级的混排顺序：收藏夹与提示词条目同处一个列表。
	// 每项形如 folder:<id> 或 prompt:<id>；缺省时按「收藏夹在前、条目在后」派生。
	RootEntryOrder []string       `json:"rootEntryOrder,omitempty"`
	Folders       []PromptFolder `json:"folders"`
}

// PromptFolder 是一个收藏夹节点。
// 树只有一份真相：父级由「谁把本 id 写进 FolderIDs」决定，
// 因此不保存反向的父节点字段。FolderIDs 与 PromptIDs 的数组顺序即显式顺序。
type PromptFolder struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	FolderIDs []string `json:"folderIds"`
	PromptIDs []string `json:"promptIds"`
	// EntryOrder 是本夹内的混排顺序，元素形如 folder:<id> 或 prompt:<id>。
	// 它是显示的单一事实源；FolderIDs 与 PromptIDs 保持为索引事实。
	EntryOrder []string `json:"entryOrder,omitempty"`
	CreatedAt string   `json:"createdAt,omitempty"`
	UpdatedAt string   `json:"updatedAt,omitempty"`
}
