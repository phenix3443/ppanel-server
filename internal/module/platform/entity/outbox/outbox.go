// Package outbox holds the generic domain-event outbox row (ADR-001 step-6
// preparation): appended in the owning domain's transaction, published onto
// the asynq queue by the dispatch pump, consumed idempotently via the inbox.
package outbox

import "time"

type Event struct {
	ID       int64  `gorm:"primaryKey;autoIncrement"`
	Topic    string `gorm:"type:varchar(64);not null"`
	EventKey string `gorm:"type:varchar(191);not null"`
	Payload  string `gorm:"type:text;not null"`
	// TraceCarrier is the producing request's W3C trace context (a JSON
	// map), captured at append time so the event's delivery joins the
	// originating trace.
	TraceCarrier string `gorm:"type:varchar(1024);not null;default:''"`
	CreatedAt    time.Time
	PublishedAt  *time.Time
}

func (Event) TableName() string {
	return "domain_event_outbox"
}
