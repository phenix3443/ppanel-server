package auth

// VerifyType identifies the purpose shared by identity's verification codes
// and the corresponding notification task.
type VerifyType uint8

const (
	Register VerifyType = iota + 1
	Security
)

func ParseVerifyType(i uint8) VerifyType {
	return VerifyType(i)
}

func (v VerifyType) String() string {
	switch v {
	case Register:
		return "register"
	case Security:
		return "security"
	default:
		return "unknown"
	}
}
