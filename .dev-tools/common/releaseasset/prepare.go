package releaseasset

import (
	"archive/zip"
	"context"
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"eucli-box/pkg/types"
)

//go:embed notices/git-bash.md
var gitBashNotice []byte

//go:embed notices/python-science.md
var pythonScienceNotice []byte

//go:embed notices/powershell.md
var powershellNotice []byte

//go:embed notices/nushell.md
var nushellNotice []byte

type PrepareOptions struct {
	RepositoryRoot string
	Artifact       types.ReleaseArtifactIdentity
	OutputRoot     string
	CacheRoot      string
	TempRoot       string
	Client         *http.Client
}

func PrepareRequired(ctx context.Context, options PrepareOptions) (map[string]string, error) {
	if ctx == nil {
		return nil, fmt.Errorf("外部随包准备上下文不能为空")
	}
	catalog, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	repositoryRoot, err := existingDirectory(options.RepositoryRoot)
	if err != nil {
		return nil, fmt.Errorf("读取仓库根目录失败：%w", err)
	}
	outputRoot, cacheRoot, tempRoot, err := prepareRoots(options.OutputRoot, options.CacheRoot, options.TempRoot)
	if err != nil {
		return nil, err
	}
	for _, root := range []string{outputRoot, cacheRoot, tempRoot} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return nil, err
		}
	}
	client := options.Client
	if client == nil {
		client = &http.Client{}
	}
	result := map[string]string{}
	for _, recipe := range catalog.RecipesForArtifact(options.Artifact) {
		if recipe.Kind == "existing" {
			root := filepath.Join(repositoryRoot, filepath.FromSlash(recipe.RepositoryPath))
			if _, err := Inspect(ctx, root, recipe.Name); err != nil {
				return nil, err
			}
			result[recipe.Name] = root
			continue
		}
		target := filepath.Join(outputRoot, recipe.Name)
		if !pathWithin(outputRoot, target) {
			return nil, fmt.Errorf("外部随包内容目标越过准备目录")
		}
		if _, err := os.Stat(target); err == nil {
			if _, inspectErr := Inspect(ctx, target, recipe.Name); inspectErr == nil {
				result[recipe.Name] = target
				continue
			}
			if err := os.RemoveAll(target); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		staging, err := os.MkdirTemp(outputRoot, ".staging-"+recipe.Name+"-")
		if err != nil {
			return nil, err
		}
		completed := false
		defer func() {
			if !completed {
				_ = os.RemoveAll(staging)
			}
		}()
		inputs, err := obtainInputs(ctx, client, recipe, cacheRoot)
		if err != nil {
			return nil, err
		}
		switch recipe.Kind {
		case "git-bash":
			err = prepareGitBash(ctx, recipe, inputs, staging, tempRoot)
		case "python-science":
			err = preparePythonScience(recipe, inputs, staging)
		case "powershell":
			err = prepareSingleZip(recipe, inputs, staging, powershellNotice)
		case "nushell":
			err = prepareSingleZip(recipe, inputs, staging, nushellNotice)
		default:
			err = fmt.Errorf("外部随包配方 %s 不能自动准备", recipe.Name)
		}
		if err != nil {
			return nil, err
		}
		if _, err := writeManifest(staging, recipe); err != nil {
			return nil, err
		}
		if _, err := Inspect(ctx, staging, recipe.Name); err != nil {
			return nil, err
		}
		if err := os.Rename(staging, target); err != nil {
			return nil, fmt.Errorf("启用外部随包内容 %s 失败：%w", recipe.Name, err)
		}
		completed = true
		result[recipe.Name] = target
	}
	return result, nil
}

func Inspect(ctx context.Context, root string, name string) (types.ReleaseExternalAsset, error) {
	catalog, err := LoadCatalog()
	if err != nil {
		return types.ReleaseExternalAsset{}, err
	}
	recipe, err := catalog.Recipe(name)
	if err != nil {
		return types.ReleaseExternalAsset{}, err
	}
	manifest, err := inspectDirectory(root, recipe)
	if err != nil {
		return types.ReleaseExternalAsset{}, err
	}
	if err := probe(ctx, root, recipe); err != nil {
		return types.ReleaseExternalAsset{}, fmt.Errorf("外部随包内容 %s 运行核对失败：%w", name, err)
	}
	verified, err := inspectDirectory(root, recipe)
	if err != nil {
		return types.ReleaseExternalAsset{}, fmt.Errorf("外部随包内容 %s 在运行核对后发生改变：%w", name, err)
	}
	if verified.TreeSHA256 != manifest.TreeSHA256 {
		return types.ReleaseExternalAsset{}, fmt.Errorf("外部随包内容 %s 在运行核对后目录完整性发生改变", name)
	}
	return externalAssetRecord(recipe, manifest), nil
}

