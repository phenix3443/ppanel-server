package telegram

import (
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// handleGroup processes one message inside the admin group: topic lifecycle
// service messages, administrator commands, and replies inside support or
// ticket topics.
func (l *TelegramLogic) handleGroup(msg *models.Message) {
	if msg.ForumTopicClosed != nil || msg.ForumTopicReopened != nil {
		l.syncTopicLifecycle(msg)
		return
	}
	if msg.From == nil || msg.From.IsBot {
		return
	}
	if cmd := messageCommand(msg); cmd != "" {
		if isAdminCommand(cmd) {
			l.deps.Admin.Handle(msg)
		}
		return
	}
	if msg.MessageThreadID == 0 || l.deps.Topics == nil {
		return
	}
	topic, err := l.deps.Topics.FindByThread(l.ctx, msg.Chat.ID, int64(msg.MessageThreadID))
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			l.Errorw("group relay: topic lookup failed", logger.Field("error", err.Error()))
		}
		return
	}
	// Only support and ticket topics carry a user conversation; chatter in
	// the notification feed (or any future kind) is none of the bot's
	// business.
	if topic.Kind != telegramtopic.KindSupport && topic.Kind != telegramtopic.KindTicket {
		return
	}
	// Whatever staff write in a mapped topic reaches a customer, so it
	// carries the same authority as an administrator command. The rejection
	// is spoken but rate-limited: an unbound member flooding a topic must
	// not make the bot flood it too.
	if reject := l.rejectNonAdminSender(msg); reject != "" {
		if l.deps.Limiter != nil {
			if allowed, _ := l.deps.Limiter.Allow(l.ctx, msg.From.ID); !allowed {
				return
			}
		}
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), reject)
		return
	}
	switch topic.Kind {
	case telegramtopic.KindSupport:
		l.relayAdminReply(msg, topic)
	case telegramtopic.KindTicket:
		l.appendTicketFollow(msg, topic)
	}
}

// rejectNonAdminSender enforces panel-administrator identity for topic
// replies. The sender would otherwise reasonably believe their message
// reached the customer, so a rejection is always spoken, never silent.
func (l *TelegramLogic) rejectNonAdminSender(msg *models.Message) string {
	if l.deps.Users == nil || l.deps.UserAuth == nil {
		return "系统未配置，回复未送达用户。"
	}
	auth, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "telegram", strconv.FormatInt(msg.From.ID, 10))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "⚠️ 您的 Telegram 未绑定管理员账号，回复未送达用户。"
		}
		l.Errorw("group relay: sender lookup failed", logger.Field("error", err.Error()))
		return "系统错误，回复未送达用户。"
	}
	u, err := l.deps.Users.FindOne(l.ctx, auth.UserId)
	if err != nil {
		l.Errorw("group relay: sender user lookup failed", logger.Field("error", err.Error()))
		return "系统错误，回复未送达用户。"
	}
	if u.IsAdmin == nil || !*u.IsAdmin {
		return "⚠️ 您不是管理员，回复未送达用户。"
	}
	return ""
}

// relayAdminReply copies a staff message from a support topic to the bound
// user's private chat, hiding which administrator wrote it.
func (l *TelegramLogic) relayAdminReply(msg *models.Message, topic *telegramtopic.Topic) {
	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "telegram", topic.RefId)
	if err != nil {
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 该用户已解绑 Telegram，消息无法送达。")
		return
	}
	customerChat, err := strconv.ParseInt(method.AuthIdentifier, 10, 64)
	if err != nil {
		l.Errorw("support relay: malformed chat id", logger.Field("value", method.AuthIdentifier))
		return
	}
	if err := l.deps.TopicClient.CopyTo(l.ctx, customerChat, msg.Chat.ID, msg.ID); err != nil {
		l.Errorw("support relay: copy to user failed", logger.Field("error", err.Error()), logger.Field("user_id", topic.RefId))
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 回复送达失败，用户可能已停用 Bot。")
	}
}

// appendTicketFollow records a staff message in a ticket topic as an
// administrator reply, exactly like the /rp command, so the website thread
// and the topic stay one conversation.
func (l *TelegramLogic) appendTicketFollow(msg *models.Message, topic *telegramtopic.Topic) {
	if l.deps.Tickets == nil {
		return
	}
	if msg.Text == "" {
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "工单回复目前仅支持文本消息。")
		return
	}
	if err := l.deps.Tickets.InsertTicketFollow(l.ctx, &ticket.Follow{
		TicketId: topic.RefId,
		From:     "admin",
		Type:     1,
		Content:  msg.Text,
	}); err != nil {
		l.Errorw("ticket relay: insert follow failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 回复保存失败，请稍后再试。")
		return
	}
	if err := l.deps.Tickets.UpdateTicketStatus(l.ctx, topic.RefId, 0, ticket.Waiting); err != nil {
		l.Errorw("ticket relay: status update failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
	}
}

// syncTopicLifecycle mirrors a HUMAN closing or reopening a topic inside
// Telegram back onto the mapping and, for ticket topics, the ticket itself.
// The bot's own Close/Reopen calls emit the same service messages (with the
// bot as From) and must be ignored: syncing them back would overwrite the
// status the website side just wrote — e.g. an automatic reopen while
// posting a website reply would flip the ticket from Waiting to Pending.
func (l *TelegramLogic) syncTopicLifecycle(msg *models.Message) {
	if msg.From != nil && msg.From.IsBot {
		return
	}
	if msg.MessageThreadID == 0 || l.deps.Topics == nil {
		return
	}
	topic, err := l.deps.Topics.FindByThread(l.ctx, msg.Chat.ID, int64(msg.MessageThreadID))
	if err != nil {
		return
	}
	closed := msg.ForumTopicClosed != nil
	status := uint8(telegramtopic.StatusActive)
	if closed {
		status = telegramtopic.StatusClosed
	}
	if err := l.deps.Topics.UpdateStatus(l.ctx, topic.Id, status); err != nil {
		l.Errorw("topic lifecycle: mapping update failed", logger.Field("error", err.Error()))
	}
	if topic.Kind == telegramtopic.KindTicket && l.deps.Tickets != nil {
		ticketStatus := uint8(ticket.Pending)
		if closed {
			ticketStatus = ticket.Closed
		}
		if err := l.deps.Tickets.UpdateTicketStatus(l.ctx, topic.RefId, 0, ticketStatus); err != nil {
			l.Errorw("topic lifecycle: ticket status sync failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
		}
	}
	if topic.Kind == telegramtopic.KindSupport && closed {
		// Best effort: tell the customer the conversation ended.
		if method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "telegram", topic.RefId); err == nil {
			if chatID, perr := strconv.ParseInt(method.AuthIdentifier, 10, 64); perr == nil {
				_ = l.sendMessage("本次客服会话已结束。如需帮助，直接发送消息即可重新开启。", chatID)
			}
		}
	}
}
