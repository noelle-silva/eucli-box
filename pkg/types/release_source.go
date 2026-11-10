package types

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateReleaseSourceAddress 校验发布来源地址：固定 HTTPS；隔离验证允许回环主机的 HTTP。
// 来源地址规则只在这里维护一份，发布资料与商店索引共同引用。
func ValidateReleaseSourceAddress(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return err
	}
	if parsed.User != nil {
		return fmt.Errorf("不能包含账号信息")
	}
	host := parsed.Hostname()
	if strings.EqualFold(parsed.Scheme, "https") && host != "" {
		return nil
	}
	if strings.EqualFold(parsed.Scheme, "http") && isLoopbackHost(host) {
		return nil
	}
	return fmt.Errorf("必须是固定的 HTTPS 地址")
}

// isLoopbackHost 判断主机是否为本机回环地址。
func isLoopbackHost(host string) bool {
	return host == "127.0.0.1" || host == "::1" || strings.EqualFold(host, "localhost")
}
