package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"devtools/common/releaseops"
	"eucli-box/pkg/release"
)

type options struct {
	root    string
	target  string
	version string
	bump    release.VersionLevel
	message string
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "版本命令失败："+err.Error())
		os.Exit(1)
	}
}

func run(args []string, output io.Writer) error {
	opts, err := parseOptions(args)
	if err != nil {
		return err
	}
	if opts.version == "" && opts.bump == "" {
		return runCheck(opts, output)
	}
	nextVersion, err := resolveNextVersion(opts)
	if err != nil {
		return err
	}
	result, err := releaseops.SetVersion(opts.root, opts.target, nextVersion, opts.message)
	if err != nil {
		return err
	}
	fmt.Fprintf(output, "%s 版本已从 %s 调整为 %s，完整检查通过。\n", result.Target, result.PreviousVersion, result.Version)
	if result.Warning != "" {
		fmt.Fprintln(output, "警告："+result.Warning)
	}
	return nil
}

func parseOptions(args []string) (options, error) {
	var opts options
	var bumpText string
	flags := flag.NewFlagSet("eucli-version", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.root, "root", ".", "仓库根目录")
	flags.StringVar(&opts.target, "target", "", "发布物：eucli-box、eucli-studio、tool:<id> 或 plugin:<id>")
	flags.StringVar(&opts.version, "version", "", "要调整到的三段正式版本；省略时只检查")
	flags.StringVar(&bumpText, "bump", "", "自动递增三段正式版本的档位：patch、minor 或 major")
	flags.StringVar(&opts.message, "message", "", "调整版本时写入 CHANGELOG 的中文说明")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("不接受位置参数")
	}
	opts.root = strings.TrimSpace(opts.root)
	opts.target = strings.TrimSpace(opts.target)
	opts.version = strings.TrimSpace(opts.version)
	opts.message = strings.TrimSpace(opts.message)
	bumpText = strings.TrimSpace(bumpText)
	if opts.root == "" {
		return options{}, fmt.Errorf("仓库根目录不能为空")
	}
	if bumpText != "" {
		level, err := release.ParseVersionLevel(bumpText)
		if err != nil {
			return options{}, err
		}
		opts.bump = level
	}
	adjusting := opts.version != "" || opts.bump != ""
	if !adjusting && opts.message != "" {
		return options{}, fmt.Errorf("只检查时不能提供 -message")
	}
	if opts.version != "" && opts.bump != "" {
		return options{}, fmt.Errorf("-version 与 -bump 只能二选一")
	}
	if adjusting && opts.target == "" {
		return options{}, fmt.Errorf("调整版本时必须提供 -target")
	}
	return opts, nil
}

func resolveNextVersion(opts options) (string, error) {
	if opts.bump == "" {
		return opts.version, nil
	}
	artifact, err := releaseops.Resolve(opts.root, opts.target)
	if err != nil {
		return "", err
	}
	nextVersion, err := release.NextFormalVersion(artifact.Version, opts.bump)
	if err != nil {
		return "", fmt.Errorf("计算下一个版本失败：%w", err)
	}
	return nextVersion, nil
}

func runCheck(opts options, output io.Writer) error {
	if opts.target != "" {
		artifact, err := releaseops.Resolve(opts.root, opts.target)
		if err != nil {
			return err
		}
		if err := releaseops.Check(artifact); err != nil {
			return err
		}
		fmt.Fprintf(output, "%s %s：完整检查通过。\n", artifact.Target(), artifact.Version)
		return nil
	}
	results, err := releaseops.CheckAll(opts.root)
	if err != nil {
		return err
	}
	for _, result := range results {
		fmt.Fprintf(output, "%s %s：检查通过。\n", result.Target, result.Version)
	}
	fmt.Fprintf(output, "共 %d 个发布物完成完整检查。\n", len(results))
	return nil
}
