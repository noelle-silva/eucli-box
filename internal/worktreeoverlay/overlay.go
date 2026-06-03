package worktreeoverlay

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const stateVersion = 1

type State struct {
	Version    int          `json:"version"`
	SourceSpec string       `json:"sourceSpec"`
	SourceRoot string       `json:"sourceRoot"`
	TargetRoot string       `json:"targetRoot"`
	TargetHead string       `json:"targetHead"`
	SourceHead string       `json:"sourceHead"`
	MergeBase  string       `json:"mergeBase"`
	Session    string       `json:"session"`
	AppliedAt  time.Time    `json:"appliedAt"`
	Entries    []StateEntry `json:"entries"`
}

type StateEntry struct {
	Path           string `json:"path"`
	OriginalExists bool   `json:"originalExists"`
	OriginalHash   string `json:"originalHash,omitempty"`
	BackupPath     string `json:"backupPath,omitempty"`
	OverlayExists  bool   `json:"overlayExists"`
	OverlayHash    string `json:"overlayHash,omitempty"`
}

type overlayPlan struct {
	State   State
	Entries []plannedEntry
}

type plannedEntry struct {
	StateEntry
	SourcePath string
	TargetPath string
	BackupPath string
	Mode       os.FileMode
}

func (rt *runtimeContext) Apply(sourceSpec string, output io.Writer) error {
	if _, exists, err := rt.readState(); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("overlay is already active; use refresh or clear")
	}
	if err := rt.requireCleanTarget(); err != nil {
		return err
	}
	source, canonicalSpec, err := rt.resolveSource(sourceSpec)
	if err != nil {
		return err
	}
	plan, err := rt.buildPlan(source, canonicalSpec)
	if err != nil {
		return err
	}
	if len(plan.Entries) == 0 {
		fmt.Fprintln(output, "no source changes to overlay")
		return nil
	}
	if err := rt.applyPlan(plan); err != nil {
		return err
	}
	fmt.Fprintf(output, "applied overlay from %s (%d paths)\n", canonicalSpec, len(plan.Entries))
	return nil
}

func (rt *runtimeContext) Refresh(sourceSpec string, output io.Writer) error {
	state, exists, err := rt.readState()
	if err != nil {
		return err
	}
	if !exists {
		if sourceSpec == "" {
			return fmt.Errorf("no active overlay; use apply <worktree-name-or-path>")
		}
		return rt.Apply(sourceSpec, output)
	}
	if sourceSpec == "" {
		sourceSpec = state.SourceSpec
	}
	if err := rt.restoreState(state); err != nil {
		return err
	}
	if err := rt.removeState(state); err != nil {
		return err
	}
	if err := rt.requireCleanTarget(); err != nil {
		return err
	}
	source, canonicalSpec, err := rt.resolveSource(sourceSpec)
	if err != nil {
		return err
	}
	plan, err := rt.buildPlan(source, canonicalSpec)
	if err != nil {
		return err
	}
	if len(plan.Entries) == 0 {
		fmt.Fprintln(output, "cleared overlay; no source changes remain")
		return nil
	}
	if err := rt.applyPlan(plan); err != nil {
		return err
	}
	fmt.Fprintf(output, "refreshed overlay from %s (%d paths)\n", canonicalSpec, len(plan.Entries))
	return nil
}

func (rt *runtimeContext) Clear(output io.Writer) error {
	state, exists, err := rt.readState()
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintln(output, "no active overlay")
		return nil
	}
	if err := rt.restoreState(state); err != nil {
		return err
	}
	if err := rt.removeState(state); err != nil {
		return err
	}
	fmt.Fprintf(output, "cleared overlay from %s (%d paths)\n", state.SourceSpec, len(state.Entries))
	return nil
}

func (rt *runtimeContext) Status(output io.Writer) error {
	state, exists, err := rt.readState()
	if err != nil {
		return err
	}
	if !exists {
		fmt.Fprintln(output, "no active overlay")
		return nil
	}
	if err := rt.verifyState(state); err != nil {
		return err
	}
	fmt.Fprintf(output, "active overlay\n")
	fmt.Fprintf(output, "  source: %s\n", state.SourceSpec)
	fmt.Fprintf(output, "  sourceRoot: %s\n", state.SourceRoot)
	fmt.Fprintf(output, "  targetRoot: %s\n", state.TargetRoot)
	fmt.Fprintf(output, "  paths: %d\n", len(state.Entries))
	fmt.Fprintf(output, "  appliedAt: %s\n", state.AppliedAt.Format(time.RFC3339))
	fmt.Fprintf(output, "  integrity: ok\n")
	return nil
}

func (rt *runtimeContext) requireCleanTarget() error {
	status, err := cleanStatus(rt.ctx, rt.target.Root)
	if err != nil {
		return fmt.Errorf("check target status: %w", err)
	}
	if status != "" {
		return fmt.Errorf("target worktree must be clean before applying overlay:\n%s", status)
	}
	return nil
}

