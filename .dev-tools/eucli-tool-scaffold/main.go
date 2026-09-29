// eucli-tool-scaffold 是 AI 工具脚手架：整份复制基础模板并改写全部身份信息，
// 直接生成一个可继续开发的新 AI 工具。
//
// 用法：
//
//	create-tool.cmd -id my_tool -name 我的工具 -description "工具描述"
package main

import (
	"flag"
	"fmt"
	"go/token"
	"os"
	"strings"

	"devtools/common/toolruntime"
	"eucli-box/pkg/release"
	"eucli-box/pkg/releasecatalog"
	"eucli-box/pkg/types"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "生成新工具失败："+err.Error())
		os.Exit(1)
	}
}

type options struct {
	root        string
	id          string
	name        string
	description string
	version     string
}

func run(args []string) error {
	var opts options
	flags := flag.NewFlagSet("eucli-tool-scaffold", flag.ContinueOnError)
	flags.StringVar(&opts.root, "root", ".", "仓库根目录")
	flags.StringVar(&opts.id, "id", "", "新工具 ID（发布物标识）")
	flags.StringVar(&opts.name, "name", "", "新工具显示名")
	flags.StringVar(&opts.description, "description", "", "新工具描述")
	flags.StringVar(&opts.version, "version", "0.1.0", "新工具三段正式版本")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("不接受位置参数")
	}
	opts.root = strings.TrimSpace(opts.root)
	opts.id = strings.TrimSpace(opts.id)
	opts.name = strings.TrimSpace(opts.name)
	opts.description = strings.TrimSpace(opts.description)
	opts.version = strings.TrimSpace(opts.version)
	if opts.id == "" || opts.name == "" || opts.description == "" {
		return fmt.Errorf("必须提供 -id、-name 与 -description")
	}
	if err := releasecatalog.ValidateArtifactIdentity(types.ReleaseArtifactIdentity{
		Kind: types.ReleaseArtifactKindTool,
		ID:   opts.id,
	}); err != nil {
		return err
	}
	if err := release.ValidateFormalVersion(opts.version); err != nil {
		return fmt.Errorf("新工具版本无效：%w", err)
	}
	packageName, err := derivePackageName(opts.id)
	if err != nil {
		return err
	}
	root, err := toolruntime.ValidateRepositoryRoot(opts.root)
	if err != nil {
		return err
	}
	result, err := scaffold(root, scaffoldInput{
		ID:          opts.id,
		Name:        opts.name,
		Description: opts.description,
		Version:     opts.version,
		PackageName: packageName,
	})
	if err != nil {
		return err
	}
	fmt.Printf("已生成新工具 %s -> %s\n", opts.id, result.Directory)
	fmt.Println("提示：脚手架不登记发布名册；需要正式发布时，请另行把该工具加入名册。")
	return nil
}

// derivePackageName 从工具 ID 派生 Go 包名：去掉非字母数字字符并转小写。
// 结果必须是以字母开头的合法 Go 标识符，否则明确报错要求更换 ID。
func derivePackageName(toolID string) (string, error) {
	var builder strings.Builder
	for _, char := range toolID {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	name := strings.ToLower(builder.String())
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return "", fmt.Errorf("工具 ID %q 无法派生出以字母开头的 Go 包名", toolID)
	}
	if token.Lookup(name).IsKeyword() {
		return "", fmt.Errorf("工具 ID %q 派生出的 Go 包名 %q 是 Go 关键字", toolID, name)
	}
	return name, nil
}
