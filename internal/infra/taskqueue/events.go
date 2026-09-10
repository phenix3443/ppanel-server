package taskqueue

import "fmt"

const (
	// EventDeliver carries one domain event from the outbox to its topic
	// subscribers; asynq is the broker (retry, backoff, dead-letter archive).
	EventDeliver = "events:deliver"
)

// EventDeliverPayload is the broker representation of one outbox event.
type EventDeliverPayload struct {
	ID      int64  `json:"id"`
	Topic   string `json:"topic"`
	Key     string `json:"key"`
	Payload string `json:"payload"`
}

// EventTaskID deduplicates enqueues of the same outbox event: the publish
// pump may replay an enqueue when marking the event published fails, and the
// task-id conflict turns that replay into a no-op.
func EventTaskID(id int64) string {
	return fmt.Sprintf("events:deliver:%d", id)
}
