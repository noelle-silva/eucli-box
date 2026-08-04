package localrun

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"eucli-box/pkg/release"
)

const (
	RegistrationSchemaVersion = 1
	RegistrationStatusRunning = "running"
	RegistrationStatusStale   = "stale"
)

type Registration struct {
	SchemaVersion     int       `json:"schemaVersion"`
	InstallIdentity   string    `json:"installIdentity"`
	DataIdentity      string    `json:"dataIdentity"`
	RunIdentity       string    `json:"runIdentity"`
	Endpoint          string    `json:"endpoint"`
	SessionCredential string    `json:"sessionCredential"`
	ProcessID         int       `json:"processId"`
	ProcessStartedAt  time.Time `json:"processStartedAt"`
	BoxVersion        string    `json:"boxVersion"`
	Status            string    `json:"status"`
}

type RegistrationFacts struct {
	InstallIdentity   string
	DataIdentity      string
	RunIdentity       string
	Endpoint          string
	SessionCredential string
	ProcessID         int
	ProcessStartedAt  time.Time
	BoxVersion        string
}

type DataIdentityRecord struct {
	SchemaVersion int       `json:"schemaVersion"`
	DataIdentity  string    `json:"dataIdentity"`
	CreatedBy     string    `json:"createdBy"`
	CreatedAt     time.Time `json:"createdAt"`
}

func ValidateRegistration(value Registration) error {
	if value.SchemaVersion != RegistrationSchemaVersion {
		return fmt.Errorf("运行登记 schemaVersion 无效")
	}
	if err := ValidateIdentity(value.InstallIdentity, IdentityKindInstall); err != nil {
		return err
	}
	if err := ValidateIdentity(value.DataIdentity, IdentityKindData); err != nil {
		return err
	}
	if err := ValidateIdentity(value.RunIdentity, IdentityKindRun); err != nil {
		return err
	}
	if err := ValidateIdentity(value.SessionCredential, IdentityKindSession); err != nil {
		return err
	}
	if err := validateLocalEndpoint(value.Endpoint); err != nil {
		return err
	}
	if value.ProcessID <= 0 || value.ProcessStartedAt.IsZero() {
		return fmt.Errorf("运行登记进程事实无效")
	}
	if err := validateVersion(value.BoxVersion); err != nil {
		return err
	}
	if value.Status != RegistrationStatusRunning && value.Status != RegistrationStatusStale {
		return fmt.Errorf("运行登记状态无效")
	}
	return nil
}

func ReadRegistration(path string) (Registration, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return Registration{}, fmt.Errorf("读取运行登记失败：%w", err)
	}
	var value Registration
	if err := decodeStrict(payload, &value); err != nil {
		return Registration{}, fmt.Errorf("运行登记资料无效：%w", err)
	}
	if err := ValidateRegistration(value); err != nil {
		return Registration{}, err
	}
	return value, nil
}

func ReadStrictJSON(path string, target any) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeStrict(payload, target)
}

func WriteRegistration(path string, value Registration) error {
	if err := ValidateRegistration(value); err != nil {
		return err
	}
	return writeProtectedJSON(path, value)
}

func WritePrivateJSON(path string, value any) error {
	return writeProtectedJSON(path, value)
}

func DeleteRegistration(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除运行登记失败：%w", err)
	}
	return nil
}

func MarkRegistrationStale(path string) error {
	value, err := ReadRegistration(path)
	if err != nil {
		return err
	}
	value.Status = RegistrationStatusStale
	return WriteRegistration(path, value)
}

func MatchRegistration(value Registration, facts RegistrationFacts) error {
	if err := ValidateRegistration(value); err != nil {
		return err
	}
	if value.Status != RegistrationStatusRunning {
		return fmt.Errorf("运行登记不是运行状态")
	}
	if value.InstallIdentity != facts.InstallIdentity || value.DataIdentity != facts.DataIdentity || value.RunIdentity != facts.RunIdentity || value.Endpoint != facts.Endpoint || value.SessionCredential != facts.SessionCredential || value.ProcessID != facts.ProcessID || !value.ProcessStartedAt.Equal(facts.ProcessStartedAt) || value.BoxVersion != facts.BoxVersion {
		return fmt.Errorf("运行登记事实不匹配")
	}
	return nil
}

