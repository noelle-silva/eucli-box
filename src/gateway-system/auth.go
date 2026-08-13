package gateway

import (
	"context"
	"net"
	"net/http"
	"strings"

	apperrors "eucli-box/pkg/errors"
	"eucli-box/pkg/release"
	"eucli-box/pkg/types"
)

const (
	clientVersionHeader        = "X-Eucli-Studio-Version"
	clientMinimumVersionHeader = "X-Eucli-Studio-Minimum-Box-Version"
	clientMaximumVersionHeader = "X-Eucli-Studio-Maximum-Box-Version"
)

type gatewayContextKey string

// contextKeyAuthenticatedKeyID 在请求上下文中携带已通过长期 Key 核对的 Key ID。
// 只有长期端口入口会设置；受托本机入口和普通入口不携带。
const contextKeyAuthenticatedKeyID gatewayContextKey = "access-system-authenticated-key-id"

// requireTrustedConnection 是访问设置管理路由的身份边界：
// 只有本次受托启动交接凭证可以管理访问设置；长期 Key 被明确拒绝。
func (s *system) requireTrustedConnection(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.LocalRun {
			writeError(w, gatewayForbidden("访问设置只允许受托本机连接管理", nil))
			return
		}
		if !requestFromLoopback(r) {
			writeError(w, gatewayForbidden("访问设置只接受本机回环连接", nil))
			return
		}
		provided := strings.TrimSpace(extractAuthorizationBearer(r))
		if provided == strings.TrimSpace(s.config.LocalCredential) {
			next(w, r)
			return
		}
		if s.access != nil && s.access.VerifyKey(r.Context(), provided).Valid {
			writeError(w, gatewayForbidden("长期 Key 无权管理访问设置", nil))
			return
		}
		writeError(w, gatewayNotAuthorized("受托本机凭证不匹配", nil))
	}
}

// longTermAuthWrap 是长期端口连接处理入口的鉴权包装：
// 提取请求 Key，通过长期 Key 核对后才转发到统一业务处理；验证失败立即返回 401。
// 核对通过后在请求上下文中携带 Key ID，供持续连接登记和结束使用。
func (s *system) longTermAuthWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.access == nil {
			writeError(w, gatewayDependencyFailed("长期访问系统未初始化", nil))
			return
		}
		provided := strings.TrimSpace(extractRequestKey(r))
		result := s.access.VerifyKey(r.Context(), provided)
		if !result.Valid {
			writeError(w, gatewayNotAuthorized("eucli-box long-term key mismatch", nil))
			return
		}
		if !releaseMaintenancePath(r.URL.Path) {
			if err := s.validateClientCompatibility(r); err != nil {
				writeError(w, err)
				return
			}
		}
		ctx := context.WithValue(r.Context(), contextKeyAuthenticatedKeyID, result.KeyID)
		next(w, r.WithContext(ctx))
	}
}

// longTermKeyIDFromContext 返回请求上下文中的长期 Key ID；非长期入口请求为空。
func longTermKeyIDFromContext(r *http.Request) string {
	if r == nil {
		return ""
	}
	value, _ := r.Context().Value(contextKeyAuthenticatedKeyID).(string)
	return strings.TrimSpace(value)
}

func (s *system) authWrap(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := s.validateRequestKey(r); err != nil {
			writeError(w, err)
			return
		}
		if !releaseMaintenancePath(r.URL.Path) {
			if err := s.validateClientCompatibility(r); err != nil {
				writeError(w, err)
				return
			}
		}
		next(w, r)
	}
}

func releaseMaintenancePath(path string) bool {
	switch path {
	case "/api/release", "/api/release-checks", "/api/release-checks/refresh":
		return true
	default:
		return false
	}
}

func (s *system) clientCompatibility(r *http.Request) *types.CompatibilityStatus {
	version := strings.TrimSpace(r.Header.Get(clientVersionHeader))
	minimum := strings.TrimSpace(r.Header.Get(clientMinimumVersionHeader))
	maximum := strings.TrimSpace(r.Header.Get(clientMaximumVersionHeader))
	if version == "" && minimum == "" && maximum == "" {
		return nil
	}
	compatibility := types.EucliBoxCompatibility{MinimumVersion: minimum, MaximumVersionExclusive: maximum}
	status := release.AssessEucliBoxCompatibility(version, s.boxRelease.Version, compatibility)
	return &status
}

func (s *system) validateClientCompatibility(r *http.Request) error {
	status := s.clientCompatibility(r)
	if status == nil || status.Compatible {
		return nil
	}
	return gatewayClientIncompatible(status.Reason, *status)
}

func (s *system) validateRequestKey(r *http.Request) error {
	// 长期端口入口已经完成长期 Key 核对，转发到统一业务路由时不再重复校验固定凭证。
	if longTermKeyIDFromContext(r) != "" {
		return nil
	}
	if s.config.LocalRun {
		if !requestFromLoopback(r) {
			return gatewayNotAuthorized("local gateway only accepts loopback requests", nil)
		}
		if r.URL.Query().Get("token") != "" {
			return gatewayNotAuthorized("local gateway does not accept query credentials", nil)
		}
		if extractAuthorizationBearer(r) != strings.TrimSpace(s.config.LocalCredential) {
			return gatewayNotAuthorized("local session credential mismatch", nil)
		}
		return nil
	}
	key := strings.TrimSpace(s.config.Key)
	if key == "" {
		return nil
	}
	requestKey := extractRequestKey(r)
	if requestKey == key {
		return nil
	}
	return gatewayNotAuthorized("eucli-box key mismatch", nil)
}

func extractRequestKey(r *http.Request) string {
	if value := extractAuthorizationBearer(r); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func extractAuthorizationBearer(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func requestFromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func gatewayNotAuthorized(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.unauthorized", message, cause)
}
