package everything

// 以下结构镜像 Everything 应用开放接口的响应：只解出本工具需要的字段，
// 未知字段一律忽略（接口向前演进不应打断读取）。

type searchResult struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	FullPath   string `json:"fullPath"`
	Kind       string `json:"kind"`
	Size       string `json:"size"`
	ModifiedAt string `json:"modifiedAt"`
}

type searchResultPayload struct {
	Query     string         `json:"query"`
	Limit     int            `json:"limit"`
	ScopePath string         `json:"scopePath"`
	Results   []searchResult `json:"results"`
}
