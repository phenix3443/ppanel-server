package identifier

import (
	"strings"
)

func MaskEmail(email string) string {
	atIndex := strings.Index(email, "@")
	if atIndex == -1 || atIndex == 0 || atIndex == len(email)-1 {
		return "***"
	}
	localPart := email[:atIndex]
	domainPart := email[atIndex+1:]
	localRunes := []rune(localPart)

	if len(localRunes) == 1 {
		return "*@" + domainPart
	}
	if len(localRunes) == 2 {
		return string(localRunes[0]) + "*@" + domainPart
	}
	// 替换本地部分中间字符为星号
	maskedLocal := string(localRunes[0]) + strings.Repeat("*", len(localRunes)-2) + string(localRunes[len(localRunes)-1])
	// 返回处理后的邮箱地址
	return maskedLocal + "@" + domainPart
}
