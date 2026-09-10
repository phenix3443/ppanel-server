// Package telegramtopic maps forum topics in the administrators' Telegram
// group to what they carry. The bot mirrors its administrator surface into
// that group: one feed topic for operational notifications, one topic per
// website ticket, and one live-chat topic per bound panel user.
package telegramtopic

import "time"

// Topic kinds. The (chat_id, kind, ref_id) tuple is unique, as is the
// (chat_id, thread_id) tuple: one topic never carries two conversations.
const (
	KindNotify  = 1 // operations feed (order notifications, daily reports); ref_id 0
	KindTicket  = 2 // website ticket; ref_id is the ticket id
	KindSupport = 3 // live chat; ref_id is the panel user id
)

const (
	StatusActive = 1
	StatusClosed = 2
)

type Topic struct {
	Id        int64     `gorm:"primaryKey"`
	ChatId    int64     `gorm:"type:bigint;not null;default:0;comment:Admin group chat id"`
	Kind      uint8     `gorm:"type:tinyint(1);not null;default:0;comment:1 notify 2 ticket 3 support"`
	RefId     int64     `gorm:"type:bigint;not null;default:0;comment:Ticket id / user id / 0"`
	ThreadId  int64     `gorm:"type:bigint;not null;default:0;comment:Forum topic message_thread_id"`
	Title     string    `gorm:"type:varchar(128);not null;default:'';comment:Topic title snapshot"`
	Status    uint8     `gorm:"type:tinyint(1);not null;default:1;comment:1 active 2 closed"`
	CreatedAt time.Time `gorm:"<-:create;comment:Create Time"`
	UpdatedAt time.Time `gorm:"comment:Update Time"`
}

func (Topic) TableName() string {
	return "telegram_topic"
}
