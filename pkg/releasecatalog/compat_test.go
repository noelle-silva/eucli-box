package releasecatalog

import (
	"context"
	"testing"

	"eucli-box/pkg/types"
)

// TestLegacySignaturesStayCallable 钉住三个旧签名导出函数。
// 删除它们或改动签名都会让本包测试无法编译，从而在提交前拦住破坏既有调用方的变更。
// 旧入口的移除条件写在各自注释里：随下一个主版本统一移除。
func TestLegacySignaturesStayCallable(t *testing.T) {
	var _ func(string, types.OfficialReleaseSource) (string, error) = IndexRawURL
	var _ func(context.Context, HTTPDoer, types.OfficialReleaseSource, string) (Index, error) = ReadIndex
	var _ func(string, types.OfficialReleaseSource, IndexPackage) (string, error) = DownloadURL
}

// TestLegacyEntriesMatchCurrentEntries 验证旧入口与新入口产出完全一致，转发层不会漂移。
func TestLegacyEntriesMatchCurrentEntries(t *testing.T) {
	source := types.OfficialReleaseSource{Repository: "https://github.com/owner/repo", Ref: "main"}
	repository, err := ParseRepository(source.Repository)
	if err != nil {
		t.Fatalf("ParseRepository() error = %v", err)
	}
	legacyURL, err := IndexRawURL("", source)
	if err != nil {
		t.Fatalf("IndexRawURL() error = %v", err)
	}
	currentURL, err := RepositoryIndexURL("", repository, source.Ref)
	if err != nil {
		t.Fatalf("RepositoryIndexURL() error = %v", err)
	}
	if legacyURL != currentURL {
		t.Fatalf("index url = %q, want %q", legacyURL, currentURL)
	}
	pkg := IndexPackage{ReleaseTag: "demo/v0.1.0", FileName: "tool-demo_0.1.0_windows-x64.zip"}
	legacyDownload, err := DownloadURL("", source, pkg)
	if err != nil {
		t.Fatalf("DownloadURL() error = %v", err)
	}
	currentDownload, err := ResolveDownloadURL("", source.Repository, "", pkg)
	if err != nil {
		t.Fatalf("ResolveDownloadURL() error = %v", err)
	}
	if legacyDownload != currentDownload {
		t.Fatalf("download url = %q, want %q", legacyDownload, currentDownload)
	}
}
