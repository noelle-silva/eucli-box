package systemplugin

import (
	"strings"

	"eucli-box/pkg/systemplugin"
)

// pluginIndex 是发现结果的内存索引：能力到提供者、占位符接口到所属插件。
// 它不落盘、不参与持久化，每次发现从清单现场派生，是清单的影子而非新的事实源。
type pluginIndex struct {
	records             []pluginRecord
	byID                map[string]int
	placeholderOwners   map[string]int
	capabilityProviders map[string][]int
}

func buildPluginIndex(records []pluginRecord) *pluginIndex {
	index := &pluginIndex{
		records:             records,
		byID:                map[string]int{},
		placeholderOwners:   map[string]int{},
		capabilityProviders: map[string][]int{},
	}
	for position, record := range records {
		pluginID := record.locatorID()
		if pluginID != "" {
			if _, exists := index.byID[pluginID]; !exists {
				index.byID[pluginID] = position
			}
		}
		for _, capability := range record.manifest.Capabilities {
			index.capabilityProviders[capability.Type] = append(index.capabilityProviders[capability.Type], position)
			if capability.Type != systemplugin.CapabilityPlaceholderValues {
				continue
			}
			for _, item := range capability.Interfaces {
				key := placeholderOwnerKey(pluginID, item.ID)
				if _, exists := index.placeholderOwners[key]; !exists {
					index.placeholderOwners[key] = position
				}
			}
		}
	}
	return index
}

func placeholderOwnerKey(pluginID string, interfaceID string) string {
	return strings.TrimSpace(pluginID) + "\x00" + strings.TrimSpace(interfaceID)
}

func (i *pluginIndex) find(pluginID string) (pluginRecord, bool) {
	pluginID = strings.TrimSpace(pluginID)
	if pluginID == "" {
		return pluginRecord{}, false
	}
	position, ok := i.byID[pluginID]
	if !ok {
		return pluginRecord{}, false
	}
	return i.records[position], true
}

// placeholderOwner 返回某个占位符接口的所属插件；不存在时返回 false。
func (i *pluginIndex) placeholderOwner(pluginID string, interfaceID string) (pluginRecord, bool) {
	position, ok := i.placeholderOwners[placeholderOwnerKey(pluginID, interfaceID)]
	if !ok {
		return pluginRecord{}, false
	}
	return i.records[position], true
}
