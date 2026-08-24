package gateway

import (
	"context"
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
// 只有长期端口入口会设置；网关直连入口不携带。
const contextKeyAuthenticatedKeyID gatewayContextKey = "access-system-authenticated-key-id"

// requireDirectAccess 是访问设置管理路由的身份边界：
// 只有网关直连入口（通过直连固定 Key 或有效长期 Key 鉴权）可以管理访问设置；
// 长期端口入口是纯业务访问身份，被明确拒绝。
func (s *system) requireDirectAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if longTermKeyIDFromContext(r) != "" {
			writeError(w, gatewayForbidden("长期 Key 无权管理访问设置", nil))
			return
		}
		next(w, r)
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
	// 网关直连入口同时接受两种身份：
	// 1. 直连固定 Key（正式配钥、EUCLI_BOX_KEY 注入的网关身份）；
	// 2. 有效长期 Key（客户端以长期 Key 直连网关的身份）。
	// 两者都未配置时，网关处于未安装身份状态，不做鉴权。
	requestKey := extractRequestKey(r)
	if fixedKey := strings.TrimSpace(s.config.Key); fixedKey != "" {
		if requestKey == fixedKey {
			return nil
		}
	}
	if s.access != nil && s.access.VerifyKey(r.Context(), requestKey).Valid {
		return nil
	}
	if strings.TrimSpace(s.config.Key) == "" && s.access == nil {
		return nil
	}
	if strings.TrimSpace(s.config.Key) == "" && !s.accessHasAnyIdentity(r.Context()) {
		return nil
	}
	return gatewayNotAuthorized("eucli-box key mismatch", nil)
}

// accessHasAnyIdentity 判断访问系统是否已配置了任何长期 Key 记录；
// 未配置任何身份时网关入口保持"未安装身份"状态，允许首次建立连接。
func (s *system) accessHasAnyIdentity(ctx context.Context) bool {
	if s.access == nil {
		return false
	}
	keys, err := s.access.ListKeys(ctx)
	if err != nil {
		return true
	}
	return len(keys) > 0
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

func gatewayNotAuthorized(message string, cause error) error {
	return apperrors.Wrap(systemName, "gateway.unauthorized", message, cause)
}
