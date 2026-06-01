package roleprompt

import "context"

func (s *system) SaveRoleAvatar(ctx context.Context, roleID string, dataURL string) error {
	return s.storage.SaveRoleAvatar(ctx, roleID, dataURL)
}

func (s *system) LoadRoleAvatar(ctx context.Context, roleID string) (string, error) {
	return s.storage.LoadRoleAvatar(ctx, roleID)
}

func (s *system) DeleteRoleAvatar(ctx context.Context, roleID string) error {
	return s.storage.DeleteRoleAvatar(ctx, roleID)
}
