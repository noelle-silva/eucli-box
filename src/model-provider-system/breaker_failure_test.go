package modelprovider

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/types"
)

// freezingBreaker 造一个时钟可控的熔断表，便于验证计数窗口与冷却到期。
func freezingBreaker() (*memberBreaker, *time.Time) {
	clock := time.Now()
	breaker := newMemberBreaker()
	breaker.now = func() time.Time { return clock }
	return breaker, &clock
}

// TestGradeMemberFailureByUpstreamStatus 钉住按上游状态码分档：
// 凭据失效一次就关很久，限流听上游的 Retry-After，5xx 连两次才关，
// 请求本身有问题（400、404）连换人都不该触发。
func TestGradeMemberFailureByUpstreamStatus(t *testing.T) {
	cases := []struct {
		name             string
		response         types.HTTPResponse
		wantAttributable bool
		wantTier         breakerTier
		wantRetryAfter   time.Duration
	}{
		{"凭据失效 401", openAIFailureResponse(http.StatusUnauthorized), true, breakerTierHard, 0},
		{"凭据失效 403", openAIFailureResponse(http.StatusForbidden), true, breakerTierHard, 0},
		{"限流 429 带 Retry-After", openAIRateLimitResponse("90"), true, breakerTierRateLimited, 90 * time.Second},
		{"限流 429 未给 Retry-After", openAIFailureResponse(http.StatusTooManyRequests), true, breakerTierRateLimited, 0},
		{"上游 500", openAIFailureResponse(http.StatusInternalServerError), true, breakerTierTransient, 0},
		{"上游 503", openAIFailureResponse(http.StatusServiceUnavailable), true, breakerTierTransient, 0},
		{"上游 408", openAIFailureResponse(http.StatusRequestTimeout), true, breakerTierNone, 0},
		{"上游 409", openAIFailureResponse(http.StatusConflict), true, breakerTierNone, 0},
		{"请求参数错 400", openAIFailureResponse(http.StatusBadRequest), false, breakerTierNone, 0},
		{"模型不存在 404", openAIFailureResponse(http.StatusNotFound), false, breakerTierNone, 0},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// 走真实链路：适配器就是用 requireSuccess 把上游响应翻成失败的。
			grade := gradeMemberFailure(requireSuccess(testCase.response))
			if grade.attributable != testCase.wantAttributable {
				t.Fatalf("attributable = %v, want %v", grade.attributable, testCase.wantAttributable)
			}
			if grade.tier != testCase.wantTier {
				t.Fatalf("tier = %v, want %v", grade.tier, testCase.wantTier)
			}
			if grade.retryAfter != testCase.wantRetryAfter {
				t.Fatalf("retryAfter = %v, want %v", grade.retryAfter, testCase.wantRetryAfter)
			}
		})
	}
}

// TestGradeMemberFailureByErrorCode 钉住按错误码分档：
// 连接层硬伤与解析失败一次就关很久，偶发超时断连连两次才关，
// 我们自己拼错请求或调用方已取消不算成员的责任。
func TestGradeMemberFailureByErrorCode(t *testing.T) {
	networkFailure := func(code string) error {
		return apperrors.Wrap("network-request-system", code, "transport failed", nil)
	}
	cases := []struct {
		name             string
		err              error
		wantAttributable bool
		wantTier         breakerTier
	}{
		{"连接被拒", networkFailure("network.connection_refused"), true, breakerTierHard},
		{"域名解析失败", networkFailure("network.dns_failed"), true, breakerTierHard},
		{"证书校验失败", networkFailure("network.tls_failed"), true, breakerTierHard},
		{"响应解析不出内容", apperrors.Wrap(systemName, "provider.parse_failed", "bad body", nil), true, breakerTierHard},
		{"传输超时", networkFailure("network.timeout"), true, breakerTierTransient},
		{"连接中断", networkFailure("network.connection_lost"), true, breakerTierTransient},
		{"连接失败", networkFailure("network.connection_failed"), true, breakerTierTransient},
		{"请求失败", networkFailure("network.request_failed"), true, breakerTierTransient},
		{"没有具体成因的网络失败", providerNetworkFailed("failed to stream provider completion", errors.New("connection reset by peer")), true, breakerTierTransient},
		{"硬伤码藏在包装层之下", providerNetworkFailed("failed to request provider completion", networkFailure("network.connection_refused")), true, breakerTierHard},
		{"我们自己拼错了请求", networkFailure("network.invalid_request"), false, breakerTierNone},
		{"调用方已取消", providerNetworkFailed("failed to request provider completion", context.Canceled), false, breakerTierNone},
		{"没有失败", nil, false, breakerTierNone},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			grade := gradeMemberFailure(testCase.err)
			if grade.attributable != testCase.wantAttributable {
				t.Fatalf("attributable = %v, want %v", grade.attributable, testCase.wantAttributable)
			}
			if grade.tier != testCase.wantTier {
				t.Fatalf("tier = %v, want %v", grade.tier, testCase.wantTier)
			}
		})
	}
}

// TestMemberBreakerTransientTripsOnSecondStrike 钉住偶发档：一次失败只记一笔，
// 连着第二次才关人，关够默认时长自己放出来。
func TestMemberBreakerTransientTripsOnSecondStrike(t *testing.T) {
	breaker, clock := freezingBreaker()
	key := memberKey("provider-0", "gpt-4.1")
	transient := memberFailureGrade{attributable: true, tier: breakerTierTransient}

	breaker.recordFailure(key, transient)
	if breaker.isCooling(key) {
		t.Fatalf("偶发档第一次失败不该关人")
	}
	breaker.recordFailure(key, transient)
	if !breaker.isCooling(key) {
		t.Fatalf("偶发档连续两次失败应当关人")
	}
	*clock = clock.Add(breakerCooldown + time.Second)
	if breaker.isCooling(key) {
		t.Fatalf("关够时长后应当放人")
	}
}

