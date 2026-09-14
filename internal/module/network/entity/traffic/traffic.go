package traffic

import "time"

//goland:noinspection GoNameStartsWithPackageName
type TrafficLog struct {
	Id          int64     `gorm:"primaryKey"`
	ServerId    int64     `gorm:"index:idx_server_id;not null;comment:Server ID"`
	UserId      int64     `gorm:"index:idx_user_id;not null;comment:User ID"`
	SubscribeId int64     `gorm:"index:idx_subscribe_id;not null;comment:Subscription ID"`
	Download    int64     `gorm:"default:0;comment:Download Traffic"`
	Upload      int64     `gorm:"default:0;comment:Upload Traffic"`
	Timestamp   time.Time `gorm:"default:CURRENT_TIMESTAMP(3);not null;comment:Traffic Log Time"`
}

type TotalTraffic struct {
	Download int64
	Upload   int64
}

type SubscribeTrafficDelta struct {
	SubscribeId int64
	Download    int64
	Upload      int64
}

type ServerTrafficRanking struct {
	ServerId int64
	Download int64
	Upload   int64
	Total    int64
}

type UserTrafficRanking struct {
	UserId      int64
	SubscribeId int64
	Download    int64
	Upload      int64
	Total       int64
}

func (TrafficLog) TableName() string {
	return "traffic_log"
}

// TrafficLogDetailsFilter traffic 明细查询过滤条件
type TrafficLogDetailsFilter struct {
	ServerId    int64
	UserId      int64
	SubscribeId int64
	Start       time.Time
	End         time.Time
	Page        int
	Size        int
}

// SubscribeTrafficScope narrows a traffic query to one user's own subscription.
// UserId is always applied alongside SubscribeId: a caller-supplied id that
// belongs to somebody else then matches nothing instead of leaking rows.
type SubscribeTrafficScope struct {
	UserId      int64
	SubscribeId int64
	Start       time.Time
	End         time.Time
}

// HourlyTraffic is one hour of a subscription's usage. Hour is the wall-clock
// key "2006-01-02 15" produced by the database, not an instant: the grouping is
// done on the stored local wall-clock so MySQL and Postgres agree. Callers turn
// it into a time.Time with timeutil.Location().
type HourlyTraffic struct {
	Hour     string
	Download int64
	Upload   int64
}
