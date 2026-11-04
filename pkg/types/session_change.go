package types

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// SessionChangeTargetKind 标识会话变更事件作用的目标类别。
type SessionChangeTargetKind string

const (
	SessionChangeTargetSession SessionChangeTargetKind = "session"
	SessionChangeTargetMessage SessionChangeTargetKind = "message"
)

// SessionLocationKind 标识会话定位的形态。
type SessionLocationKind string

const (
	SessionLocationRole      SessionLocationKind = "role"
	SessionLocationGroup     SessionLocationKind = "group"
	SessionLocationWorkspace SessionLocationKind = "workspace"
)

// SessionLocation 以「形态 + 该形态的完整定位字段 + 会话 ID」唯一标识一个会话。
type SessionLocation struct {
	Kind        SessionLocationKind `json:"kind"`
	RoleID      string              `json:"roleId,omitempty"`
	GroupID     string              `json:"groupId,omitempty"`
	WorkspaceID string              `json:"workspaceId,omitempty"`
	SessionID   string              `json:"sessionId"`
}

func RoleSessionLocation(roleID string, sessionID string) SessionLocation {
	return SessionLocation{Kind: SessionLocationRole, RoleID: strings.TrimSpace(roleID), SessionID: strings.TrimSpace(sessionID)}
}

func GroupSessionLocation(groupID string, sessionID string) SessionLocation {
	return SessionLocation{Kind: SessionLocationGroup, GroupID: strings.TrimSpace(groupID), SessionID: strings.TrimSpace(sessionID)}
}

func WorkspaceSessionLocation(workspaceID string, roleID string, sessionID string) SessionLocation {
	return SessionLocation{Kind: SessionLocationWorkspace, WorkspaceID: strings.TrimSpace(workspaceID), RoleID: strings.TrimSpace(roleID), SessionID: strings.TrimSpace(sessionID)}
}

// SessionLocationFromSession 从会话事实派生完整定位；形态与字段互斥关系在此统一判定。
func SessionLocationFromSession(session Session) (SessionLocation, error) {
	sessionID := strings.TrimSpace(session.ID)
	roleID := strings.TrimSpace(session.RoleID)
	groupID := strings.TrimSpace(session.GroupID)
	workspaceID := strings.TrimSpace(session.WorkspaceID)
	switch {
	case groupID != "":
		if roleID != "" || workspaceID != "" {
			return SessionLocation{}, fmt.Errorf("session cannot belong to both group and role or workspace")
		}
		location := GroupSessionLocation(groupID, sessionID)
		return location, location.Validate()
	case workspaceID != "":
		location := WorkspaceSessionLocation(workspaceID, roleID, sessionID)
		return location, location.Validate()
	default:
		location := RoleSessionLocation(roleID, sessionID)
		return location, location.Validate()
	}
}

// Validate 要求定位形态与字段完全匹配：不缺字段，也不夹带其他形态的字段。
func (location SessionLocation) Validate() error {
	roleID := strings.TrimSpace(location.RoleID)
	groupID := strings.TrimSpace(location.GroupID)
	workspaceID := strings.TrimSpace(location.WorkspaceID)
	sessionID := strings.TrimSpace(location.SessionID)
	if sessionID == "" {
		return fmt.Errorf("session location requires session id")
	}
	switch location.Kind {
	case SessionLocationRole:
		if roleID == "" {
			return fmt.Errorf("role session location requires role id")
		}
		if groupID != "" || workspaceID != "" {
			return fmt.Errorf("role session location cannot carry group or workspace id")
		}
	case SessionLocationGroup:
		if groupID == "" {
			return fmt.Errorf("group session location requires group id")
		}
		if roleID != "" || workspaceID != "" {
			return fmt.Errorf("group session location cannot carry role or workspace id")
		}
	case SessionLocationWorkspace:
		if workspaceID == "" {
			return fmt.Errorf("workspace session location requires workspace id")
		}
		if roleID == "" {
			return fmt.Errorf("workspace session location requires role id")
		}
		if groupID != "" {
			return fmt.Errorf("workspace session location cannot carry group id")
		}
	default:
		return fmt.Errorf("session location kind is invalid: %s", location.Kind)
	}
	return nil
}

// SessionChangeType 覆盖会话写入的五类变更。
type SessionChangeType string

const (
	SessionChangeMessageAdded          SessionChangeType = "message_added"
	SessionChangeMessageDeleted        SessionChangeType = "message_deleted"
	SessionChangeMessageSubtreeDeleted SessionChangeType = "message_subtree_deleted"
	SessionChangeMessageEdited         SessionChangeType = "message_edited"
	SessionChangeSessionPersisted      SessionChangeType = "session_persisted"
)

// TargetKind 是变更类型唯一对应的目标类别。
func (changeType SessionChangeType) TargetKind() SessionChangeTargetKind {
	switch changeType {
	case SessionChangeMessageAdded, SessionChangeMessageDeleted, SessionChangeMessageSubtreeDeleted, SessionChangeMessageEdited:
		return SessionChangeTargetMessage
	case SessionChangeSessionPersisted:
		return SessionChangeTargetSession
	default:
		return ""
	}
}

// SessionChangeEvent 是会话变更事件的固定五项契约。
type SessionChangeEvent struct {
	TargetKind SessionChangeTargetKind `json:"targetKind"`
	Location   SessionLocation         `json:"location"`
	ChangeType SessionChangeType       `json:"changeType"`
	Version    uint64                  `json:"version"`
	OccurredAt time.Time               `json:"occurredAt"`
}

func (event SessionChangeEvent) Validate() error {
	switch event.TargetKind {
	case SessionChangeTargetSession, SessionChangeTargetMessage:
	default:
		return fmt.Errorf("session change target kind is invalid: %s", event.TargetKind)
	}
	if err := event.Location.Validate(); err != nil {
		return err
	}
	wantTarget := event.ChangeType.TargetKind()
	if wantTarget == "" {
		return fmt.Errorf("session change type is invalid: %s", event.ChangeType)
	}
	if event.TargetKind != wantTarget {
		return fmt.Errorf("session change type %s targets %s, not %s", event.ChangeType, wantTarget, event.TargetKind)
	}
	if event.Version == 0 {
		return fmt.Errorf("session change version is required")
	}
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("session change occurrence time is required")
	}
	return nil
}

// MarshalSessionChangeEvent 校验后编码，保证写出的事件满足完整契约。
func MarshalSessionChangeEvent(event SessionChangeEvent) ([]byte, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

// UnmarshalSessionChangeEvent 解码后校验，残缺或矛盾的事件不会进入调用方。
func UnmarshalSessionChangeEvent(payload []byte) (SessionChangeEvent, error) {
	var event SessionChangeEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return SessionChangeEvent{}, err
	}
	if err := event.Validate(); err != nil {
		return SessionChangeEvent{}, err
	}
	return event, nil
}
