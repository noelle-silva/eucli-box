package modelprovider

import (
	"strings"

	apperrors "eucli-box/pkg/errors"
)

// failedMember 记录兜底链上一个成员的失败。
type failedMember struct {
	label string
	err   error
}

// aggregateMemberFailures 把整条兜底链的失败合成一个错误：
// 顶层说明"链已试完"，每个成员各自的失败挂成并列原因，便于一眼看出谁坏在哪。
func aggregateMemberFailures(failures []failedMember) error {
	labels := make([]string, 0, len(failures))
	causes := make([]error, 0, len(failures))
	for _, failure := range failures {
		labels = append(labels, failure.label)
		causes = append(causes, failure.err)
	}
	message := "model group fallback exhausted: " + strings.Join(labels, ", ")
	return apperrors.WrapWithCauses(systemName, apperrors.CodeProviderModelGroupExhausted, message, causes)
}
