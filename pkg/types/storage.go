package types

import "time"

type StorageItemKind string

const (
	StorageItemSession  StorageItemKind = "session"
	StorageItemRole     StorageItemKind = "role"
	StorageItemProvider StorageItemKind = "provider"
	StorageItemTool     StorageItemKind = "tool"
)

type RecycleRecord struct {
	ID           string          `json:"id"`
	OriginalID   string          `json:"originalId"`
	OriginalType StorageItemKind `json:"originalType"`
	DeletedAt    time.Time       `json:"deletedAt"`
}

type ListOptions struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}
