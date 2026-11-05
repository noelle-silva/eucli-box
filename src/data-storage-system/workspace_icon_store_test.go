package datastorage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"eucli-box/pkg/types"
)

const testIconPNGDataURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lz2YNgAAAABJRU5ErkJggg=="

const testIconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#c2410c" d="M4 4h16v16H4z"/></svg>`

func TestWorkspaceIconSVGAndImageReplaceEachOther(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	workspace := types.Workspace{ID: "workspace-icon", Name: "带图标的工作区"}
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	iconDir := filepath.Join(system.paths.root, "workspaces", workspace.ID, "attachments", "icon")

	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: testIconSVG}); err != nil {
		t.Fatalf("SaveWorkspaceIcon(svg) error = %v", err)
	}
	assertFile(t, filepath.Join(iconDir, "icon.json"))
	content, err := system.LoadWorkspaceIcon(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("LoadWorkspaceIcon(svg) error = %v", err)
	}
	if content.Kind != types.WorkspaceIconKindSVG || content.SVG != testIconSVG || content.DataURL != "" {
		t.Fatalf("loaded svg icon = %#v", content)
	}

	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindImage, DataURL: testIconPNGDataURL}); err != nil {
		t.Fatalf("SaveWorkspaceIcon(image) error = %v", err)
	}
	assertFile(t, filepath.Join(iconDir, "image.png"))
	content, err = system.LoadWorkspaceIcon(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("LoadWorkspaceIcon(image) error = %v", err)
	}
	if content.Kind != types.WorkspaceIconKindImage || content.DataURL != testIconPNGDataURL || content.SVG != "" {
		t.Fatalf("loaded image icon = %#v", content)
	}

	// 切回 SVG 时旧图像文件不留在原地：一个工作区只对应一个载体。
	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: testIconSVG}); err != nil {
		t.Fatalf("SaveWorkspaceIcon(svg again) error = %v", err)
	}
	assertNoFile(t, filepath.Join(iconDir, "image.png"))

	if err := system.DeleteWorkspaceIcon(ctx, workspace.ID); err != nil {
		t.Fatalf("DeleteWorkspaceIcon() error = %v", err)
	}
	assertNoFile(t, filepath.Join(iconDir, "icon.json"))
	if _, err := os.Stat(iconDir); !os.IsNotExist(err) {
		t.Fatalf("icon directory still exists err=%v", err)
	}
	if _, err := system.LoadWorkspaceIcon(ctx, workspace.ID); err == nil {
		t.Fatal("LoadWorkspaceIcon() after delete = nil error")
	} else {
		assertAppErrorCode(t, err, "storage.not_found")
	}
}

func TestWorkspaceIconKeepsSavedWorkspaceConfigIntact(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	workspace := types.Workspace{ID: "workspace-icon-kept", Name: "配置与图标解耦"}
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: testIconSVG}); err != nil {
		t.Fatalf("SaveWorkspaceIcon() error = %v", err)
	}

	// 全量保存工作区（客户端目前的形态）不得带走图标。
	workspace.Name = "改名后的工作区"
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace(rename) error = %v", err)
	}
	content, err := system.LoadWorkspaceIcon(ctx, workspace.ID)
	if err != nil {
		t.Fatalf("LoadWorkspaceIcon() after workspace save error = %v", err)
	}
	if content.SVG != testIconSVG {
		t.Fatalf("icon after workspace save = %#v", content)
	}
}

func TestWorkspaceIconRejectsInvalidSubmission(t *testing.T) {
	system := newTestSystem(t)
	ctx := context.Background()
	workspace := types.Workspace{ID: "workspace-icon-invalid", Name: "非法图标"}
	if err := system.SaveWorkspace(ctx, workspace); err != nil {
		t.Fatalf("SaveWorkspace() error = %v", err)
	}
	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: `<svg><script>alert(1)</script></svg>`}); err == nil {
		t.Fatal("SaveWorkspaceIcon(script svg) = nil error")
	} else {
		assertAppErrorCode(t, err, "storage.invalid_request")
	}
	if err := system.SaveWorkspaceIcon(ctx, workspace.ID, types.WorkspaceIconContent{Kind: types.WorkspaceIconKindImage, DataURL: "data:image/gif;base64,R0lGODlhAQABAAAAACw="}); err == nil {
		t.Fatal("SaveWorkspaceIcon(gif) = nil error")
	} else {
		assertAppErrorCode(t, err, "storage.invalid_request")
	}
	if _, err := system.LoadWorkspaceIcon(ctx, workspace.ID); err == nil {
		t.Fatal("LoadWorkspaceIcon() with no icon = nil error")
	} else {
		assertAppErrorCode(t, err, "storage.not_found")
	}
	if err := system.SaveWorkspaceIcon(ctx, "workspace-missing", types.WorkspaceIconContent{Kind: types.WorkspaceIconKindSVG, SVG: testIconSVG}); err == nil {
		t.Fatal("SaveWorkspaceIcon(missing workspace) = nil error")
	} else {
		assertAppErrorCode(t, err, "storage.not_found")
	}
}