func obtainInputs(ctx context.Context, client *http.Client, recipe Recipe, cacheRoot string) (map[string]string, error) {
	result := make(map[string]string, len(recipe.Inputs))
	for _, input := range recipe.Inputs {
		target := filepath.Join(cacheRoot, input.Name)
		if !pathWithin(cacheRoot, target) {
			return nil, fmt.Errorf("外部随包输入越过缓存目录")
		}
		if record, err := recordForFile(target, input.Name); err == nil && record.Size == input.Size && strings.EqualFold(record.SHA256, input.SHA256) {
			result[input.Name] = target
			continue
		}
		if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("User-Agent", "eucli-box-release-assets/1.0")
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("下载外部随包输入 %s 失败：%w", input.Name, err)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, fmt.Errorf("下载外部随包输入 %s 失败：远端返回 %d", input.Name, response.StatusCode)
		}
		temporary := target + ".partial"
		output, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = response.Body.Close()
			return nil, err
		}
		written, copyErr := io.Copy(output, io.LimitReader(response.Body, input.Size+1))
		closeBodyErr := response.Body.Close()
		closeFileErr := output.Close()
		if copyErr != nil || closeBodyErr != nil || closeFileErr != nil || written != input.Size {
			_ = os.Remove(temporary)
			return nil, fmt.Errorf("外部随包输入 %s 下载不完整", input.Name)
		}
		record, err := recordForFile(temporary, input.Name)
		if err != nil || !strings.EqualFold(record.SHA256, input.SHA256) {
			_ = os.Remove(temporary)
			return nil, fmt.Errorf("外部随包输入 %s 完整性不一致", input.Name)
		}
		if err := os.Rename(temporary, target); err != nil {
			return nil, err
		}
		result[input.Name] = target
	}
	return result, nil
}

