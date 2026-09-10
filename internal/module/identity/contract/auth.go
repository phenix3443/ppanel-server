package dto

type AppleLoginCallbackRequest struct {
	Code    string `form:"code"`
	IDToken string `form:"id_token"`
	State   string `form:"state"`
}

type AuthMethodConfig struct {
	Id      int64       `json:"id"`
	Method  string      `json:"method"`
	Config  interface{} `json:"config"`
	Enabled bool        `json:"enabled"`
}

type BindOAuthCallbackRequest struct {
	Method   string      `json:"method" validate:"required,oneof=google apple telegram github"`
	Callback interface{} `json:"callback" validate:"required"`
}

type BindOAuthRequest struct {
	Method   string `json:"method" validate:"required,oneof=google apple telegram github"`
	Redirect string `json:"redirect" validate:"required"`
}

type BindOAuthResponse struct {
	Redirect string `json:"redirect"`
}

type BindTelegramResponse struct {
	Url       string `json:"url"`
	ExpiredAt int64  `json:"expired_at"`
}

type CheckUserRequest struct {
	Email string `form:"email" validate:"required,email"`
}

type CheckUserResponse struct {
	Exist bool `json:"exist"`
}

type CheckVerificationCodeRequest struct {
	Method  string `json:"method" validate:"required,oneof=email mobile"`
	Account string `json:"account" validate:"required"`
	Code    string `json:"code" validate:"required"`
	Type    uint8  `json:"type" validate:"required,oneof=1 2"`
}

type CheckVerificationCodeRespone struct {
	Status bool `json:"status"`
}

type DeviceLoginRequest struct {
	Identifier string `json:"identifier" validate:"required,max=255"`
	Invite     string `json:"invite,optional"`
	IP         string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent  string `header:"User-Agent" json:"-" swaggerignore:"true"`
	CfToken    string `json:"cf_token,optional"`
}

type GetAuthMethodConfigRequest struct {
	Method string `form:"method"`
}

type GetAuthMethodListResponse struct {
	List []AuthMethodConfig `json:"list"`
}

type GetOAuthMethodsResponse struct {
	Methods []UserAuthMethod `json:"methods"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

type OAthLoginRequest struct {
	Method   string `json:"method" validate:"required"` // google, facebook, apple, telegram, github etc.
	Redirect string `json:"redirect"`
}

type OAuthLoginGetTokenRequest struct {
	Method   string      `json:"method" validate:"required"` // google, facebook, apple, telegram, github etc.
	Callback interface{} `json:"callback" validate:"required"`
	Invite   string      `json:"invite,optional"`
	CfToken  string      `json:"cf_token,optional"`
}

type OAuthLoginResponse struct {
	Redirect string `json:"redirect"`
}

type ResetPasswordRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=8,max=128"`
	Code       string `json:"code,optional"`
	IP         string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent  string `header:"User-Agent" swaggerignore:"true"`
	LoginType  string `header:"Login-Type" swaggerignore:"true"`
	CfToken    string `json:"cf_token,optional"`
}

type SendCodeRequest struct {
	Email string `json:"email" validate:"required,email"`
	Type  uint8  `json:"type" validate:"required,oneof=1 2"`
}

type SendCodeResponse struct {
	Code   string `json:"code,omitempty"`
	Status bool   `json:"status"`
}

type SendSmsCodeRequest struct {
	Type              uint8  `json:"type" validate:"required,oneof=1 2"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
}

type TelephoneCheckUserRequest struct {
	Telephone         string `form:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
}

type TelephoneCheckUserResponse struct {
	Exist bool `json:"exist"`
}

type TelephoneLoginRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneCode     string `json:"telephone_code"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password"`
	IP                string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent         string `header:"User-Agent" swaggerignore:"true"`
	LoginType         string `header:"Login-Type" swaggerignore:"true"`
	CfToken           string `json:"cf_token,optional"`
}

type TelephoneRegisterRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password" validate:"required,min=8,max=128"`
	Invite            string `json:"invite,optional"`
	Code              string `json:"code,optional"`
	IP                string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent         string `header:"User-Agent" swaggerignore:"true"`
	LoginType         string `header:"Login-Type,optional" swaggerignore:"true"`
	CfToken           string `json:"cf_token,optional"`
}

type TelephoneResetPasswordRequest struct {
	Identifier        string `json:"identifier"`
	Telephone         string `json:"telephone" validate:"required"`
	TelephoneAreaCode string `json:"telephone_area_code" validate:"required"`
	Password          string `json:"password" validate:"required,min=8,max=128"`
	Code              string `json:"code,optional"`
	IP                string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent         string `header:"User-Agent" swaggerignore:"true"`
	LoginType         string `header:"Login-Type,optional" swaggerignore:"true"`
	CfToken           string `json:"cf_token,optional"`
}

type TestEmailSendRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type TestSmsSendRequest struct {
	AreaCode  string `json:"area_code" validate:"required"`
	Telephone string `json:"telephone" validate:"required"`
}

type UnbindOAuthRequest struct {
	Method string `json:"method"`
}

type UpdateAuthMethodConfigRequest struct {
	Id      int64       `json:"id"`
	Method  string      `json:"method"`
	Config  interface{} `json:"config"`
	Enabled *bool       `json:"enabled"`
}

type UserLoginRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required"`
	IP         string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent  string `header:"User-Agent" swaggerignore:"true"`
	LoginType  string `header:"Login-Type" swaggerignore:"true"`
	CfToken    string `json:"cf_token,optional"`
}

type UserRegisterRequest struct {
	Identifier string `json:"identifier"`
	Email      string `json:"email" validate:"required,email"`
	Password   string `json:"password" validate:"required,min=8,max=128"`
	Invite     string `json:"invite,optional"`
	Code       string `json:"code,optional"`
	IP         string `header:"X-Original-Forwarded-For" swaggerignore:"true"`
	UserAgent  string `header:"User-Agent" swaggerignore:"true"`
	LoginType  string `header:"Login-Type" swaggerignore:"true"`
	CfToken    string `json:"cf_token,optional"`
}

type VerifyEmailRequest struct {
	Email string `json:"email" validate:"required,email"`
	Code  string `json:"code" validate:"required"`
}
