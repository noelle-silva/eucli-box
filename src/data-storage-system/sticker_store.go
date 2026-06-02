package datastorage

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
	"eucli-box/pkg/utils"
)

var stickerAllowedImageMIMEs = map[string]string{"image/png": "png", "image/jpeg": "jpg", "image/webp": "webp", "image/gif": "gif"}

type stickerCategoryIndex struct {
	Name      string              `json:"name"`
	Items     []types.StickerItem `json:"items"`
	UpdatedAt time.Time           `json:"updatedAt"`
}

func (s *system) CreateStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error) {
	name, err := cleanStickerCategoryName(categoryName)
	if err != nil {
		return types.StickerCategory{}, err
	}
	dir, err := s.paths.stickerCategoryDir(name)
	if err != nil {
		return types.StickerCategory{}, err
	}
	if _, err := os.Stat(dir); err == nil {
		return types.StickerCategory{}, storageInvalid("sticker category already exists", nil)
	} else if !errors.Is(err, os.ErrNotExist) {
		return types.StickerCategory{}, storageReadFailed("failed to inspect sticker category", err)
	}
	if err := ensureDirs(dir); err != nil {
		return types.StickerCategory{}, storageWriteFailed("failed to create sticker category", err)
	}
	now := time.Now().UTC()
	category := types.StickerCategory{Name: name, Items: []types.StickerItem{}, UpdatedAt: now}
	if err := writeStickerCategoryIndex(ctx, dir, category); err != nil {
		return types.StickerCategory{}, err
	}
	if err := s.rebuildStickerIndexes(ctx); err != nil {
		return types.StickerCategory{}, err
	}
	return category, nil
}

func (s *system) ListStickerCategories(ctx context.Context) ([]types.StickerCategorySummary, error) {
	entries, err := os.ReadDir(s.paths.stickersRoot())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, storageReadFailed("failed to scan sticker categories", err)
	}
	summaries := make([]types.StickerCategorySummary, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, storageReadFailed("scan cancelled", err)
		}
		if !entry.IsDir() {
			continue
		}
		name, err := cleanStickerCategoryName(entry.Name())
		if err != nil {
			continue
		}
		category, err := s.LoadStickerCategory(ctx, name)
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, types.StickerCategorySummary{Name: name, Count: len(category.Items), UpdatedAt: category.UpdatedAt})
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Name < summaries[j].Name })
	return summaries, nil
}

