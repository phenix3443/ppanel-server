package telegram

import (
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// relaySupport forwards a bound user's private message into their live-chat
// topic in the admin group. Support deliberately requires a bound panel
// account: the topic is keyed by the panel user id, and staff see who they
// are talking to.
func (l *TelegramLogic) relaySupport(msg *models.Message) {
	group := l.groupChatID()
	if group == 0 || l.deps.Topics == nil || l.deps.TopicClient == nil {
		// Group features are off; a plain chat message keeps its historical
		// behaviour of being ignored.
		return
	}
	chatID := msg.Chat.ID

	auth, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "telegram", strconv.FormatInt(chatID, 10))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = l.sendMessage("请先绑定账号后再联系客服：登录面板 → 个人设置 → 绑定 Telegram。", chatID)
			return
		}
		l.Errorw("support relay: query binding failed", logger.Field("error", err.Error()), logger.Field("chat_id", chatID))
		return
	}

	if l.deps.Limiter != nil {
		allowed, shouldNotify := l.deps.Limiter.Allow(l.ctx, chatID)
		if !allowed {
			if shouldNotify {
				_ = l.sendMessage("消息发送过于频繁，请稍后再试。", chatID)
			}
			return
		}
	}

	topics := NewTopicService(l.ctx, l.deps.TopicClient, l.deps.Topics, group)
	topic, created, err := topics.Ensure(telegramtopic.KindSupport, auth.UserId, l.supportTopicTitle(auth.UserId, msg))
	if err != nil {
		l.Errorw("support relay: ensure topic failed", logger.Field("error", err.Error()), logger.Field("user_id", auth.UserId))
		_ = l.sendMessage("客服暂时不可用，请稍后再试。", chatID)
		return
	}
	if created {
		_ = l.sendMessage("已为您接入人工客服，直接发送消息即可，客服会尽快回复。", chatID)
	}

	if _, err := topics.Relay(topic, func(threadID int64) error {
		return l.deps.TopicClient.ForwardToThread(l.ctx, group, threadID, chatID, msg.ID)
	}); err != nil {
		l.Errorw("support relay: forward failed", logger.Field("error", err.Error()), logger.Field("user_id", auth.UserId))
		_ = l.sendMessage("消息转发失败，请稍后再试。", chatID)
	}
}

// supportTopicTitle names a live-chat topic after the panel account, with
// the Telegram username as a human-friendly hint.
func (l *TelegramLogic) supportTopicTitle(userID int64, msg *models.Message) string {
	label := "ID:" + strconv.FormatInt(userID, 10)
	if email, err := l.deps.UserAuth.FindUserAuthMethodByPlatform(l.ctx, userID, "email"); err == nil && email.AuthIdentifier != "" {
		label = email.AuthIdentifier
	}
	title := "💬 " + label
	if msg.From != nil && msg.From.Username != "" {
		title += " · @" + msg.From.Username
	}
	return title
}
