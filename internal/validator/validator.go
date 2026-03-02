package validator

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var nameRE = regexp.MustCompile(`^[a-zA-Z0-9_\-.]+$`)

// ValidateName 校验名称是否满足白名单规则，用于 packageName 和 tableName。
//
// 输入:
//   - name: 待校验的名称字符串
//
// 输出:
//   - error: 不合规时返回具体原因，合规返回 nil
//
// 注意事项:
//   - 允许字符：a-z A-Z 0-9 _ - .
//   - 长度限制：1 ~ 128 字符
func ValidateName(name string) error {
	if len(name) < 1 || len(name) > 128 {
		return fmt.Errorf("name length must be between 1 and 128, got %d", len(name))
	}
	if !nameRE.MatchString(name) {
		return fmt.Errorf("name %q contains invalid characters (allowed: a-z A-Z 0-9 _ - .)", name)
	}
	return nil
}

// ValidateOutputPath 校验输出路径是否在 baseDir 目录内，防止路径穿越攻击。
//
// 输入:
//   - input:   用户提供的输出路径；空字符串表示使用默认路径
//   - baseDir: 输出文件的根目录（来自 --db 文件所在目录）
//
// 输出:
//   - string: 解析后的绝对路径；input 为空时返回 baseDir
//   - error:  路径解析失败或路径穿越时返回原因
func ValidateOutputPath(input, baseDir string) (string, error) {
	if input == "" {
		return baseDir, nil
	}
	abs, err := filepath.Abs(input)
	if err != nil {
		return "", fmt.Errorf("cannot resolve path %q: %w", input, err)
	}
	cleanBase := filepath.Clean(baseDir) + string(filepath.Separator)
	cleanAbs := filepath.Clean(abs) + string(filepath.Separator)
	if !strings.HasPrefix(cleanAbs, cleanBase) {
		return "", fmt.Errorf("path traversal detected: %q is outside base dir %q", abs, baseDir)
	}
	return abs, nil
}