func (rt *runtimeContext) buildPlan(source repository, sourceSpec string) (overlayPlan, error) {
	mergeBase, err := gitOutput(rt.ctx, rt.target.Root, "merge-base", rt.target.Head, source.Head)
	if err != nil {
		return overlayPlan{}, fmt.Errorf("resolve merge base: %w", err)
	}
	if err := rt.requireNoUnmergedPaths(source.Root); err != nil {
		return overlayPlan{}, err
	}
	sourcePaths, err := rt.sourceChangedPaths(source.Root, mergeBase)
	if err != nil {
		return overlayPlan{}, err
	}
	targetPaths, err := gitList(rt.ctx, rt.target.Root, "diff", "--name-only", "-z", mergeBase, rt.target.Head, "--")
	if err != nil {
		return overlayPlan{}, fmt.Errorf("list target changed paths: %w", err)
	}
	if overlap := overlappingPaths(sourcePaths, targetPaths); len(overlap) > 0 {
		return overlayPlan{}, fmt.Errorf("source and target both changed these paths since merge base: %v", overlap)
	}
	session := fmt.Sprintf("sessions/%s-%d", time.Now().UTC().Format("20060102T150405.000000000Z"), os.Getpid())
	state := State{
		Version:    stateVersion,
		SourceSpec: sourceSpec,
		SourceRoot: source.Root,
		TargetRoot: rt.target.Root,
		TargetHead: rt.target.Head,
		SourceHead: source.Head,
		MergeBase:  mergeBase,
		Session:    session,
		AppliedAt:  time.Now().UTC(),
	}
	plan := overlayPlan{State: state}
	for _, repoPath := range sourcePaths {
		entry, include, err := rt.planEntry(source, session, repoPath)
		if err != nil {
			return overlayPlan{}, err
		}
		if !include {
			continue
		}
		plan.Entries = append(plan.Entries, entry)
		plan.State.Entries = append(plan.State.Entries, entry.StateEntry)
	}
	return plan, nil
}

func (rt *runtimeContext) requireNoUnmergedPaths(sourceRoot string) error {
	paths, err := gitList(rt.ctx, sourceRoot, "diff", "--name-only", "--diff-filter=U", "-z", "--")
	if err != nil {
		return fmt.Errorf("check source unmerged paths: %w", err)
	}
	if len(paths) > 0 {
		return fmt.Errorf("source worktree has unmerged paths: %v", paths)
	}
	return nil
}

func (rt *runtimeContext) sourceChangedPaths(sourceRoot string, mergeBase string) ([]string, error) {
	tracked, err := gitList(rt.ctx, sourceRoot, "diff", "--name-only", "-z", mergeBase, "--")
	if err != nil {
		return nil, fmt.Errorf("list source changed paths: %w", err)
	}
	untracked, err := gitList(rt.ctx, sourceRoot, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list source untracked paths: %w", err)
	}
	paths := map[string]struct{}{}
	for _, value := range append(tracked, untracked...) {
		path, err := validateRepoPath(value)
		if err != nil {
			return nil, err
		}
		paths[path] = struct{}{}
	}
	values := make([]string, 0, len(paths))
	for path := range paths {
		values = append(values, path)
	}
	sort.Strings(values)
	return values, nil
}

func (rt *runtimeContext) planEntry(source repository, session string, repoPath string) (plannedEntry, bool, error) {
	sourcePath, err := repoFile(source.Root, repoPath)
	if err != nil {
		return plannedEntry{}, false, err
	}
	targetPath, err := repoFile(rt.target.Root, repoPath)
	if err != nil {
		return plannedEntry{}, false, err
	}
	sourceSnapshot, err := snapshotFile(sourcePath)
	if err != nil {
		return plannedEntry{}, false, err
	}
	targetSnapshot, err := snapshotFile(targetPath)
	if err != nil {
		return plannedEntry{}, false, err
	}
	if sameSnapshot(sourceSnapshot, targetSnapshot) {
		return plannedEntry{}, false, nil
	}
	backupRel := ""
	backupAbs := ""
	if targetSnapshot.Exists {
		backupRel = filepath.ToSlash(filepath.Join(session, "backups", filepath.FromSlash(repoPath)))
		backupAbs = filepath.Join(rt.stateRoot, filepath.FromSlash(backupRel))
	}
	entry := plannedEntry{
		StateEntry: StateEntry{
			Path:           repoPath,
			OriginalExists: targetSnapshot.Exists,
			OriginalHash:   targetSnapshot.Hash,
			BackupPath:     backupRel,
			OverlayExists:  sourceSnapshot.Exists,
			OverlayHash:    sourceSnapshot.Hash,
		},
		SourcePath: sourcePath,
		TargetPath: targetPath,
		BackupPath: backupAbs,
		Mode:       sourceSnapshot.Mode,
	}
	return entry, true, nil
}

