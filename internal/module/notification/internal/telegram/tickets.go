package telegram

import (
	"fmt"

	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// TicketTopicTitle names a ticket topic; the leading number keeps the topic
// list scannable next to /tk.
func TicketTopicTitle(t *ticket.Ticket) string {
	return fmt.Sprintf("🎫 #%d %s", t.Id, t.Title)
}

// TicketCreated opens the ticket's topic and posts its opening message.
// Only tickets created after the group went live get a topic; older tickets
// have no mapping and their events are skipped upstream.
func (s *TopicService) TicketCreated(m TelegramMessenger, t *ticket.Ticket, userLabel string) error {
	topic, _, err := s.Ensure(telegramtopic.KindTicket, t.Id, TicketTopicTitle(t))
	if err != nil {
		return err
	}
	body := fmt.Sprintf("🎫 新工单 #%d\n用户：%s\n标题：%s", t.Id, userLabel, t.Title)
	if t.Description != "" {
		body += "\n\n" + t.Description
	}
	body += "\n\n直接在本话题回复即可答复用户；关闭话题即关闭工单。"
	_, err = s.PostText(m, topic, body)
	return err
}

// TicketReplied posts a website-side reply into the ticket's topic. A
// ticket without a mapping predates the group and is silently skipped.
func (s *TopicService) TicketReplied(m TelegramMessenger, ticketID int64, from, content string) error {
	topic, err := s.topics.FindByKindRef(s.ctx, s.group, telegramtopic.KindTicket, ticketID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Either the ticket predates the group, or its topic creation
			// failed and this ticket is invisible to the group — worth a
			// log line, not an error.
			s.Errorw("ticket has no forum topic, reply not mirrored", logger.Field("ticket_id", ticketID))
			return nil
		}
		return err
	}
	label := "💻 网站回复（管理员）"
	if from == "user" || from == "" {
		label = "👤 用户回复"
	}
	_, err = s.PostText(m, topic, label+"：\n"+content)
	return err
}

// TicketStatusChanged mirrors a website-side status change onto the topic:
// closing the ticket closes the topic, any other status reopens it.
func (s *TopicService) TicketStatusChanged(ticketID int64, status uint8) error {
	topic, err := s.topics.FindByKindRef(s.ctx, s.group, telegramtopic.KindTicket, ticketID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if status == ticket.Closed {
		return s.Close(topic)
	}
	if topic.Status != telegramtopic.StatusActive {
		_, err = s.Reopen(topic)
		return err
	}
	return nil
}