func ValidateDataIdentity(value DataIdentityRecord) error {
	if value.SchemaVersion != RegistrationSchemaVersion {
		return fmt.Errorf("数据身份 schemaVersion 无效")
	}
	if err := ValidateIdentity(value.DataIdentity, IdentityKindData); err != nil {
		return err
	}
	if strings.TrimSpace(value.CreatedBy) != "eucli-studio" || value.CreatedAt.IsZero() {
		return fmt.Errorf("数据身份资料无效")
	}
	return nil
}

func ReadDataIdentity(dataDir string) (DataIdentityRecord, error) {
	path := filepath.Join(dataDir, "meta", "local-identity.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		return DataIdentityRecord{}, fmt.Errorf("读取数据身份失败：%w", err)
	}
	var value DataIdentityRecord
	if err := decodeStrict(payload, &value); err != nil {
		return DataIdentityRecord{}, fmt.Errorf("数据身份资料无效：%w", err)
	}
	if err := ValidateDataIdentity(value); err != nil {
		return DataIdentityRecord{}, err
	}
	return value, nil
}

func EnsureDataIdentity(dataDir string) (DataIdentityRecord, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil || strings.TrimSpace(dataDir) == "" {
		return DataIdentityRecord{}, fmt.Errorf("数据目录无效")
	}
	absolute = filepath.Clean(absolute)
	identityPath := filepath.Join(absolute, "meta", "local-identity.json")
	if _, err := os.Stat(identityPath); err == nil {
		return ReadDataIdentity(absolute)
	} else if !errors.Is(err, os.ErrNotExist) {
		return DataIdentityRecord{}, err
	}
	if err := ensureEmptyDataDirectory(absolute); err != nil {
		return DataIdentityRecord{}, err
	}
	identity, err := NewIdentity(IdentityKindData)
	if err != nil {
		return DataIdentityRecord{}, err
	}
	value := DataIdentityRecord{SchemaVersion: RegistrationSchemaVersion, DataIdentity: identity, CreatedBy: "eucli-studio", CreatedAt: time.Now().UTC()}
	if err := writeProtectedJSON(identityPath, value); err != nil {
		return DataIdentityRecord{}, err
	}
	return value, nil
}

func RegistrationPath(runtimeDir string) string {
	return filepath.Join(runtimeDir, "registration.json")
}

func DataLockPath(dataDir string) (string, error) {
	absolute, err := filepath.Abs(strings.TrimSpace(dataDir))
	if err != nil || strings.TrimSpace(dataDir) == "" {
		return "", fmt.Errorf("数据目录无效")
	}
	return filepath.Join(filepath.Dir(filepath.Clean(absolute)), "data.lock"), nil
}

func decodeStrict(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return err
	}
	return fmt.Errorf("资料包含多余内容")
}

func writeProtectedJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("生成资料失败：%w", err)
	}
	payload = append(payload, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("建立资料目录失败：%w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".localrun-*.tmp")
	if err != nil {
		return fmt.Errorf("建立资料临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置资料权限失败：%w", err)
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入资料失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("刷新资料失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭资料失败：%w", err)
	}
	if err := ProtectFileForCurrentUser(temporaryPath); err != nil {
		return err
	}
	if err := atomicReplace(temporaryPath, path); err != nil {
		return fmt.Errorf("原子替换资料失败：%w", err)
	}
	return ProtectFileForCurrentUser(path)
}

func ensureEmptyDataDirectory(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("建立数据目录失败：%w", err)
	}
	err := filepath.WalkDir(dataDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("数据目录不能包含符号链接")
		}
		if !entry.IsDir() {
			return fmt.Errorf("LOCAL_BOX_DATA_IDENTITY_MISSING")
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("读取数据目录失败：%w", err)
	}
	return nil
}

func validateLocalEndpoint(value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("运行登记地址无效")
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil || host != "127.0.0.1" || port == "" {
		return fmt.Errorf("运行登记地址必须是本机回环地址")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("运行登记端口无效")
	}
	return nil
}

func validateVersion(value string) error {
	if err := release.ValidateVersion(value); err != nil {
		return fmt.Errorf("业务端版本无效：%w", err)
	}
	return nil
}