func (rt *runtimeContext) applyPlan(plan overlayPlan) error {
	sessionDir := filepath.Join(rt.stateRoot, filepath.FromSlash(plan.State.Session))
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return fmt.Errorf("create overlay session: %w", err)
	}
	for _, entry := range plan.Entries {
		if entry.OriginalExists {
			if err := copyFile(entry.TargetPath, entry.BackupPath, 0o644); err != nil {
				os.RemoveAll(sessionDir)
				return fmt.Errorf("backup %s: %w", entry.Path, err)
			}
		}
	}
	applied := []StateEntry{}
	for _, entry := range plan.Entries {
		if entry.OverlayExists {
			if err := copyFile(entry.SourcePath, entry.TargetPath, entry.Mode); err != nil {
				rt.rollback(applied, plan.State)
				os.RemoveAll(sessionDir)
				return err
			}
		} else if err := removeFileAndEmptyParents(rt.target.Root, entry.TargetPath); err != nil {
			rt.rollback(applied, plan.State)
			os.RemoveAll(sessionDir)
			return err
		}
		applied = append(applied, entry.StateEntry)
	}
	if err := rt.writeState(plan.State); err != nil {
		rt.rollback(applied, plan.State)
		os.RemoveAll(sessionDir)
		return err
	}
	return nil
}

func (rt *runtimeContext) rollback(entries []StateEntry, state State) {
	rollbackState := state
	rollbackState.Entries = entries
	_ = rt.restoreStateWithoutVerify(rollbackState)
}

func (rt *runtimeContext) restoreState(state State) error {
	if err := rt.verifyState(state); err != nil {
		return err
	}
	return rt.restoreStateWithoutVerify(state)
}

func (rt *runtimeContext) verifyState(state State) error {
	if state.Version != stateVersion {
		return fmt.Errorf("unsupported overlay state version %d", state.Version)
	}
	if !samePath(state.TargetRoot, rt.target.Root) {
		return fmt.Errorf("overlay belongs to %s, current target is %s", state.TargetRoot, rt.target.Root)
	}
	for _, entry := range state.Entries {
		targetPath, err := repoFile(rt.target.Root, entry.Path)
		if err != nil {
			return err
		}
		current, err := snapshotFile(targetPath)
		if err != nil {
			return fmt.Errorf("verify overlay %s: %w", entry.Path, err)
		}
		if entry.OverlayExists {
			if !current.Exists {
				return fmt.Errorf("overlay path %s is missing", entry.Path)
			}
			if current.Hash != entry.OverlayHash {
				return fmt.Errorf("overlay path %s changed after apply; refusing to modify it", entry.Path)
			}
			continue
		}
		if current.Exists {
			return fmt.Errorf("overlay path %s was recreated after deletion; refusing to modify it", entry.Path)
		}
	}
	return nil
}

func (rt *runtimeContext) restoreStateWithoutVerify(state State) error {
	entries := append([]StateEntry(nil), state.Entries...)
	sort.SliceStable(entries, func(i int, j int) bool {
		return entries[i].Path > entries[j].Path
	})
	for _, entry := range entries {
		targetPath, err := repoFile(rt.target.Root, entry.Path)
		if err != nil {
			return err
		}
		if entry.OriginalExists {
			backupPath := filepath.Join(rt.stateRoot, filepath.FromSlash(entry.BackupPath))
			if err := copyFile(backupPath, targetPath, 0o644); err != nil {
				return fmt.Errorf("restore %s: %w", entry.Path, err)
			}
			continue
		}
		if err := removeFileAndEmptyParents(rt.target.Root, targetPath); err != nil {
			return err
		}
	}
	return nil
}

func (rt *runtimeContext) readState() (State, bool, error) {
	payload, err := os.ReadFile(rt.stateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, false, nil
		}
		return State{}, false, fmt.Errorf("read overlay state: %w", err)
	}
	var state State
	if err := json.Unmarshal(payload, &state); err != nil {
		return State{}, false, fmt.Errorf("decode overlay state: %w", err)
	}
	return state, true, nil
}

func (rt *runtimeContext) writeState(state State) error {
	if err := os.MkdirAll(rt.stateRoot, 0o755); err != nil {
		return fmt.Errorf("create overlay state directory: %w", err)
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode overlay state: %w", err)
	}
	payload = append(payload, '\n')
	if err := os.WriteFile(rt.stateFile(), payload, 0o644); err != nil {
		return fmt.Errorf("write overlay state: %w", err)
	}
	return nil
}

func (rt *runtimeContext) removeState(state State) error {
	if err := os.Remove(rt.stateFile()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove overlay state: %w", err)
	}
	if state.Session != "" {
		if err := os.RemoveAll(filepath.Join(rt.stateRoot, filepath.FromSlash(state.Session))); err != nil {
			return fmt.Errorf("remove overlay session: %w", err)
		}
	}
	return nil
}

func (rt *runtimeContext) stateFile() string {
	return filepath.Join(rt.stateRoot, "state.json")
}

func sameSnapshot(left fileSnapshot, right fileSnapshot) bool {
	if left.Exists != right.Exists {
		return false
	}
	if !left.Exists {
		return true
	}
	return left.Hash == right.Hash
}

func overlappingPaths(left []string, right []string) []string {
	seen := map[string]struct{}{}
	for _, path := range left {
		seen[path] = struct{}{}
	}
	overlap := []string{}
	for _, path := range right {
		cleaned, err := validateRepoPath(path)
		if err != nil {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			overlap = append(overlap, cleaned)
		}
	}
	sort.Strings(overlap)
	return overlap
}
