package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// NotifyTopicTitle names the operations feed topic the bot creates in the
// administrators' group.
const NotifyTopicTitle = "📣 运营通知"

// TelegramTopicClient is the group-side Bot API surface the topic layer
// drives. telegramBotMessenger implements it; tests fake it.
type TelegramTopicClient interface {
	// ValidateAdminGroup confirms the chat is a forum-enabled supergroup
	// where the bot may manage topics.
	ValidateAdminGroup(ctx context.Context, chatID int64) error
	CreateTopic(ctx context.Context, chatID int64, name string) (int64, error)
	DeleteTopic(ctx context.Context, chatID, threadID int64) error
	CloseTopic(ctx context.Context, chatID, threadID int64) error
	ReopenTopic(ctx context.Context, chatID, threadID int64) error
	// ForwardToThread relays a user's message into a group topic keeping
	// the sender identity visible to the support staff.
	ForwardToThread(ctx context.Context, chatID, threadID, fromChatID int64, messageID int) error
	// CopyTo relays a group message to a private chat without exposing
	// which administrator wrote it.
	CopyTo(ctx context.Context, toChatID, fromChatID int64, messageID int) error
}

// NewTelegramTopicClient adapts the bot client to the topic-management port.
func NewTelegramTopicClient(bot *tgbot.Bot) TelegramTopicClient {
	return telegramBotMessenger{bot: bot}
}

func (m telegramBotMessenger) ValidateAdminGroup(ctx context.Context, chatID int64) error {
	chat, err := m.bot.GetChat(ctx, &tgbot.GetChatParams{ChatID: chatID})
	if err != nil {
		return fmt.Errorf("get chat: %w", err)
	}
	if chat.Type != models.ChatTypeSupergroup || !chat.IsForum {
		return errors.New("the chat is not a supergroup with topics enabled")
	}
	member, err := m.bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: chatID, UserID: m.bot.ID()})
	if err != nil {
		return fmt.Errorf("get bot membership: %w", err)
	}
	switch {
	case member.Owner != nil:
	case member.Administrator != nil && member.Administrator.CanManageTopics:
	default:
		return errors.New("the bot must be a group administrator with the manage-topics right")
	}
	return nil
}

func (m telegramBotMessenger) CreateTopic(ctx context.Context, chatID int64, name string) (int64, error) {
	topic, err := m.bot.CreateForumTopic(ctx, &tgbot.CreateForumTopicParams{ChatID: chatID, Name: name})
	if err != nil {
		return 0, err
	}
	return int64(topic.MessageThreadID), nil
}

func (m telegramBotMessenger) DeleteTopic(ctx context.Context, chatID, threadID int64) error {
	_, err := m.bot.DeleteForumTopic(ctx, &tgbot.DeleteForumTopicParams{ChatID: chatID, MessageThreadID: int(threadID)})
	return err
}

func (m telegramBotMessenger) CloseTopic(ctx context.Context, chatID, threadID int64) error {
	_, err := m.bot.CloseForumTopic(ctx, &tgbot.CloseForumTopicParams{ChatID: chatID, MessageThreadID: int(threadID)})
	return err
}

func (m telegramBotMessenger) ReopenTopic(ctx context.Context, chatID, threadID int64) error {
	_, err := m.bot.ReopenForumTopic(ctx, &tgbot.ReopenForumTopicParams{ChatID: chatID, MessageThreadID: int(threadID)})
	return err
}

func (m telegramBotMessenger) ForwardToThread(ctx context.Context, chatID, threadID, fromChatID int64, messageID int) error {
	_, err := m.bot.ForwardMessage(ctx, &tgbot.ForwardMessageParams{
		ChatID:          chatID,
		MessageThreadID: int(threadID),
		FromChatID:      fromChatID,
		MessageID:       messageID,
	})
	return err
}

func (m telegramBotMessenger) CopyTo(ctx context.Context, toChatID, fromChatID int64, messageID int) error {
	_, err := m.bot.CopyMessage(ctx, &tgbot.CopyMessageParams{
		ChatID:     toChatID,
		FromChatID: fromChatID,
		MessageID:  messageID,
	})
	return err
}

// isMissingThreadError classifies the Bot API rejection for a forum topic
// that an administrator deleted; the mapping self-heals by recreating it.
func isMissingThreadError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "message thread not found") || strings.Contains(msg, "topic_deleted")
}

// isTopicClosedError classifies the rejection for posting into a closed
// topic; the sender reopens the topic and retries.
func isTopicClosedError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "topic_closed")
}

// TopicService owns the mapping between forum topics and the conversation
// each carries.
type TopicService struct {
	logger.Logger
	ctx    context.Context
	client TelegramTopicClient
	topics repository.TelegramTopicRepo
	group  int64
}

func NewTopicService(ctx context.Context, client TelegramTopicClient, topics repository.TelegramTopicRepo, group int64) *TopicService {
	return &TopicService{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		client: client,
		topics: topics,
		group:  group,
	}
}

// topicTitleLimit is Telegram's cap on forum topic names.
const topicTitleLimit = 128