func (s *system) LoadStickerCategory(ctx context.Context, categoryName string) (types.StickerCategory, error) {
	name, dir, err := s.requireStickerCategory(categoryName)
	if err != nil {
		return types.StickerCategory{}, err
	}
	items, err := readStickerItems(ctx, dir)
	if err != nil {
		return types.StickerCategory{}, err
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	updatedAt := time.Time{}
	if dataFileExists(filepath.Join(dir, "index.json")) {
		index, err := readJSON[stickerCategoryIndex](ctx, filepath.Join(dir, "index.json"))
		if err == nil {
			updatedAt = index.UpdatedAt
		}
	}
	for _, item := range items {
		if item.UpdatedAt.After(updatedAt) {
			updatedAt = item.UpdatedAt
		}
	}
	if updatedAt.IsZero() {
		updatedAt = time.Now().UTC()
	}
	return types.StickerCategory{Name: name, Items: items, UpdatedAt: updatedAt}, nil
}

func (s *system) LoadStickerLibrary(ctx context.Context) (types.StickerLibrary, error) {
	summaries, err := s.ListStickerCategories(ctx)
	if err != nil {
		return types.StickerLibrary{}, err
	}
	library := types.StickerLibrary{Categories: summaries, Map: map[string][]types.StickerItem{}}
	for _, summary := range summaries {
		category, err := s.LoadStickerCategory(ctx, summary.Name)
		if err != nil {
			return types.StickerLibrary{}, err
		}
		library.Map[summary.Name] = category.Items
		if summary.UpdatedAt.After(library.UpdatedAt) {
			library.UpdatedAt = summary.UpdatedAt
		}
	}
	if library.UpdatedAt.IsZero() {
		library.UpdatedAt = time.Now().UTC()
	}
	return library, nil
}

func (s *system) AddSticker(ctx context.Context, categoryName string, stickerName string, dataURL string) (types.StickerItem, error) {
	name, dir, err := s.requireStickerCategory(categoryName)
	if err != nil {
		return types.StickerItem{}, err
	}
	stickerName, err = cleanStickerName(stickerName)
	if err != nil {
		return types.StickerItem{}, err
	}
	if _, err := s.stickerByName(ctx, name, stickerName); err == nil {
		return types.StickerItem{}, storageInvalid("sticker name already exists", nil)
	} else if !isStorageNotFound(err) {
		return types.StickerItem{}, err
	}
	image, err := decodeImageDataURL(dataURL, stickerAllowedImageMIMEs)
	if err != nil {
		return types.StickerItem{}, storageInvalid("sticker image must be a png, jpg, webp, or gif data url", err)
	}
	stickerID := utils.NewID("sticker")
	itemDir, err := s.paths.stickerItemDir(name, stickerID)
	if err != nil {
		return types.StickerItem{}, err
	}
	if err := ensureDirs(itemDir); err != nil {
		return types.StickerItem{}, storageWriteFailed("failed to create sticker item directory", err)
	}
	imageName := "image." + image.Ext
	if err := writeStoredImageFile(ctx, itemDir, imageName, image); err != nil {
		_ = os.RemoveAll(itemDir)
		return types.StickerItem{}, storageWriteFailed("failed to write sticker image", err)
	}
	now := time.Now().UTC()
	item := types.StickerItem{ID: stickerID, Name: stickerName, RelPath: filepath.ToSlash(filepath.Join("stickers", name, stickerID, imageName)), CreatedAt: now, UpdatedAt: now}
	if err := writeJSON(ctx, filepath.Join(itemDir, "data.json"), item); err != nil {
		_ = os.RemoveAll(itemDir)
		return types.StickerItem{}, err
	}
	if err := s.rebuildStickerCategoryIndex(ctx, name, dir); err != nil {
		return types.StickerItem{}, err
	}
	if err := s.rebuildStickerIndexes(ctx); err != nil {
		return types.StickerItem{}, err
	}
	return item, nil
}

func (s *system) RenameSticker(ctx context.Context, categoryName string, oldStickerName string, newStickerName string) (types.StickerItem, error) {
	name, dir, err := s.requireStickerCategory(categoryName)
	if err != nil {
		return types.StickerItem{}, err
	}
	oldStickerName, err = cleanStickerName(oldStickerName)
	if err != nil {
		return types.StickerItem{}, err
	}
	newStickerName, err = cleanStickerName(newStickerName)
	if err != nil {
		return types.StickerItem{}, err
	}
	if oldStickerName == newStickerName {
		return types.StickerItem{}, storageInvalid("sticker name is unchanged", nil)
	}
	if _, err := s.stickerByName(ctx, name, newStickerName); err == nil {
		return types.StickerItem{}, storageInvalid("sticker name already exists", nil)
	} else if !isStorageNotFound(err) {
		return types.StickerItem{}, err
	}
	item, err := s.stickerByName(ctx, name, oldStickerName)
	if err != nil {
		return types.StickerItem{}, err
	}
	item.Name = newStickerName
	item.UpdatedAt = time.Now().UTC()
	target, err := s.paths.stickerItemDataFile(name, item.ID)
	if err != nil {
		return types.StickerItem{}, err
	}
	if err := writeJSON(ctx, target, item); err != nil {
		return types.StickerItem{}, err
	}
	if err := s.rebuildStickerCategoryIndex(ctx, name, dir); err != nil {
		return types.StickerItem{}, err
	}
	if err := s.rebuildStickerIndexes(ctx); err != nil {
		return types.StickerItem{}, err
	}
	return item, nil
}

func (s *system) DeleteSticker(ctx context.Context, categoryName string, stickerName string) error {
	name, dir, err := s.requireStickerCategory(categoryName)
	if err != nil {
		return err
	}
	stickerName, err = cleanStickerName(stickerName)
	if err != nil {
		return err
	}
	item, err := s.stickerByName(ctx, name, stickerName)
	if err != nil {
		return err
	}
	itemDir, err := s.paths.stickerItemDir(name, item.ID)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemSticker, item.ID, itemDir); err != nil {
		return err
	}
	if err := s.rebuildStickerCategoryIndex(ctx, name, dir); err != nil {
		return err
	}
	return s.rebuildStickerIndexes(ctx)
}

func (s *system) DeleteStickerCategory(ctx context.Context, categoryName string) error {
	name, dir, err := s.requireStickerCategory(categoryName)
	if err != nil {
		return err
	}
	if err := moveToRecycle(ctx, s.paths, types.StorageItemStickerCategory, name, dir); err != nil {
		return err
	}
	return s.rebuildStickerIndexes(ctx)
}

func (s *system) LoadStickerImage(ctx context.Context, relPath string) (string, error) {
	imagePath, mime, err := s.stickerImagePath(relPath)
	if err != nil {
		return "", err
	}
	payload, err := os.ReadFile(imagePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", storageReadFailed("sticker image does not exist", err)
		}
		return "", storageReadFailed("failed to read sticker image", err)
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *system) stickerByName(ctx context.Context, categoryName string, stickerName string) (types.StickerItem, error) {
	category, err := s.LoadStickerCategory(ctx, categoryName)
	if err != nil {
		return types.StickerItem{}, err
	}
	for _, item := range category.Items {
		if item.Name == stickerName {
			return item, nil
		}
	}
	return types.StickerItem{}, storageNotFound("sticker does not exist", nil)
}

func (s *system) requireStickerCategory(categoryName string) (string, string, error) {
	name, err := cleanStickerCategoryName(categoryName)
	if err != nil {
		return "", "", err
	}
	dir, err := s.paths.stickerCategoryDir(name)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", "", storageNotFound("sticker category does not exist", err)
		}
		return "", "", storageReadFailed("failed to inspect sticker category", err)
	}
	if !info.IsDir() {
		return "", "", storageInvalid("sticker category path is not a directory", nil)
	}
	return name, dir, nil
}

