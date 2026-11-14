package modelprovider

import (
	"context"
	"errors"
	"net/http"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

// 三档熔断的时长参数集中在这里：档位与时长是同一件事，分散写就容易各改一半。
const (
	// breakerCooldown 是"关一会儿再看"的时长：偶发档关这么久，
	// 限流档上游没交代 Retry-After 时也退回它。
	breakerCooldown = 30 * time.Second
	// breakerHardCooldown 是硬伤档的关停时长：凭据失效、连接层不通、响应解析不出内容，
	// 这些问题不会自己好，关久一点，省掉那些注定失败的试探。
	breakerHardCooldown = 5 * time.Minute
	// breakerStrikeWindow 是连续失败的计数窗口：两次失败间隔超过它，旧计数作废，
	// 这次失败重新算第一次——两分钟前抽风过一次，不算和现在"连着"。
	breakerStrikeWindow = 2 * time.Minute
	// breakerStrikeThreshold 是偶发档攒够几次连续失败才关人。
	breakerStrikeThreshold = 2
)

// breakerTier 是成员失败在熔断表里的档位：它决定这次失败关不关人、关多久。
type breakerTier int

const (
	// breakerTierNone 表示这次失败只换人、不关人：成因说不清，不足以据此拉黑一个成员。
	breakerTierNone breakerTier = iota
	// breakerTierHard 是配置或账号层面的硬伤：一次就关，且关得久。
	breakerTierHard
	// breakerTierRateLimited 是上游明确限流：一次就关，时长听上游的 Retry-After。
	breakerTierRateLimited
	// breakerTierTransient 是疑似偶发抽风：连续两次才关，关一会儿就放出来。
	breakerTierTransient
)

// hardFailureCodes 是硬伤档的传输与解析层错误码：
// 连接层根本没通（拒绝、域名、证书）或响应解析不出内容时，重试同一个成员只是白等。
var hardFailureCodes = []string{
	"network.connection_refused",
	"network.dns_failed",
	"network.tls_failed",
	"provider.parse_failed",
}

// transientFailureCodes 是偶发档的错误码：偶发的超时与断连，这一次踩上了下一次可能就通，
// 连着两次才值得关人。
// provider.network_failed 是不带更具体成因的"这一趟没打通"：无法证明是硬伤，
// 按偶发处理——既换人也给它下一次机会。
var transientFailureCodes = []string{
	"network.timeout",
	"network.connection_lost",
	"network.connection_failed",
	"network.request_failed",
	"provider.network_failed",
}

// memberFailureGrade 是对一次请求失败的判定，两件事一次算清：
// 这次失败该不该记在成员账上（attributable），以及该按哪一档记进熔断表（tier）。
type memberFailureGrade struct {
	// attributable 表示这是成员自己的责任：可以顺位换下一个成员去试。
	// 请求本身有问题（参数错、模型不存在、调用方已取消）时为 false——换谁都是同一个错。
	attributable bool
	// tier 说明该按哪一档熔断；breakerTierNone 表示这次只换人、不关人。
	tier breakerTier
	// retryAfter 是上游明确要求等待的时长，只有限流档会带。
	retryAfter time.Duration
}

// gradeMemberFailure 是一次失败的唯一判定入口：换不换人、关不关人都在这里定，
// 调用方不再各自解析错误内容，避免同一套规则在几处长歪。
// 先看上游状态码（凭据、限流、服务端错误都有明确档位），没有状态码时再看错误码
// （传输层与解析层的失败只有码，没有状态码）。
func gradeMemberFailure(err error) memberFailureGrade {
	if err == nil || errors.Is(err, context.Canceled) {
		return memberFailureGrade{}
	}
	payload := apperrors.BuildErrorPayload(err)
	if payload == nil {
		return memberFailureGrade{}
	}
	if status, ok := apperrors.ProviderStatusCode(payload); ok {
		return gradeStatusFailure(status, payload)
	}
	return gradeCodeFailure(payload)
}

// gradeStatusFailure 按上游状态码定档：状态码是上游给出的最明确态度。
func gradeStatusFailure(status int, payload *types.ErrorPayload) memberFailureGrade {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		// 凭据失效：不换 Key 之前，这个成员永远会是同样的结果。
		return memberFailureGrade{attributable: true, tier: breakerTierHard}
	case status == http.StatusTooManyRequests:
		// 上游限流：它自己说了要等多久，就等多久。
		retryAfter, _ := apperrors.ProviderRetryAfter(payload)
		return memberFailureGrade{attributable: true, tier: breakerTierRateLimited, retryAfter: retryAfter}
	case status >= http.StatusInternalServerError:
		// 上游自己出问题：也许只是这一下，先记一笔，连着两次才关。
		return memberFailureGrade{attributable: true, tier: breakerTierTransient}
	case status == http.StatusRequestTimeout || status == http.StatusConflict:
		// 上游声明这次请求状态不对，换一个成员往往就过去了；
		// 但成因说不清，不足以据此关人。
		return memberFailureGrade{attributable: true}
	default:
		// 其余 4xx 由我们的请求质量决定，换谁都是同一个错。
		return memberFailureGrade{}
	}
}

// gradeCodeFailure 按错误码定档：走到这里说明失败没带上游状态码。
// 判定顺序固定为硬伤优先——同一棵错误树里既有硬伤码又有偶发码时，按重的那档算。
func gradeCodeFailure(payload *types.ErrorPayload) memberFailureGrade {
	switch {
	case apperrors.HasErrorCode(payload, hardFailureCodes...):
		return memberFailureGrade{attributable: true, tier: breakerTierHard}
	case apperrors.HasErrorCode(payload, transientFailureCodes...):
		return memberFailureGrade{attributable: true, tier: breakerTierTransient}
	default:
		// 剩下的失败（例如我们自己拼错了请求）换成员只是把同一个错误再犯一次。
		return memberFailureGrade{}
	}
}