// TestMemberBreakerStrikeWindowExpires 钉住计数窗口：间隔超过窗口的两条失败不算"连着"，
// 旧计数作废后重新从第一次算起。
func TestMemberBreakerStrikeWindowExpires(t *testing.T) {
	breaker, clock := freezingBreaker()
	key := memberKey("provider-0", "gpt-4.1")
	transient := memberFailureGrade{attributable: true, tier: breakerTierTransient}

	breaker.recordFailure(key, transient)
	*clock = clock.Add(breakerStrikeWindow + time.Second)
	breaker.recordFailure(key, transient)
	if breaker.isCooling(key) {
		t.Fatalf("窗口外的旧计数必须作废，单独一次失败不该关人")
	}
	breaker.recordFailure(key, transient)
	if !breaker.isCooling(key) {
		t.Fatalf("窗口内的两次失败应当关人")
	}
}

// TestMemberBreakerRateLimitHonorsRetryAfter 钉住限流档：上游给了等待时长就按它关，
// 没给（或给了 0）就退回默认时长。
func TestMemberBreakerRateLimitHonorsRetryAfter(t *testing.T) {
	t.Run("上游给了 Retry-After", func(t *testing.T) {
		breaker, clock := freezingBreaker()
		key := memberKey("provider-0", "gpt-4.1")
		breaker.recordFailure(key, memberFailureGrade{attributable: true, tier: breakerTierRateLimited, retryAfter: 90 * time.Second})

		*clock = clock.Add(89 * time.Second)
		if !breaker.isCooling(key) {
			t.Fatalf("上游要求等 90 秒，第 89 秒不该放人")
		}
		*clock = clock.Add(2 * time.Second)
		if breaker.isCooling(key) {
			t.Fatalf("超过上游给的等待时长后应当放人")
		}
	})

	t.Run("上游没给 Retry-After", func(t *testing.T) {
		breaker, clock := freezingBreaker()
		key := memberKey("provider-0", "gpt-4.1")
		breaker.recordFailure(key, memberFailureGrade{attributable: true, tier: breakerTierRateLimited})

		*clock = clock.Add(breakerCooldown - time.Second)
		if !breaker.isCooling(key) {
			t.Fatalf("没给等待时长时应当关满默认时长")
		}
		*clock = clock.Add(2 * time.Second)
		if breaker.isCooling(key) {
			t.Fatalf("默认时长到期后应当放人")
		}
	})
}

// TestMemberBreakerHardCooldownExpires 钉住硬伤档一次就关、关满 5 分钟，
// 以及成功响应会把连续失败计数一起抹掉。
func TestMemberBreakerHardCooldownExpires(t *testing.T) {
	breaker, clock := freezingBreaker()
	key := memberKey("provider-0", "gpt-4.1")
	hard := memberFailureGrade{attributable: true, tier: breakerTierHard}

	breaker.recordFailure(key, hard)
	if !breaker.isCooling(key) {
		t.Fatalf("硬伤档一次失败就应当关人")
	}
	if breaker.isCooling("") {
		t.Fatalf("空 key 不应参与熔断")
	}
	*clock = clock.Add(breakerHardCooldown - time.Second)
	if !breaker.isCooling(key) {
		t.Fatalf("硬伤档应当关满 %v", breakerHardCooldown)
	}
	*clock = clock.Add(2 * time.Second)
	if breaker.isCooling(key) {
		t.Fatalf("硬伤档关满时长后应当放人")
	}
	if _, ok := breaker.openUntil[key]; ok {
		t.Fatalf("到期记录没有被清掉")
	}

	// 成功响应既解冷却，也把攒着的连续失败计数清零。
	transient := memberFailureGrade{attributable: true, tier: breakerTierTransient}
	breaker.recordFailure(key, transient)
	breaker.clear(key)
	breaker.recordFailure(key, transient)
	if breaker.isCooling(key) {
		t.Fatalf("成功清零后单独一次失败不该关人")
	}
}

// TestMemberBreakerCooldownOnlyExtends 钉住冷却只延长不缩短：
// 已在硬伤档关了 5 分钟的成员，不该被后来一次偶发失败降格成 30 秒。
func TestMemberBreakerCooldownOnlyExtends(t *testing.T) {
	breaker, clock := freezingBreaker()
	key := memberKey("provider-0", "gpt-4.1")

	breaker.recordFailure(key, memberFailureGrade{attributable: true, tier: breakerTierHard})
	// 全员冷却时仍会去试探，这一次到来的是偶发失败，冷却不该因此变短。
	breaker.recordFailure(key, memberFailureGrade{attributable: true, tier: breakerTierTransient})
	breaker.recordFailure(key, memberFailureGrade{attributable: true, tier: breakerTierTransient})

	*clock = clock.Add(breakerHardCooldown - time.Second)
	if !breaker.isCooling(key) {
		t.Fatalf("硬伤档的冷却不该被后来的偶发失败缩短")
	}
	*clock = clock.Add(2 * time.Second)
	if breaker.isCooling(key) {
		t.Fatalf("硬伤档关满时长后应当放人")
	}
}
