package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type boxCatalogSnapshot struct {
	Roles     []map[string]any `json:"roles"`
	Providers []map[string]any `json:"providers"`
}

func (svc *service) syncBoxCatalog(ctx context.Context) error {
	_, err := svc.fetchBoxCatalog(ctx)
	return err
}

func (svc *service) fetchBoxCatalog(ctx context.Context) (boxCatalogSnapshot, error) {
	var errs []string
	snapshot := boxCatalogSnapshot{Roles: []map[string]any{}, Providers: []map[string]any{}}

	roles, err := svc.box.listRoles(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("sync eucli-box roles: %v", err))
	} else {
		for _, role := range roles {
			if clientRole := boxRoleToClient(role); clientRole != nil {
				snapshot.Roles = append(snapshot.Roles, clientRole)
			}
		}
	}

	providers, err := svc.box.listProviders(ctx)
	if err != nil {
		errs = append(errs, fmt.Sprintf("sync eucli-box providers: %v", err))
	} else {
		for _, provider := range providers {
			if clientProvider := boxProviderToClient(provider); clientProvider != nil {
				snapshot.Providers = append(snapshot.Providers, clientProvider)
			}
		}
	}

	if len(errs) > 0 {
		return snapshot, fmt.Errorf("eucli-box 目录同步失败: %s", strings.Join(errs, "; "))
	}
	return snapshot, nil
}

func boxRoleToClient(role map[string]any) map[string]any {
	roleID := strings.TrimSpace(asString(role["id"]))
	if roleID == "" {
		return nil
	}
	modelConfig := asMap(role["modelConfig"])
	return map[string]any{
		"id":           roleID,
		"name":         strings.TrimSpace(asString(role["name"])),
		"avatar":       strings.TrimSpace(asString(role["avatar"])),
		"description":  strings.TrimSpace(asString(role["description"])),
		"systemPrompt": strings.TrimSpace(boxFirstPromptText(role)),
		"temperature":  asFloat64(modelConfig["temperature"], 0.7),
		"modelRef": map[string]any{
			"providerId": strings.TrimSpace(asString(modelConfig["providerId"])),
			"modelId":    strings.TrimSpace(asString(modelConfig["model"])),
		},
		"createdAt": boxTimeOrDefault(role, "createdAt"),
		"updatedAt": boxTimeOrDefault(role, "updatedAt"),
	}
}

func boxProviderToClient(provider map[string]any) map[string]any {
	providerID := strings.TrimSpace(asString(provider["id"]))
	if providerID == "" {
		return nil
	}
	return map[string]any{
		"id":       providerID,
		"name":     strings.TrimSpace(asString(provider["name"])),
		"baseUrl":  strings.TrimSpace(asString(provider["baseUrl"])),
		"apiKey":   "",
		"protocol": strings.TrimSpace(asString(provider["protocol"])),
		"modelsCache": map[string]any{
			"items":     boxModels(provider["models"]),
			"fetchedAt": nowMs(),
		},
		"createdAt": boxTimeOrDefault(provider, "createdAt"),
		"updatedAt": boxTimeOrDefault(provider, "updatedAt"),
	}
}

func boxFirstPromptText(role map[string]any) string {
	prompts := asSlice(role["prompts"])
	if len(prompts) == 0 {
		return strings.TrimSpace(asString(role["description"]))
	}
	var texts []string
	for _, p := range prompts {
		text := strings.TrimSpace(asString(asMap(p)["content"]))
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n")
}

func boxModels(raw any) []map[string]any {
	list := asSlice(raw)
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		model := asMap(item)
		out = append(out, map[string]any{
			"id":   strings.TrimSpace(asString(model["id"])),
			"name": strings.TrimSpace(asString(model["name"])),
		})
	}
	return out
}

func boxTimeOrDefault(obj map[string]any, key string) int64 {
	if raw, ok := obj[key]; ok {
		if t, err := parseBoxTime(raw); err == nil && t > 0 {
			return t
		}
	}
	return nowMs()
}

func parseBoxTime(raw any) (int64, error) {
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case string:
		t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v))
		if err != nil {
			return 0, err
		}
		return t.UnixMilli(), nil
	default:
		return 0, fmt.Errorf("unsupported time type %T", raw)
	}
}
