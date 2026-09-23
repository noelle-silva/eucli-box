package toolcalling

import (
	"context"
	"testing"
	"time"

	"eucli-box/pkg/types"
)

// warmupStubSource is a controlled helper tool that only reports success for
// warm-up requests: any other request fails, proving the host really sent a
// warm-up request. It also sleeps so execution duration is measurable.
const warmupStubSource = `package main
import (
  "encoding/json"
  "os"
  "time"
)
type input struct {
  RequestKind string ` + "`json:\"requestKind\"`" + `
}
func main() {
  var in input
  if err := json.NewDecoder(os.Stdin).Decode(&in); err != nil { panic(err) }
  if in.RequestKind != "warmup" {
    json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"failed","content":"not a warmup request","error":"not a warmup request"})
    return
  }
  time.Sleep(120 * time.Millisecond)
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"success","content":"warmed","metadata":map[string]any{"warmup":true}})
}
`

// warmupFailStubSource is a controlled helper tool whose warm-up action fails.
const warmupFailStubSource = `package main
import (
  "encoding/json"
  "os"
)
func main() {
  json.NewEncoder(os.Stdout).Encode(map[string]any{"status":"failed","content":"warmup action failed","error":"warmup action failed","metadata":map[string]any{}})
}
`

func TestWarmupTool(t *testing.T) {
	executable := buildTool(t, warmupStubSource)

	newSystem := func(t *testing.T, tool types.ToolDefinition) *system {
		t.Helper()
		storage := newFakeToolStorage()
		storage.tools[tool.ID] = tool
		return mustConcreteToolSystem(t, newTestToolSystem(t, &fakePermission{}, storage, Config{}))
	}

	t.Run("runs declared tool with warmup request", func(t *testing.T) {
		tool := testTool(t, executable)
		tool.DefaultConfig = map[string]any{"warmup": true}
		system := newSystem(t, tool)
		duration, err := system.warmupTool(context.Background(), tool.ID)
		if err != nil {
			t.Fatalf("warmupTool() error = %v", err)
		}
		if duration < 100*time.Millisecond {
			t.Fatalf("duration = %v, want >= 100ms", duration)
		}
	})

	t.Run("rejects undeclared tool", func(t *testing.T) {
		tool := testTool(t, executable)
		system := newSystem(t, tool)
		_, err := system.warmupTool(context.Background(), tool.ID)
		assertAppErrorCode(t, err, "tool.execution_invalid")
	})

	t.Run("rejects tool disabled by user config", func(t *testing.T) {
		tool := testTool(t, executable)
		tool.DefaultConfig = map[string]any{"warmup": true}
		tool.UserConfig = map[string]any{"warmup": false}
		system := newSystem(t, tool)
		_, err := system.warmupTool(context.Background(), tool.ID)
		assertAppErrorCode(t, err, "tool.execution_invalid")
	})

	t.Run("runs tool enabled by user config", func(t *testing.T) {
		tool := testTool(t, executable)
		tool.DefaultConfig = map[string]any{"warmup": false}
		tool.UserConfig = map[string]any{"warmup": true}
		system := newSystem(t, tool)
		if _, err := system.warmupTool(context.Background(), tool.ID); err != nil {
			t.Fatalf("warmupTool() error = %v", err)
		}
	})
}

func TestWarmupToolReportsToolFailure(t *testing.T) {
	executable := buildTool(t, warmupFailStubSource)
	tool := testTool(t, executable)
	tool.DefaultConfig = map[string]any{"warmup": true}
	storage := newFakeToolStorage()
	storage.tools[tool.ID] = tool
	system := mustConcreteToolSystem(t, newTestToolSystem(t, &fakePermission{}, storage, Config{}))

	_, err := system.warmupTool(context.Background(), tool.ID)
	if err == nil {
		t.Fatal("warmupTool() error = nil, want tool-reported failure")
	}
	assertAppErrorCode(t, err, "tool.execution_invalid")
}

// mustConcreteToolSystem 把测试系统的接口形态收敛为具体形态，
// 让同包测试可以调用不进入公共接口的预热能力。
func mustConcreteToolSystem(t *testing.T, tools System) *system {
	t.Helper()
	concrete, ok := tools.(*system)
	if !ok {
		t.Fatalf("system type = %T, want *system", tools)
	}
	return concrete
}
