package hypercortexwriter

// 以下结构镜像 HyperCortex 外部访问接口的响应：只解出本工具需要的字段，
// 未知字段一律忽略（接口向前演进不应打断写入）。

type faceKindInfo struct {
	Kind            string          `json:"kind"`
	Label           string          `json:"label"`
	DefaultFaceID   string          `json:"defaultFaceId"`
	DefaultFileName string          `json:"defaultFileName"`
	Settings        []faceSettingField `json:"settings"`
}

// faceSettingField 是面类型的一个设置项声明（键名、形态与默认值）。
type faceSettingField struct {
	Key     string              `json:"key"`
	Kind    string              `json:"kind"`
	Label   string              `json:"label"`
	Default any                 `json:"default"`
	Options []faceSettingOption `json:"options"`
	Min     *float64            `json:"min"`
	Max     *float64            `json:"max"`
	Step    *float64            `json:"step"`
}

// faceSettingOption 是枚举设置项的一个可选值。
type faceSettingOption struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
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

type noteMeta struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Dir         string  `json:"dir"`
	CreatedAtMs float64 `json:"createdAtMs"`
	UpdatedAtMs float64 `json:"updatedAtMs"`
}

// noteSaveResult 是保存 / 补丁 / 面顺序 / 面设置等笔记写入结果的公共视图：
// meta + manifest + 显式的新版本标记。
type noteSaveResult struct {
	Meta     noteMeta     `json:"meta"`
	Manifest noteManifest `json:"manifest"`
	Version  float64      `json:"version"`
}

// noteMetadataResult 是笔记元数据增量入口的结果视图。
type noteMetadataResult struct {
	Version float64  `json:"version"`
	Changed bool     `json:"changed"`
	Meta    noteMeta `json:"meta"`
}

// noteVersionSummary 是版本快照的摘要视图。
type noteVersionSummary struct {
	VersionID   string  `json:"versionId"`
	CommitName  string  `json:"commitName"`
	CreatedAtMs float64 `json:"createdAtMs"`
	Title       string  `json:"title"`
}

type resourceRef struct {
	AssetID string `json:"assetId"`
	Mime    string `json:"mime"`
	Ext     string `json:"ext"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Marker  string `json:"marker"`
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

// favoriteFolder 是收藏夹文档中的一个收藏夹。
type favoriteFolder struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	CreatedAtMs float64 `json:"createdAtMs"`
	UpdatedAtMs float64 `json:"updatedAtMs"`
}

type favoriteItemRef struct {
	ID       string `json:"id"`
	FolderID string `json:"folderId"`
	Kind     string `json:"kind"`
	TargetID string `json:"targetId"`
}

type favoritesDoc struct {
	Version        int                          `json:"version"`
	RootFolderID   string                       `json:"rootFolderId"`
	Folders        map[string]favoriteFolder    `json:"folders"`
	RefsByFolderID map[string][]favoriteItemRef `json:"refsByFolderId"`
	UpdatedAtMs    float64                      `json:"updatedAtMs"`
}

// favoriteWriteResult 是收藏夹语义写入口的公共结果视图。
type favoriteWriteResult struct {
	Version      float64 `json:"version"`
	FolderID     string  `json:"folderId"`
	ParentID     string  `json:"parentId"`
	RefID        string  `json:"refId"`
	FromFolderID string  `json:"fromFolderId"`
	ToFolderID   string  `json:"toFolderId"`
	Changed      *bool   `json:"changed"`
}
