package types

import "time"

type ChatGroupRandomConfig struct {
	WeightsByRoleID map[string]float64 `json:"weightsByRoleId,omitempty"`
	MinCount        int                `json:"minCount"`
	MaxCount        int                `json:"maxCount"`
}

type ChatGroup struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Avatar          string                `json:"avatar"`
	Prompt          string                `json:"prompt,omitempty"`
	Mode            string                `json:"mode"`
	MemberRoleIDs   []string              `json:"memberRoleIds"`
	RoundRobinOrder []string              `json:"roundRobinOrder"`
	Random          ChatGroupRandomConfig `json:"random"`
	CreatedAt       time.Time             `json:"createdAt"`
	UpdatedAt       time.Time             `json:"updatedAt"`
}

type ChatGroupSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Avatar    string    `json:"avatar"`
	UpdatedAt time.Time `json:"updatedAt"`
}
