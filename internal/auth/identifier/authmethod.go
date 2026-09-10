package identifier

import "strings"

const (
	Email  = "email"  //邮箱
	Mobile = "mobile" //手机
	Device = "device" //设备

)

func CanonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CanonicalIdentifier(authType, identifier string) string {
	if authType == Email {
		return CanonicalEmail(identifier)
	}
	return identifier
}
