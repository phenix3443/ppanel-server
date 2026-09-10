// Package usersub holds the subscription domain's user-subscription entities
// (ADR-001 step 5: entity packages follow table ownership).
package usersub

import (
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
)

// Cache key prefixes for the user-subscription cache.
const (
	cacheTokenPrefix = "cache:user:subscribe:token:"
	cacheUserPrefix  = "cache:user:subscribe:user:v3:"
	cacheIdPrefix    = "cache:user:subscribe:id:"
)

type Subscribe struct {
	Id          int64      `gorm:"primaryKey"`
	UserId      int64      `gorm:"index:idx_user_id;not null;comment:User ID"`
	OrderId     int64      `gorm:"index:idx_order_id;not null;comment:Order ID"`
	SubscribeId int64      `gorm:"index:idx_subscribe_id;not null;comment:Subscription ID"`
	StartTime   time.Time  `gorm:"default:CURRENT_TIMESTAMP(3);not null;comment:Subscription Start Time"`
	ExpireTime  time.Time  `gorm:"default:NULL;comment:Subscription Expire Time"`
	FinishedAt  *time.Time `gorm:"default:NULL;comment:Finished Time"`
	Traffic     int64      `gorm:"default:0;comment:Traffic"`
	Download    int64      `gorm:"default:0;comment:Download Traffic"`
	Upload      int64      `gorm:"default:0;comment:Upload Traffic"`
	Token       string     `gorm:"index:idx_token;unique;type:varchar(255);default:'';comment:Token"`
	UUID        string     `gorm:"type:varchar(255);unique;index:idx_uuid;default:'';comment:UUID"`
	Status      uint8      `gorm:"type:tinyint(1);default:0;comment:Subscription Status: 0: Pending 1: Active 2: Finished 3: Expired 4: Deducted 5: stopped"`
	Note        string     `gorm:"type:varchar(500);default:'';comment:User note for subscription"`
	CreatedAt   time.Time  `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt   time.Time  `gorm:"comment:Update Time"`
}

const (
	SubscribeStatusPending uint8 = iota
	SubscribeStatusActive
	SubscribeStatusFinished
	SubscribeStatusExpired
	SubscribeStatusDeducted
	SubscribeStatusStopped
)

func (*Subscribe) TableName() string {
	return "user_subscribe"
}

func (s *Subscribe) GetCacheKeys() []string {
	if s == nil {
		return []string{}
	}
	keys := make([]string, 0)
	if s.Token != "" {
		keys = append(keys, fmt.Sprintf("%s%s", cacheTokenPrefix, s.Token))
	}
	if s.UserId != 0 {
		keys = append(keys, fmt.Sprintf("%s%d", cacheUserPrefix, s.UserId))
	}
	if s.Id != 0 {
		keys = append(keys, fmt.Sprintf("%s%d", cacheIdPrefix, s.Id))
	}
	return keys
}

// SubscribeDetails is the subscription row with its plan association. The
// identity owner is composed at the module layer through the identity read
// port, never through a cross-domain association (ADR-001 step 5).
type SubscribeDetails struct {
	Id          int64                `gorm:"primarykey"`
	UserId      int64                `gorm:"index:idx_user_id;not null;comment:User ID"`
	OrderId     int64                `gorm:"index:idx_order_id;not null;comment:Order ID"`
	SubscribeId int64                `gorm:"index:idx_subscribe_id;not null;comment:Subscription ID"`
	Subscribe   *subscribe.Subscribe `gorm:"foreignKey:SubscribeId;references:Id"`
	StartTime   time.Time            `gorm:"default:CURRENT_TIMESTAMP(3);not null;comment:Subscription Start Time"`
	ExpireTime  time.Time            `gorm:"default:NULL;comment:Subscription Expire Time"`
	FinishedAt  *time.Time           `gorm:"default:NULL;comment:Finished Time"`
	Traffic     int64                `gorm:"default:0;comment:Traffic"`
	Download    int64                `gorm:"default:0;comment:Download Traffic"`
	Upload      int64                `gorm:"default:0;comment:Upload Traffic"`
	Token       string               `gorm:"index:idx_token;unique;type:varchar(255);default:'';comment:Token"`
	UUID        string               `gorm:"type:varchar(255);unique;index:idx_uuid;default:'';comment:UUID"`
	Status      uint8                `gorm:"type:tinyint(1);default:0;comment:Subscription Status: 0: Pending 1: Active 2: Finished 3: Expired; 4: Cancelled"`
	Note        string               `gorm:"type:varchar(500);default:'';comment:User note for subscription"`
	CreatedAt   time.Time            `gorm:"<-:create;comment:Creation Time"`
	UpdatedAt   time.Time            `gorm:"comment:Update Time"`
}

func (*SubscribeDetails) TableName() string {
	return "user_subscribe"
}

// SubscribeLogFilterParams filters user subscribe access logs.
type SubscribeLogFilterParams struct {
	IP              string
	UserAgent       string
	UserId          int64
	Token           string
	UserSubscribeId int64
}

// SubscribeFilter filters user subscriptions.
type SubscribeFilter struct {
	Subscribers []int64
	IsActive    *bool
	StartTime   int64
	EndTime     int64
}