func (s *system) stickerImagePath(relPath string) (string, string, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	parts := strings.Split(relPath, "/")
	if len(parts) != 4 || parts[0] != "stickers" {
		return "", "", storageInvalid("sticker image path is invalid", nil)
	}
	categoryName, err := cleanStickerCategoryName(parts[1])
	if err != nil {
		return "", "", err
	}
	stickerID, err := cleanID(parts[2])
	if err != nil {
		return "", "", err
	}
	fileName := strings.TrimSpace(parts[3])
	if !strings.HasPrefix(fileName, "image.") {
		return "", "", storageInvalid("sticker image filename is invalid", nil)
	}
	mime, ok := imageMIMEFromExt(filepath.Ext(fileName))
	if !ok {
		return "", "", storageInvalid("sticker image extension is unsupported", nil)
	}
	itemDir, err := s.paths.stickerItemDir(categoryName, stickerID)
	if err != nil {
		return "", "", err
	}
	joined := filepath.Join(itemDir, fileName)
	if !isWithin(itemDir, joined) {
		return "", "", storageInvalid("path escapes sticker item directory", nil)
	}
	return joined, mime, nil
}

func readStickerItems(ctx context.Context, categoryDir string) ([]types.StickerItem, error) {
	entries, err := os.ReadDir(categoryDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, storageNotFound("sticker category does not exist", err)
		}
		return nil, storageReadFailed("failed to scan sticker category", err)
	}
	items := make([]types.StickerItem, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, storageReadFailed("scan cancelled", err)
		}
		if !entry.IsDir() {
			continue
		}
		dataFile := filepath.Join(categoryDir, entry.Name(), "data.json")
		if !dataFileExists(dataFile) {
			continue
		}
		item, err := readJSON[types.StickerItem](ctx, dataFile)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *system) rebuildStickerIndexes(ctx context.Context) error {
	categories, err := s.ListStickerCategories(ctx)
	if err != nil {
		return err
	}
	for _, category := range categories {
		dir, err := s.paths.stickerCategoryDir(category.Name)
		if err != nil {
			return err
		}
		if err := s.rebuildStickerCategoryIndex(ctx, category.Name, dir); err != nil {
			return err
		}
	}
	return writeIndex(ctx, filepath.Join(s.paths.stickersRoot(), "index.json"), rootIndex[types.StickerCategorySummary]{Items: categories})
}

func (s *system) rebuildStickerCategoryIndex(ctx context.Context, categoryName string, dir string) error {
	category, err := s.LoadStickerCategory(ctx, categoryName)
	if err != nil {
		return err
	}
	return writeStickerCategoryIndex(ctx, dir, category)
}

func writeStickerCategoryIndex(ctx context.Context, dir string, category types.StickerCategory) error {
	return writeIndex(ctx, filepath.Join(dir, "index.json"), stickerCategoryIndex{Name: category.Name, Items: category.Items, UpdatedAt: category.UpdatedAt})
}

func writeStoredImageFile(ctx context.Context, dir string, fileName string, image storedImage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target := filepath.Join(dir, fileName)
	tmp, err := os.CreateTemp(dir, ".image-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(image.Payload); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := removeStickerImageFiles(dir, filepath.Base(tmpName)); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func removeStickerImageFiles(dir string, preserveFile string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == preserveFile {
			continue
		}
		if strings.HasPrefix(name, "image.") || strings.HasPrefix(name, ".image-") {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func cleanStickerCategoryName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", storageInvalid("sticker category name is required", nil)
	}
	if len([]rune(name)) > 60 {
		return "", storageInvalid("sticker category name is too long", nil)
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\\<>:"|?*`) || hasControlRune(name) || strings.HasSuffix(name, ".") {
		return "", storageInvalid("sticker category name contains unsafe path characters", nil)
	}
	if isWindowsReservedName(name) {
		return "", storageInvalid("sticker category name is reserved", nil)
	}
	return name, nil
}

func cleanStickerName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", storageInvalid("sticker name is required", nil)
	}
	if len([]rune(name)) > 80 {
		return "", storageInvalid("sticker name is too long", nil)
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "]") || strings.ContainsAny(name, "\n\r") {
		return "", storageInvalid("sticker name contains unsupported characters", nil)
	}
	return name, nil
}

func hasControlRune(value string) bool {
	for _, r := range value {
		if r >= 0 && r < 32 {
			return true
		}
	}
	return false
}

func isWindowsReservedName(value string) bool {
	upper := strings.ToUpper(strings.TrimSpace(value))
	if base, _, ok := strings.Cut(upper, "."); ok {
		upper = base
	}
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return true
	}
	return len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9'
}

func isStorageNotFound(err error) bool {
	var appErr *apperrors.AppError
	return errors.As(err, &appErr) && appErr.Code == "storage.not_found"
}
