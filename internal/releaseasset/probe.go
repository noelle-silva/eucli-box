package releaseasset

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func probe(ctx context.Context, root string, recipe Recipe) error {
	if ctx == nil {
		return fmt.Errorf("运行核对上下文不能为空")
	}
	switch recipe.Kind {
	case "existing":
		cmd := exec.CommandContext(ctx, filepath.Join(root, "es.exe"), "-version")
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "1.1.0.30") {
			return fmt.Errorf("Everything CLI 版本核对失败：%w", err)
		}
		return nil
	case "git-bash":
		cmd := exec.CommandContext(ctx, filepath.Join(root, "bin", "bash.exe"), "--noprofile", "--norc", "-lc", "test \"$(command -v grep)\" = /usr/bin/grep && test \"$(command -v git)\" = /mingw64/bin/git && printf 'release-root\\n' | grep -qx release-root && git --version")
		cmd.Dir = root
		cmd.Env = replaceEnvironment(os.Environ(), map[string]string{"PATH": `C:\Windows\System32`, "HOME": filepath.Join(root, ".probe-home")})
		hideProcessWindow(cmd)
		output, err := cmd.CombinedOutput()
		_ = os.RemoveAll(filepath.Join(root, ".probe-home"))
		if err != nil || !strings.Contains(string(output), "git version 2.42.0.windows.2") {
			return fmt.Errorf("Git Bash 固定命令核对失败：%w：%s", err, strings.TrimSpace(string(output)))
		}
		return nil
	case "python-science":
		script := "import numpy, scipy, sympy, mpmath; from scipy import stats; from scipy.integrate import quad; assert numpy.__version__ == '1.24.3'; assert scipy.__version__ == '1.11.1'; assert sympy.__version__ == '1.11.1'; assert mpmath.__version__ == '1.3.0'; assert abs(stats.norm.cdf(0)-0.5) < 1e-12; assert abs(quad(lambda x: x, 0, 1)[0]-0.5) < 1e-12; print('python-science-ok')"
		cmd := exec.CommandContext(ctx, filepath.Join(root, "python.exe"), "-I", "-B", "-c", script)
		cmd.Dir = root
		cmd.Env = replaceEnvironment(os.Environ(), map[string]string{"PYTHONHOME": root, "PYTHONPATH": "", "PYTHONNOUSERSITE": "1", "PYTHONDONTWRITEBYTECODE": "1"})
		hideProcessWindow(cmd)
		output, err := cmd.CombinedOutput()
		if err != nil || strings.TrimSpace(string(output)) != "python-science-ok" {
			return fmt.Errorf("科学计算 Python 固定能力核对失败：%w：%s", err, strings.TrimSpace(string(output)))
		}
		return nil
	default:
		return fmt.Errorf("没有为外部随包类别 %s 定义运行核对", recipe.Kind)
	}
}
