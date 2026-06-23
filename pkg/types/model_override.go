package types

import "strings"

const (
	SessionMetadataReasoningEffort         = "reasoningEffort"
	SessionMetadataModelOverrideKind       = "modelOverride.kind"
	SessionMetadataModelOverrideProviderID = "modelOverride.providerId"
	SessionMetadataModelOverrideGroupID    = "modelOverride.groupId"
	SessionMetadataModelOverrideModelID    = "modelOverride.modelId"
	ModelCoordinateKindProvider            = "provider"
	ModelCoordinateKindGroup               = "model_group"
)

func NormalizeModelOverrideCoordinate(coordinate ModelCoordinate) (ModelCoordinate, bool) {
	kind := strings.TrimSpace(coordinate.Kind)
	providerID := strings.TrimSpace(coordinate.ProviderID)
	groupID := strings.TrimSpace(coordinate.GroupID)
	modelID := strings.TrimSpace(coordinate.ModelID)
	if modelID == "" {
		return ModelCoordinate{}, false
	}
	if kind == ModelCoordinateKindGroup || groupID != "" {
		if groupID == "" {
			return ModelCoordinate{}, false
		}
		return ModelCoordinate{Kind: ModelCoordinateKindGroup, GroupID: groupID, ModelID: modelID}, true
	}
	if providerID == "" {
		return ModelCoordinate{}, false
	}
	return ModelCoordinate{Kind: ModelCoordinateKindProvider, ProviderID: providerID, ModelID: modelID}, true
}

func HasCompleteModelCoordinate(coordinate ModelCoordinate) bool {
	if strings.TrimSpace(coordinate.ModelID) == "" {
		return false
	}
	if strings.TrimSpace(coordinate.Kind) == ModelCoordinateKindGroup || strings.TrimSpace(coordinate.GroupID) != "" {
		return strings.TrimSpace(coordinate.GroupID) != ""
	}
	return strings.TrimSpace(coordinate.ProviderID) != "" || strings.TrimSpace(coordinate.ProviderName) != ""
}

func ModelOverrideFromSessionMetadata(metadata map[string]string) (ModelCoordinate, bool) {
	if len(metadata) == 0 {
		return ModelCoordinate{}, false
	}
	return NormalizeModelOverrideCoordinate(ModelCoordinate{
		Kind:       metadata[SessionMetadataModelOverrideKind],
		ProviderID: metadata[SessionMetadataModelOverrideProviderID],
		GroupID:    metadata[SessionMetadataModelOverrideGroupID],
		ModelID:    metadata[SessionMetadataModelOverrideModelID],
	})
}

func IsSessionModelOverrideMetadataKey(key string) bool {
	switch strings.TrimSpace(key) {
	case SessionMetadataModelOverrideKind,
		SessionMetadataModelOverrideProviderID,
		SessionMetadataModelOverrideGroupID,
		SessionMetadataModelOverrideModelID:
		return true
	default:
		return false
	}
}

func PutModelOverrideSessionMetadata(metadata map[string]string, coordinate ModelCoordinate) map[string]string {
	normalized, ok := NormalizeModelOverrideCoordinate(coordinate)
	if !ok {
		return ClearModelOverrideSessionMetadata(metadata)
	}
	out := copySessionMetadata(metadata)
	out[SessionMetadataModelOverrideKind] = normalized.Kind
	out[SessionMetadataModelOverrideProviderID] = normalized.ProviderID
	out[SessionMetadataModelOverrideGroupID] = normalized.GroupID
	out[SessionMetadataModelOverrideModelID] = normalized.ModelID
	return out
}

func ClearModelOverrideSessionMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	out := copySessionMetadata(metadata)
	delete(out, SessionMetadataModelOverrideKind)
	delete(out, SessionMetadataModelOverrideProviderID)
	delete(out, SessionMetadataModelOverrideGroupID)
	delete(out, SessionMetadataModelOverrideModelID)
	if len(out) == 0 {
		return nil
	}
	return out
}

func ModelOverrideSessionMetadataPatch(coordinate ModelCoordinate) map[string]string {
	normalized, ok := NormalizeModelOverrideCoordinate(coordinate)
	if !ok {
		return nil
	}
	return map[string]string{
		SessionMetadataModelOverrideKind:       normalized.Kind,
		SessionMetadataModelOverrideProviderID: normalized.ProviderID,
		SessionMetadataModelOverrideGroupID:    normalized.GroupID,
		SessionMetadataModelOverrideModelID:    normalized.ModelID,
	}
}

func SameModelOverrideCoordinate(left ModelCoordinate, right ModelCoordinate) bool {
	leftNormalized, leftOK := NormalizeModelOverrideCoordinate(left)
	rightNormalized, rightOK := NormalizeModelOverrideCoordinate(right)
	if leftOK != rightOK {
		return false
	}
	if !leftOK {
		return true
	}
	return leftNormalized.Kind == rightNormalized.Kind &&
		leftNormalized.ProviderID == rightNormalized.ProviderID &&
		leftNormalized.GroupID == rightNormalized.GroupID &&
		leftNormalized.ModelID == rightNormalized.ModelID
}

func copySessionMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}