func clampTopicTitle(title string) string {
	r := []rune(title)
	if len(r) <= topicTitleLimit {
		return title
	}
	return string(r[:topicTitleLimit-1]) + "…"
}

// Ensure returns the mapping for (kind, ref), creating the forum topic on
// first use; created reports whether this call made it. Concurrent creators
// race on the unique key; the loser adopts the winner's row.
func (s *TopicService) Ensure(kind uint8, refID int64, title string) (topic *telegramtopic.Topic, created bool, err error) {
	topic, err = s.topics.FindByKindRef(s.ctx, s.group, kind, refID)
	if err == nil {
		return topic, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	title = clampTopicTitle(title)
	threadID, err := s.client.CreateTopic(s.ctx, s.group, title)
	if err != nil {
		return nil, false, fmt.Errorf("create forum topic: %w", err)
	}
	row := &telegramtopic.Topic{
		ChatId:   s.group,
		Kind:     kind,
		RefId:    refID,
		ThreadId: threadID,
		Title:    title,
		Status:   telegramtopic.StatusActive,
	}
	if err := s.topics.Insert(s.ctx, row); err != nil {
		// The unique key lost a race: adopt the winner's mapping. The topic
		// this call created is deleted (best effort) — an orphaned topic
		// would let staff reply into a thread that reaches nobody.
		if existing, ferr := s.topics.FindByKindRef(s.ctx, s.group, kind, refID); ferr == nil {
			if derr := s.client.DeleteTopic(s.ctx, s.group, threadID); derr != nil {
				s.Errorw("orphaned forum topic could not be deleted",
					logger.Field("error", derr.Error()), logger.Field("thread_id", threadID))
			}
			return existing, false, nil
		}
		return nil, false, err
	}
	return row, true, nil
}

// Recreate repoints a mapping whose topic was deleted inside Telegram.
func (s *TopicService) Recreate(topic *telegramtopic.Topic) (*telegramtopic.Topic, error) {
	threadID, err := s.client.CreateTopic(s.ctx, s.group, topic.Title)
	if err != nil {
		return nil, fmt.Errorf("recreate forum topic: %w", err)
	}
	if err := s.topics.UpdateThread(s.ctx, topic.Id, threadID); err != nil {
		return nil, err
	}
	updated := *topic
	updated.ThreadId = threadID
	updated.Status = telegramtopic.StatusActive
	return &updated, nil
}

// Reopen reopens a closed topic and marks the mapping active. A missing
// topic is recreated instead.
func (s *TopicService) Reopen(topic *telegramtopic.Topic) (*telegramtopic.Topic, error) {
	if err := s.client.ReopenTopic(s.ctx, s.group, topic.ThreadId); err != nil && !isTopicNotModifiedError(err) {
		if isMissingThreadError(err) {
			return s.Recreate(topic)
		}
		return nil, err
	}
	if topic.Status != telegramtopic.StatusActive {
		if err := s.topics.UpdateStatus(s.ctx, topic.Id, telegramtopic.StatusActive); err != nil {
			return nil, err
		}
	}
	updated := *topic
	updated.Status = telegramtopic.StatusActive
	return &updated, nil
}

// Close closes the forum topic and marks the mapping; a topic already gone
// from Telegram still gets its mapping closed.
func (s *TopicService) Close(topic *telegramtopic.Topic) error {
	if err := s.client.CloseTopic(s.ctx, s.group, topic.ThreadId); err != nil &&
		!isMissingThreadError(err) && !isTopicNotModifiedError(err) {
		return err
	}
	return s.topics.UpdateStatus(s.ctx, topic.Id, telegramtopic.StatusClosed)
}

// isTopicNotModifiedError matches the no-op rejection Telegram returns when
// a topic is already in the requested state.
func isTopicNotModifiedError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "topic_not_modified")
}

// PostMarkdown sends MarkdownV2 into a topic through Relay's self-healing.
func (s *TopicService) PostMarkdown(m TelegramMessenger, topic *telegramtopic.Topic, text string) (*telegramtopic.Topic, error) {
	return s.Relay(topic, func(threadID int64) error {
		return m.SendMarkdown(s.group, threadID, text)
	})
}

// PostText is PostMarkdown for plain text.
func (s *TopicService) PostText(m TelegramMessenger, topic *telegramtopic.Topic, text string) (*telegramtopic.Topic, error) {
	return s.Relay(topic, func(threadID int64) error {
		return m.Send(s.group, threadID, text)
	})
}

// Relay runs op against the topic's thread, transparently recreating a
// deleted topic or reopening a closed one, then retrying once. It returns
// the mapping actually used, which may have been repointed.
func (s *TopicService) Relay(topic *telegramtopic.Topic, op func(threadID int64) error) (*telegramtopic.Topic, error) {
	err := op(topic.ThreadId)
	switch {
	case err == nil:
		return topic, nil
	case isMissingThreadError(err):
		repointed, rerr := s.Recreate(topic)
		if rerr != nil {
			return topic, err
		}
		return repointed, op(repointed.ThreadId)
	case isTopicClosedError(err):
		reopened, rerr := s.Reopen(topic)
		if rerr != nil {
			return topic, err
		}
		return reopened, op(reopened.ThreadId)
	}
	return topic, err
}
