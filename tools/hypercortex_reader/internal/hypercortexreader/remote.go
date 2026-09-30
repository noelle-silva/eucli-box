package hypercortexreader

// 以下结构镜像 HyperCortex 外部访问接口的响应：只解出本工具需要的字段，
// 未知字段一律忽略（接口向前演进不应打断读取）。

type faceKindInfo struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

type noteSearchFaceHit struct {
	FaceID  string `json:"faceId"`
	Kind    string `json:"kind"`
	Title   string `json:"title"`
	Snippet string `json:"snippet"`
}

type noteSearchHit struct {
	NoteID      string              `json:"noteId"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	Dir         string              `json:"dir"`
	CreatedAtMs float64             `json:"createdAtMs"`
	UpdatedAtMs float64             `json:"updatedAtMs"`
	NoteFields  []string            `json:"noteFields"`
	FaceHits    []noteSearchFaceHit `json:"faceHits"`
}

type noteSearchResult struct {
	Kinds            []faceKindInfo  `json:"kinds"`
	AppliedFaceKinds []string        `json:"appliedFaceKinds"`
	Items            []noteSearchHit `json:"items"`
}

type noteFaceManifest struct {
	ID       string         `json:"id"`
	Kind     string         `json:"kind"`
	Title    string         `json:"title"`
	File     string         `json:"file"`
	Settings map[string]any `json:"settings"`
}

type noteManifest struct {
	SchemaVersion int                         `json:"schemaVersion"`
	ID            string                      `json:"id"`
	Title         string                      `json:"title"`
	Description   string                      `json:"description"`
	Tags          []string                    `json:"tags"`
	CreatedAtMs   float64                     `json:"createdAtMs"`
	UpdatedAtMs   float64                     `json:"updatedAtMs"`
	FaceOrder     []string                    `json:"faceOrder"`
	Faces         map[string]noteFaceManifest `json:"faces"`
}

type noteFaceDoc struct {
	ID          string           `json:"id"`
	PackageDir  string           `json:"packageDir"`
	NoteID      string           `json:"noteId"`
	NoteTitle   string           `json:"noteTitle"`
	Face        noteFaceManifest `json:"face"`
	Content     string           `json:"content"`
	Exists      bool             `json:"exists"`
	CreatedAtMs float64          `json:"createdAtMs"`
	UpdatedAtMs float64          `json:"updatedAtMs"`
}

type refRelationNode struct {
	NoteID   string `json:"noteId"`
	Distance int    `json:"distance"`
}

// noteVersionSummary 是版本快照的摘要视图（与写工具同款）。
type noteVersionSummary struct {
	VersionID   string   `json:"versionId"`
	CommitName  string   `json:"commitName"`
	CreatedAtMs float64  `json:"createdAtMs"`
	Title       string   `json:"title"`
	FaceIDs     []string `json:"faceIds"`
}

// noteVersionFaceSnapshot 是快照中单个面的清单与内容。
type noteVersionFaceSnapshot struct {
	Manifest noteFaceManifest `json:"manifest"`
	Content  string           `json:"content"`
}

// noteVersionSnapshot 是版本快照的完整视图。
type noteVersionSnapshot struct {
	SchemaVersion int                                `json:"schemaVersion"`
	VersionID     string                             `json:"versionId"`
	NoteID        string                             `json:"noteId"`
	PackageDir    string                             `json:"packageDir"`
	CommitName    string                             `json:"commitName"`
	CreatedAtMs   float64                            `json:"createdAtMs"`
	ContentHash   string                             `json:"contentHash"`
	Manifest      noteManifest                       `json:"manifest"`
	Faces         map[string]noteVersionFaceSnapshot `json:"faces"`
}

type refRelationEdge struct {
	FromNoteID string `json:"fromNoteId"`
	FromFaceID string `json:"fromFaceId"`
	ToNoteID   string `json:"toNoteId"`
	ToFaceID   string `json:"toFaceId"`
}

type refRelationResult struct {
	Nodes []refRelationNode `json:"nodes"`
	Edges []refRelationEdge `json:"edges"`
}

type favoriteFolder struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	CreatedAtMs float64 `json:"createdAtMs"`
	UpdatedAtMs float64 `json:"updatedAtMs"`
}

type favoriteItemRef struct {
	ID          string  `json:"id"`
	FolderID    string  `json:"folderId"`
	Kind        string  `json:"kind"`
	TargetID    string  `json:"targetId"`
	CreatedAtMs float64 `json:"createdAtMs"`
	UpdatedAtMs float64 `json:"updatedAtMs"`
}

type favoritesDoc struct {
	Version        int                          `json:"version"`
	RootFolderID   string                       `json:"rootFolderId"`
	Folders        map[string]favoriteFolder    `json:"folders"`
	RefsByFolderID map[string][]favoriteItemRef `json:"refsByFolderId"`
	UpdatedAtMs    float64                      `json:"updatedAtMs"`
}

type assetItem struct {
	RelPath      string   `json:"relPath"`
	Name         string   `json:"name"`
	AssetID      string   `json:"assetId"`
	Ext          string   `json:"ext"`
	Kind         string   `json:"kind"`
	Mime         string   `json:"mime"`
	SourceName   string   `json:"sourceName"`
	DisplayName  string   `json:"displayName"`
	Remark       string   `json:"remark"`
	Tags         []string `json:"tags"`
	Size         int64    `json:"size"`
	CreatedAtMs  float64  `json:"createdAtMs"`
	UploadedAtMs float64  `json:"uploadedAtMs"`
	UpdatedAtMs  float64  `json:"updatedAtMs"`
	ModifiedMs   float64  `json:"modifiedMs"`
}

type assetPoolPage struct {
	Items []assetItem `json:"items"`
	Total int         `json:"total"`
}

type trashItem struct {
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Dir         string  `json:"dir"`
	AssetID     string  `json:"assetId"`
	Ext         string  `json:"ext"`
	NoteID      string  `json:"noteId"`
	FaceID      string  `json:"faceId"`
	CreatedAtMs float64 `json:"createdAtMs"`
	UpdatedAtMs float64 `json:"updatedAtMs"`
	DeletedAtMs float64 `json:"deletedAtMs"`
	OriginalDir string  `json:"originalDir"`
}
