package types

// SessionDesks 是工作桌面的事实：一条有序条目清单。
//
// 条目只有两种形态：
//   - chat  一条会话引用，直接摆在桌面上
//   - group 一个命名分组，名字、颜色、折叠与成员都由它自己持有
//
// 顶层顺序就是 Items 的顺序，未分组条目与分组因此可以任意穿插。
// 旧版的两层结构（folders + chatRefsByFolderId）只在读取时识别并迁移一次，
// 迁移完成后置空，写入时不产出。
type SessionDesks struct {
	Items []SessionDeskItem `json:"items"`

	// 旧版结构：仅用于读取期迁移。写入时一律为空，不出现在文件里。
	LegacyFolders            []SessionDeskFolder             `json:"folders,omitempty"`
	LegacyChatRefsByFolderID map[string][]SessionDeskChatRef `json:"chatRefsByFolderId,omitempty"`
}

// SessionDeskItem 是桌面顶层的一项：要么一条会话，要么一个分组。
type SessionDeskItem struct {
	Type  string              `json:"type"`
	Chat  *SessionDeskChatRef `json:"chat,omitempty"`
	Group *SessionDeskGroup   `json:"group,omitempty"`
}

const (
	SessionDeskItemChat  = "chat"
	SessionDeskItemGroup = "group"
)

// SessionDeskGroup 是桌面上一个命名分组。
// Chats 的顺序即组内顺序；Collapsed 是「这组现在收起来了」这一事实本身。
type SessionDeskGroup struct {
	ID        string               `json:"id"`
	Name      string               `json:"name"`
	Color     string               `json:"color,omitempty"`
	Collapsed bool                 `json:"collapsed,omitempty"`
	Chats     []SessionDeskChatRef `json:"chats"`
	CreatedAt int64                `json:"createdAt"`
	UpdatedAt int64                `json:"updatedAt"`
}

// SessionDeskFolder 是旧版分组形态，仅供读取迁移识别。
type SessionDeskFolder struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parentId"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// SessionDeskChatRef 指向某个目标下的某条会话。
// workspace 目标带 roleId（同一工作区里不同角色各有各的会话），其余目标不带。
type SessionDeskChatRef struct {
	TargetKind string `json:"targetKind"`
	TargetID   string `json:"targetId"`
	RoleID     string `json:"roleId,omitempty"`
	ChatID     string `json:"chatId"`
	AddedAt    int64  `json:"addedAt"`
}
