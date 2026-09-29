// Package hypercortexreader 是 hypercortex_reader 工具的业务动作：
// 通过 HyperCortex 的开放访问入口，以只读方式搜索与读取知识库内容。
package hypercortexreader

import (
	"context"
	"fmt"
	"strings"

	"eucli-box/pkg/types"
)

// Execute 执行一次工具动作。
func Execute(ctx context.Context, input types.ToolExecutionInput) types.ToolExecutionOutput {
	if err := ctx.Err(); err != nil {
		return failure("tool execution cancelled", err, nil)
	}
	action, err := stringArg(input, "action", true)
	if err != nil {
		return failure("parse hypercortex_reader request", err, nil)
	}
	action = strings.ToLower(action)
	switch action {
	case actionSearchNotes:
		return runSearchNotes(ctx, input)
	case actionReadNote:
		return runReadNote(ctx, input)
	case actionNoteRelations:
		return runNoteRelations(ctx, input)
	case actionListFavorites:
		return runListFavorites(ctx, input)
	case actionListRepos:
		return runListRepos(input)
	case actionSearchAssets:
		return runSearchAssets(ctx, input)
	case actionListAssets:
		return runListAssets(ctx, input)
	case actionListTrash:
		return runListTrash(ctx, input)
	default:
		return failure("parse hypercortex_reader request", fmt.Errorf("unsupported action %q", action), map[string]any{"action": action})
	}
}

const (
	actionSearchNotes   = "search_notes"
	actionReadNote      = "read_note"
	actionNoteRelations = "note_relations"
	actionListFavorites = "list_favorites"
	actionListRepos     = "list_repos"
	actionSearchAssets  = "search_assets"
	actionListAssets    = "list_assets"
	actionListTrash     = "list_trash"
)

// session 是一次动作执行的公共装配：解析后的仓库条目、对外客户端与输出上限。
type session struct {
	entry     repoEntry
	client    *rpcClient
	maxOutput int
}

// openSession 装配一次动作执行：装载配置、解析仓库并建立客户端；
// 任何一步失败都快速失败并说明修复位置。
func openSession(input types.ToolExecutionInput) (session, error) {
	config, err := loadConfig(input.ToolBodyDirectory)
	if err != nil {
		return session{}, err
	}
	repos, err := loadRepoConfig(input)
	if err != nil {
		return session{}, err
	}
	selector, err := stringArg(input, "repo", false)
	if err != nil {
		return session{}, err
	}
	entry, err := repos.resolve(selector)
	if err != nil {
		return session{}, err
	}
	client, err := newRPCClient(repos.Endpoint, entry.Key)
	if err != nil {
		return session{}, err
	}
	maxOutput, err := effectiveMaxOutputChars(input, config)
	if err != nil {
		return session{}, err
	}
	return session{entry: entry, client: client, maxOutput: maxOutput}, nil
}

// succeed 组合正文与信息条，并返回成功输出。
func (s session) succeed(action string, payload string, facts []resultFact, metadata map[string]any) types.ToolExecutionOutput {
	content, truncated := composeContent(payload, action, s.entry.ID, facts, s.maxOutput)
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["action"] = action
	metadata["repo"] = s.entry.ID
	if s.entry.Description != "" {
		metadata["repoDescription"] = s.entry.Description
	}
	if truncated {
		metadata["truncated"] = true
	}
	return types.ToolExecutionOutput{Status: types.ToolStatusSuccess, Content: content, Metadata: metadata}
}

// fail 返回带仓库身份的失败输出。
func (s session) fail(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["repo"] = s.entry.ID
	return failure(scope, err, metadata)
}

// queryMetadata 只在关键词非空时附加查询元数据。
func queryMetadata(query string) map[string]any {
	if strings.TrimSpace(query) == "" {
		return nil
	}
	return map[string]any{"query": query}
}

// failure 是统一的失败输出：如实说明失败原因，不返回假成功。
func failure(scope string, err error, metadata map[string]any) types.ToolExecutionOutput {
	if metadata == nil {
		metadata = map[string]any{}
	}
	errorMessage := scope
	if err != nil {
		errorMessage = scope + ": " + err.Error()
	}
	metadata["error"] = errorMessage
	return types.ToolExecutionOutput{Status: types.ToolStatusFailed, Content: errorMessage, Error: errorMessage, Metadata: metadata}
}
