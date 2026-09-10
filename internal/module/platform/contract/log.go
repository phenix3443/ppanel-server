package dto

type BalanceLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	Amount           int64  `json:"amount"`
	OrderNo          string `json:"order_no,omitempty"`
	Balance          int64  `json:"balance"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type CommissionLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	Amount           int64  `json:"amount"`
	OrderNo          string `json:"order_no"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type FilterBalanceLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterBalanceLogResponse struct {
	Total int64        `json:"total"`
	List  []BalanceLog `json:"list"`
}

type FilterCommissionLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterCommissionLogResponse struct {
	Total int64           `json:"total"`
	List  []CommissionLog `json:"list"`
}

type FilterEmailLogResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type FilterGiftLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterGiftLogResponse struct {
	Total int64     `json:"total"`
	List  []GiftLog `json:"list"`
}

type FilterLogParams struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Date   string `form:"date,optional"`
	Search string `form:"search,optional"`
}

type FilterLoginLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterLoginLogResponse struct {
	Total int64      `json:"total"`
	List  []LoginLog `json:"list"`
}

type FilterMobileLogResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type FilterOrderLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterOrderLogResponse struct {
	Total int64      `json:"total"`
	List  []OrderLog `json:"list"`
}

type FilterRegisterLogRequest struct {
	FilterLogParams
	UserId int64 `form:"user_id,optional"`
}

type FilterRegisterLogResponse struct {
	Total int64         `json:"total"`
	List  []RegisterLog `json:"list"`
}

type FilterResetSubscribeLogRequest struct {
	FilterLogParams
	UserSubscribeId int64 `form:"user_subscribe_id,optional"`
}

type FilterResetSubscribeLogResponse struct {
	Total int64               `json:"total"`
	List  []ResetSubscribeLog `json:"list"`
}

type FilterSubscribeLogRequest struct {
	FilterLogParams
	UserId          int64 `form:"user_id,optional"`
	UserSubscribeId int64 `form:"user_subscribe_id,optional"`
}

type FilterSubscribeLogResponse struct {
	Total int64          `json:"total"`
	List  []SubscribeLog `json:"list"`
}

type GetMessageLogListRequest struct {
	Page   int    `form:"page" validate:"required,gt=0"`
	Size   int    `form:"size" validate:"required,gt=0,lte=100"`
	Type   uint8  `form:"type" validate:"required,oneof=10 11"`
	Search string `form:"search,optional"`
}

type GetMessageLogListResponse struct {
	Total int64        `json:"total"`
	List  []MessageLog `json:"list"`
}

type GiftLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	OrderNo          string `json:"order_no"`
	SubscribeId      int64  `json:"subscribe_id"`
	Amount           int64  `json:"amount"`
	Balance          int64  `json:"balance"`
	Remark           string `json:"remark,omitempty"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type LogResponse struct {
	List interface{} `json:"list"`
}

type LogSetting struct {
	AutoClear *bool `json:"auto_clear" validate:"required"`
	ClearDays int64 `json:"clear_days" validate:"required,gte=1,lte=3650"`
}

type LoginLog struct {
	UserId           int64  `json:"user_id"`
	Method           string `json:"method"`
	LoginIP          string `json:"login_ip"`
	UserAgent        string `json:"user_agent"`
	Success          bool   `json:"success"`
	Timestamp        int64  `json:"timestamp"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type MessageLog struct {
	Id               int64       `json:"id"`
	Type             uint8       `json:"type"`
	Platform         string      `json:"platform"`
	To               string      `json:"to"`
	Subject          string      `json:"subject"`
	Content          interface{} `json:"content"`
	Status           uint8       `json:"status"`
	CreatedAt        int64       `json:"created_at"`
	ClientIP         string      `json:"client_ip,omitempty"`
	UserAgent        string      `json:"user_agent,omitempty"`
	ActorID          int64       `json:"actor_id,omitempty"`
	IPCountryCode    string      `json:"ip_country_code,omitempty"`
	IPCountry        string      `json:"ip_country,omitempty"`
	IPRegion         string      `json:"ip_region,omitempty"`
	IPCity           string      `json:"ip_city,omitempty"`
	IPASN            uint        `json:"ip_asn,omitempty"`
	IPASOrganization string      `json:"ip_as_organization,omitempty"`
}

type OrderLog struct {
	Id               int64  `json:"id"`
	UserId           int64  `json:"user_id"`
	OrderNo          string `json:"order_no"`
	OrderType        uint8  `json:"order_type"`
	Quantity         int64  `json:"quantity"`
	Price            int64  `json:"price"`
	Amount           int64  `json:"amount"`
	GiftAmount       int64  `json:"gift_amount"`
	Discount         int64  `json:"discount"`
	CouponDiscount   int64  `json:"coupon_discount"`
	PaymentId        int64  `json:"payment_id"`
	Method           string `json:"method"`
	FeeAmount        int64  `json:"fee_amount"`
	SubscribeId      int64  `json:"subscribe_id,omitempty"`
	Source           string `json:"source"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type RegisterLog struct {
	UserId           int64  `json:"user_id"`
	AuthMethod       string `json:"auth_method"`
	Identifier       string `json:"identifier"`
	RegisterIP       string `json:"register_ip"`
	UserAgent        string `json:"user_agent"`
	Timestamp        int64  `json:"timestamp"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}

type ResetSubscribeLog struct {
	Type             uint16 `json:"type"`
	UserId           int64  `json:"user_id"`
	UserSubscribeId  int64  `json:"user_subscribe_id"`
	OrderNo          string `json:"order_no,omitempty"`
	Timestamp        int64  `json:"timestamp"`
	ClientIP         string `json:"client_ip,omitempty"`
	UserAgent        string `json:"user_agent,omitempty"`
	ActorID          int64  `json:"actor_id,omitempty"`
	IPCountryCode    string `json:"ip_country_code,omitempty"`
	IPCountry        string `json:"ip_country,omitempty"`
	IPRegion         string `json:"ip_region,omitempty"`
	IPCity           string `json:"ip_city,omitempty"`
	IPASN            uint   `json:"ip_asn,omitempty"`
	IPASOrganization string `json:"ip_as_organization,omitempty"`
}
