package systemplugin

import (
	"context"
	"strings"
	"time"

	"eucli-box/pkg/types"
)

func (s *system) AvailablePlaceholderInterfaces(ctx context.Context, library types.PlaceholderLibrary) ([]types.SystemPluginAvailablePlaceholderInterface, error) {
	records, err := s.discover(ctx)
	if err != nil {
		return nil, err
	}
	existing := map[string]struct{}{}
	for _, item := range library.Placeholders {
		if source := item.Source; source != nil && source.Kind == types.PlaceholderSourceSystemPlugin {
			existing[source.PluginID+"\x00"+source.InterfaceID] = struct{}{}
		}
	}
	out := []types.SystemPluginAvailablePlaceholderInterface{}
	for _, record := range records {
		for _, item := range record.manifest.PlaceholderInterfaces {
			key := record.manifest.ID + "\x00" + item.ID
			if _, ok := existing[key]; ok {
				continue
			}
			out = append(out, types.SystemPluginAvailablePlaceholderInterface{PluginID: record.manifest.ID, PluginName: record.manifest.Name, InterfaceID: item.ID, InterfaceDescription: item.Description, PlaceholderName: record.effectiveName(item)})
		}
	}
	return out, nil
}

func (s *system) CreatePlaceholderFromInterface(ctx context.Context, library types.PlaceholderLibrary, pluginID string, interfaceID string) (types.PlaceholderLibrary, error) {
	record, err := s.findRecord(ctx, pluginID)
	if err != nil {
		return types.PlaceholderLibrary{}, err
	}
	interfaceID = strings.TrimSpace(interfaceID)
	for _, item := range library.Placeholders {
		if source := item.Source; source != nil && source.Kind == types.PlaceholderSourceSystemPlugin && source.PluginID == record.manifest.ID && source.InterfaceID == interfaceID {
			return library, nil
		}
	}
	for _, item := range record.manifest.PlaceholderInterfaces {
		if item.ID != interfaceID {
			continue
		}
		library.Placeholders = append(library.Placeholders, types.PlaceholderItem{Name: record.effectiveName(item), Description: item.Description, Source: &types.PlaceholderSource{Kind: types.PlaceholderSourceSystemPlugin, PluginID: record.manifest.ID, InterfaceID: item.ID}, CreatedAt: time.Now().UTC()})
		return library, nil
	}
	return types.PlaceholderLibrary{}, pluginInvalid("system plugin placeholder interface was not found", nil)
}
