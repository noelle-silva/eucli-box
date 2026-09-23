package toolcalling

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// warmupCountStubSource is a controlled helper tool that appends one line to a
// counter file inside its data directory for every accepted warm-up request.
const warmupCountStubSource = `package main
import (
  "encoding/json"
  "os"
  "path/filepath"
)
type input struct {
  RequestKind       string ` + "`json:\"requestKind\"`" + `
  ToolDataDirectory string ` + "`json:\"toolDataDirectory\"`" + `
}
func main() {
  var in input
  if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil { panic(err) }
  if in.RequestKind != "warmup" {
    json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"failed","content":"not a warmup request","error":"not a warmup request"})
    return
  }
  file, err := os.OpenFile(filepath.Join(in.ToolDataDirectory, "warmup-count.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
  if err != nil { panic(err) }
  if _, err := file.WriteString("x\n"); err != nil { panic(err) }
  _ = file.Close()
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"success","content":"warmed"})
}
`

func TestWarmupScheduler(t *testing.T) {
	executable := buildTool(t, warmupCountStubSource)

	t.Run("runs first round immediately", func(t *testing.T) {
		system, active, _ := newWarmupSchedulerTestSystem(t, executable, time.Hour)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := system.StartWarmup(ctx); err != nil {
			t.Fatalf("StartWarmup() error = %v", err)
		}
		waitForWarmupCount(t, warmupCountPath(active), 1, 5*time.Second)
		if err := system.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
	})

	t.Run("repeats by interval, skips undeclared tools and stops on shutdown", func(t *testing.T) {
		system, active, idle := newWarmupSchedulerTestSystem(t, executable, 100*time.Millisecond)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := system.StartWarmup(ctx); err != nil {
			t.Fatalf("StartWarmup() error = %v", err)
		}
		waitForWarmupCount(t, warmupCountPath(active), 3, 5*time.Second)
		if count := countWarmupLines(t, warmupCountPath(idle)); count != 0 {
			t.Fatalf("undeclared tool warmup count = %d, want 0", count)
		}
		if err := system.Shutdown(context.Background()); err != nil {
			t.Fatalf("Shutdown() error = %v", err)
		}
		stopped := countWarmupLines(t, warmupCountPath(active))
		time.Sleep(350 * time.Millisecond)
		if count := countWarmupLines(t, warmupCountPath(active)); count != stopped {
			t.Fatalf("warmup continued after shutdown: %d -> %d", stopped, count)
		}
	})
}

func newWarmupSchedulerTestSystem(t *testing.T, executable string, interval time.Duration) (*system, types.ToolDefinition, types.ToolDefinition) {
	t.Helper()
	active := testTool(t, executable)
	active.DefaultConfig = map[string]any{"warmup": true}
	active.DataDirectory = t.TempDir()
	idle := testTool(t, executable)
	idle.ID = "file-reader-idle"
	idle.Name = "file-reader-idle"
	idle.DataDirectory = t.TempDir()
	storage := newFakeToolStorage()
	storage.tools[active.ID] = active
	storage.tools[idle.ID] = idle
	system := mustConcreteToolSystem(t, newTestToolSystem(t, &fakePermission{}, storage, Config{ToolWarmupInterval: interval}))
	return system, active, idle
}

func warmupCountPath(tool types.ToolDefinition) string {
	return filepath.Join(tool.DataDirectory, "warmup-count.txt")
}

func countWarmupLines(t *testing.T, path string) int {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return strings.Count(string(payload), "\n")
}

func waitForWarmupCount(t *testing.T, path string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if countWarmupLines(t, path) >= want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("warmup count = %d, want >= %d within %s", countWarmupLines(t, path), want, timeout)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