func prepareGitBash(ctx context.Context, recipe Recipe, inputs map[string]string, target string, tempRoot string) error {
	minGit := inputs["MinGit-2.42.0.2-64-bit.zip"]
	portable := inputs["PortableGit-2.42.0.2-64-bit.7z.exe"]
	if minGit == "" || portable == "" {
		return fmt.Errorf("Git Bash 固定输入不完整")
	}
	if err := extractZip(minGit, target); err != nil {
		return fmt.Errorf("解开 MinGit 失败：%w", err)
	}
	extractRoot, err := os.MkdirTemp(tempRoot, "portable-git-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractRoot)
	portableCopy := filepath.Join(extractRoot, filepath.Base(portable))
	if err := copyFile(portable, portableCopy); err != nil {
		return err
	}
	processTemp := filepath.Join(extractRoot, "temp")
	if err := os.MkdirAll(processTemp, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, portableCopy, "-y", "-gm2")
	cmd.Dir = extractRoot
	cmd.Env = replaceEnvironment(os.Environ(), map[string]string{"TEMP": processTemp, "TMP": processTemp})
	hideProcessWindow(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("解开 PortableGit 失败：%w：%s", err, strings.TrimSpace(string(output)))
	}
	portableRoot := filepath.Join(extractRoot, "PortableGit")
	for _, name := range []string{"bin/bash.exe", "usr/bin/bash.exe"} {
		if err := copyFile(filepath.Join(portableRoot, filepath.FromSlash(name)), filepath.Join(target, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("取得固定 Bash 文件失败：%w", err)
		}
	}
	if err := os.WriteFile(filepath.Join(target, "THIRD_PARTY_NOTICES.md"), gitBashNotice, 0o644); err != nil {
		return err
	}
	return validatePinnedFiles(target, recipe.RequiredFiles)
}

func preparePythonScience(recipe Recipe, inputs map[string]string, target string) error {
	python := inputs["python-3.11.5-embed-amd64.zip"]
	if python == "" {
		return fmt.Errorf("Python 固定输入不完整")
	}
	if err := extractZip(python, target); err != nil {
		return fmt.Errorf("解开 Python 嵌入环境失败：%w", err)
	}
	sitePackages := filepath.Join(target, "Lib", "site-packages")
	if err := os.MkdirAll(sitePackages, 0o755); err != nil {
		return err
	}
	for _, input := range recipe.Inputs {
		if !strings.HasSuffix(strings.ToLower(input.Name), ".whl") {
			continue
		}
		if err := extractZip(inputs[input.Name], sitePackages); err != nil {
			return fmt.Errorf("解开 Python 固定依赖 %s 失败：%w", input.Name, err)
		}
	}
	pth := "python311.zip\n.\nLib\nLib/site-packages\nimport site\n"
	if err := os.WriteFile(filepath.Join(target, "python311._pth"), []byte(pth), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(target, "THIRD_PARTY_NOTICES.md"), pythonScienceNotice, 0o644); err != nil {
		return err
	}
	return validatePinnedFiles(target, recipe.RequiredFiles)
}

// prepareSingleZip 将单一 zip 输入原样解开到目标目录，并附带书面通告。
func prepareSingleZip(recipe Recipe, inputs map[string]string, target string, notice []byte) error {
	if len(recipe.Inputs) != 1 {
		return fmt.Errorf("外部随包配方 %s 要求恰好一个压缩包输入", recipe.Name)
	}
	archive, ok := inputs[recipe.Inputs[0].Name]
	if !ok || archive == "" {
		return fmt.Errorf("外部随包配方 %s 固定输入缺失", recipe.Name)
	}
	if err := extractZip(archive, target); err != nil {
		return fmt.Errorf("解开 %s 失败：%w", recipe.Name, err)
	}
	if err := os.WriteFile(filepath.Join(target, "THIRD_PARTY_NOTICES.md"), notice, 0o644); err != nil {
		return err
	}
	return validatePinnedFiles(target, recipe.RequiredFiles)
}

func extractZip(path string, target string) error {
	archive, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, file := range archive.File {
		name := filepath.ToSlash(strings.TrimSpace(file.Name))
		if !safeRelativePath(name) {
			return fmt.Errorf("压缩包包含越界路径：%s", file.Name)
		}
		destination := filepath.Join(target, filepath.FromSlash(name))
		if !pathWithin(target, destination) {
			return fmt.Errorf("压缩包路径越过目标目录：%s", name)
		}
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(destination, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return err
		}
		input, err := file.Open()
		if err != nil {
			return err
		}
		output, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		_ = input.Close()
		closeErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func copyFile(source string, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func validatePinnedFiles(root string, files []PinnedFileRecord) error {
	for _, expected := range files {
		actual, err := recordForFile(filepath.Join(root, filepath.FromSlash(expected.Path)), expected.Path)
		if err != nil || actual.Size != expected.Size || expected.SHA256 != "" && !strings.EqualFold(actual.SHA256, expected.SHA256) {
			return fmt.Errorf("固定输出文件不一致：%s", expected.Path)
		}
	}
	return nil
}

func prepareRoots(output string, cache string, temp string) (string, string, string, error) {
	values := []*string{&output, &cache, &temp}
	for _, value := range values {
		if strings.TrimSpace(*value) == "" {
			return "", "", "", fmt.Errorf("外部随包准备目录不能为空")
		}
		absolute, err := filepath.Abs(*value)
		if err != nil {
			return "", "", "", err
		}
		*value = filepath.Clean(absolute)
	}
	if samePath(output, cache) || samePath(output, temp) || samePath(cache, temp) || pathWithin(output, cache) || pathWithin(output, temp) || pathWithin(cache, output) || pathWithin(cache, temp) || pathWithin(temp, output) || pathWithin(temp, cache) {
		return "", "", "", fmt.Errorf("外部随包输出、缓存和临时目录必须彼此分开")
	}
	return output, cache, temp, nil
}

func pathWithin(base string, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(base), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left string, right string) bool {
	left, _ = filepath.Abs(left)
	right, _ = filepath.Abs(right)
	return filepath.Clean(left) == filepath.Clean(right)
}

func replaceEnvironment(base []string, replacements map[string]string) []string {
	result := make([]string, 0, len(base)+len(replacements))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := replacements[strings.ToUpper(key)]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range replacements {
		result = append(result, key+"="+value)
	}
	return result
}
