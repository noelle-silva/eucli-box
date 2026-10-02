package webfetch

import (
	"math/rand"
	"strings"

	"github.com/bogdanfinn/tls-client/profiles"
)

// browserProfile 是一套"浏览器身份"：一套与真实浏览器一致的请求头，
// 加上一个匹配的 TLS 指纹。按请求随机挑选，避免固定指纹被识别。
type browserProfile struct {
	// Name 是身份标识，用于日志与元数据。
	Name string
	// TLS 是 TLS 指纹配置，由 tls-client 在握手层伪装成对应浏览器。
	TLS profiles.ClientProfile
	// Headers 是请求头集合，键为小写；顺序在 headerOrder 中显式声明。
	Headers map[string]string
	// HeaderOrder 是请求头的发送顺序，贴近真实浏览器。
	HeaderOrder []string
}

// browserProfiles 是内置的浏览器身份池：Chrome、Firefox、Edge 各若干版本。
var browserProfiles = []browserProfile{
	{
		Name: "chrome-133",
		TLS:  profiles.Chrome_133,
		Headers: map[string]string{
			"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
			"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"accept-language":           "en-US,en;q=0.9",
			"accept-encoding":           "gzip, deflate, br, zstd",
			"sec-ch-ua":                 `"Chromium";v="133", "Google Chrome";v="133", "Not-A.Brand";v="24"`,
			"sec-ch-ua-mobile":          "?0",
			"sec-ch-ua-platform":        `"Windows"`,
			"sec-fetch-dest":            "document",
			"sec-fetch-mode":            "navigate",
			"sec-fetch-site":            "none",
			"sec-fetch-user":            "?1",
			"upgrade-insecure-requests": "1",
		},
		HeaderOrder: []string{
			"host", "connection", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
			"upgrade-insecure-requests", "user-agent", "accept", "sec-fetch-site",
			"sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest", "referer",
			"accept-encoding", "accept-language",
		},
	},
	{
		Name: "chrome-131",
		TLS:  profiles.Chrome_131,
		Headers: map[string]string{
			"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
			"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
			"accept-language":           "en-US,en;q=0.9",
			"accept-encoding":           "gzip, deflate, br, zstd",
			"sec-ch-ua":                 `"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"`,
			"sec-ch-ua-mobile":          "?0",
			"sec-ch-ua-platform":        `"Windows"`,
			"sec-fetch-dest":            "document",
			"sec-fetch-mode":            "navigate",
			"sec-fetch-site":            "none",
			"sec-fetch-user":            "?1",
			"upgrade-insecure-requests": "1",
		},
		HeaderOrder: []string{
			"host", "connection", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
			"upgrade-insecure-requests", "user-agent", "accept", "sec-fetch-site",
			"sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest", "referer",
			"accept-encoding", "accept-language",
		},
	},
	{
		Name: "firefox-133",
		TLS:  profiles.Firefox_133,
		Headers: map[string]string{
			"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:133.0) Gecko/20100101 Firefox/133.0",
			"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/png,image/svg+xml,*/*;q=0.8",
			"accept-language":           "en-US,en;q=0.5",
			"accept-encoding":           "gzip, deflate, br, zstd",
			"sec-fetch-dest":            "document",
			"sec-fetch-mode":            "navigate",
			"sec-fetch-site":            "none",
			"sec-fetch-user":            "?1",
			"upgrade-insecure-requests": "1",
		},
		HeaderOrder: []string{
			"host", "user-agent", "accept", "accept-language", "accept-encoding",
			"upgrade-insecure-requests", "sec-fetch-dest", "sec-fetch-mode",
			"sec-fetch-site", "sec-fetch-user", "referer",
		},
	},
	{
		Name: "firefox-135",
		TLS:  profiles.Firefox_135,
		Headers: map[string]string{
			"user-agent":                "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:135.0) Gecko/20100101 Firefox/135.0",
			"accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/png,image/svg+xml,*/*;q=0.8",
			"accept-language":           "en-US,en;q=0.5",
			"accept-encoding":           "gzip, deflate, br, zstd",
			"sec-fetch-dest":            "document",
			"sec-fetch-mode":            "navigate",
			"sec-fetch-site":            "none",
			"sec-fetch-user":            "?1",
			"upgrade-insecure-requests": "1",
		},
		HeaderOrder: []string{
			"host", "user-agent", "accept", "accept-language", "accept-encoding",
			"upgrade-insecure-requests", "sec-fetch-dest", "sec-fetch-mode",
			"sec-fetch-site", "sec-fetch-user", "referer",
		},
	},
}

// selectBrowserProfile 从身份池中随机挑选一套；池为空时返回零值。
func selectBrowserProfile(rng *rand.Rand) browserProfile {
	if len(browserProfiles) == 0 {
		return browserProfile{}
	}
	return browserProfiles[rng.Intn(len(browserProfiles))]
}

// profileByName 按名称查找身份，用于固定身份的场景；找不到返回 false。
func profileByName(name string) (browserProfile, bool) {
	trimmed := strings.TrimSpace(name)
	for _, profile := range browserProfiles {
		if profile.Name == trimmed {
			return profile, true
		}
	}
	return browserProfile{}, false
}

// profileNames 返回全部可用身份名称。
func profileNames() []string {
	names := make([]string, 0, len(browserProfiles))
	for _, profile := range browserProfiles {
		names = append(names, profile.Name)
	}
	return names
}
