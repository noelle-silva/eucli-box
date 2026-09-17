// Package serviceprofile 是业务端启动配置画像的唯一读写入口。
//
// 画像文件固定位于应用数据目录根下（service-profile.json），以固定结构承载
// 网关端口与访问钥匙，由 fast-window 平台按固定读法查看与编辑；
// 业务端内部的初始化、网关监听与鉴权都只通过本包读写这份画像。
package serviceprofile

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"

	"eucli-box/pkg/datapaths"
)

// 画像的固定事实：默认监听端口、端口取值范围与监听宿主地址。
const (
	DefaultPort  = 8765
	portMinValue = 1
	portMaxValue = 65535
	listenHost   = "127.0.0.1"
)

// Profile 是一份通过校验的启动配置画像。
type Profile struct {
	Port int
	Key  string
}

// ListenAddr 返回网关监听地址。
func (p Profile) ListenAddr() string {
	return net.JoinHostPort(listenHost, strconv.Itoa(p.Port))
}

// profileDocument 是画像文件的固定结构。
type profileDocument struct {
	Port *int    `json:"port"`
	Key  *string `json:"key"`
}

// Load 读取并校验画像文件；文件缺失时错误链保留 os.ErrNotExist。
func Load(dataDir string) (Profile, error) {
	path := datapaths.ServiceProfileFile(dataDir)
	payload, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("读取启动配置画像失败：%w", err)
	}
	return parse(path, payload)
}

// Ensure 读取并校验画像文件；文件缺失时由业务端生成默认画像并落盘（自举）。
func Ensure(dataDir string) (Profile, error) {
	profile, err := Load(dataDir)
	if err == nil {
		return profile, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Profile{}, err
	}
	profile, err = generate()
	if err != nil {
		return Profile{}, err
	}
	if err := write(dataDir, profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// generate 生成默认画像：默认端口与随机访问钥匙。
func generate() (Profile, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return Profile{}, fmt.Errorf("生成访问钥匙失败：%w", err)
	}
	return Profile{Port: DefaultPort, Key: hex.EncodeToString(key)}, nil
}

// write 把画像按固定结构落盘。
func write(dataDir string, profile Profile) error {
	path := datapaths.ServiceProfileFile(dataDir)
	payload, err := json.MarshalIndent(profileDocument{Port: &profile.Port, Key: &profile.Key}, "", "  ")
	if err != nil {
		return fmt.Errorf("生成启动配置画像失败：%w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("写入启动配置画像失败：%w", err)
	}
	return nil
}

// parse 解析并校验画像内容：端口必须是 1-65535 的整数，钥匙必须是非空字符串。
func parse(path string, payload []byte) (Profile, error) {
	var document profileDocument
	if err := json.Unmarshal(payload, &document); err != nil {
		return Profile{}, fmt.Errorf("启动配置画像 %s 无效：%w", path, err)
	}
	if document.Port == nil {
		return Profile{}, fmt.Errorf("启动配置画像 %s 缺少 port 字段（必须为 %d-%d 之间的整数）", path, portMinValue, portMaxValue)
	}
	if *document.Port < portMinValue || *document.Port > portMaxValue {
		return Profile{}, fmt.Errorf("启动配置画像 %s 的 port 必须在 %d-%d 之间，实际为 %d", path, portMinValue, portMaxValue, *document.Port)
	}
	if document.Key == nil {
		return Profile{}, fmt.Errorf("启动配置画像 %s 缺少 key 字段", path)
	}
	key := strings.TrimSpace(*document.Key)
	if key == "" {
		return Profile{}, fmt.Errorf("启动配置画像 %s 的 key 不能为空", path)
	}
	return Profile{Port: *document.Port, Key: key}, nil
}
