package telegram

import (
	"context"
	"fmt"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// TelegramSessionStore reads and consumes short-lived account-binding tokens.
// Deleting the token on a successful bind is what makes the deep link
// single-use.
type TelegramSessionStore interface {
	Get(ctx context.Context, key string) (string, error)
	Delete(ctx context.Context, key string) error
}

// TelegramRedisStore supports account-binding sessions, administrator
// command confirmations, and the support relay rate limit.
type TelegramRedisStore interface {
	TelegramSessionStore
	TelegramAdminActionStore
	TelegramRelayLimiter
}

// TelegramAdminHandler handles administrator Telegram commands.
type TelegramAdminHandler interface {
	Handle(msg *models.Message)
}

// TelegramRelayLimiter caps how fast one chat may relay support messages
// into the admin group.
type TelegramRelayLimiter interface {
	// Allow reports whether the chat may relay another message right now.
	// shouldNotify is true only for the first rejection of a window, so a
	// flood is answered with one notice instead of one per message.
	Allow(ctx context.Context, chatID int64) (allowed, shouldNotify bool)
}

// TelegramLogicDependencies explicitly declares the collaborators used by
// update routing: user command dispatch and account binding in the private
// chat, and the group-side relays. The group fields may be left zero, which
// turns every group feature off.
type TelegramLogicDependencies struct {
	Messenger TelegramMessenger
	Sessions  TelegramSessionStore
	UserAuth  repository.UserAuthRepo
	UserCache repository.UserCacheRepo
	Admin     TelegramAdminHandler

	// GroupChatID returns the validated admin group; zero disables group
	// routing. Read per call because re-initialisation may change it.
	GroupChatID func() int64
	Topics      repository.TelegramTopicRepo
	TopicClient TelegramTopicClient
	Tickets     repository.TicketRepo
	Users       repository.UserRepo
	Limiter     TelegramRelayLimiter
}

// NewTelegramBotMessenger adapts a Telegram Bot API client to the command
// messenger port.
func NewTelegramBotMessenger(bot *tgbot.Bot) TelegramMessenger {
	return telegramBotMessenger{bot: bot}
}

// NewTelegramBotCommandRegistrar adapts a Telegram Bot API client to the
// command-menu port.
func NewTelegramBotCommandRegistrar(bot *tgbot.Bot) TelegramCommandRegistrar {
	return telegramBotMessenger{bot: bot}
}

// NewTelegramRedisStore adapts Redis to the binding-session and administrator
// confirmation ports.
func NewTelegramRedisStore(client *redis.Client) TelegramRedisStore {
	return redisTelegramStore{client: client}
}

type redisTelegramStore struct {
	client *redis.Client
}

func (s redisTelegramStore) Get(ctx context.Context, key string) (string, error) {
	return s.client.Get(ctx, key).Result()
}

func (s redisTelegramStore) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s redisTelegramStore) Delete(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

// supportRelayPerMinute caps one chat's relayed messages. Telegram itself
// throttles bots at roughly twenty messages per minute per group, so one
// flooding user must not exhaust the whole group's budget.
const supportRelayPerMinute = 15

// Allow implements a fixed one-minute window. The TTL is created (SETNX)
// before the first increment so a crash between the two commands can never
// leave an expiry-less counter that locks the chat out forever. Errors fail
// open: the limit protects against floods, not against Redis being down.
func (s redisTelegramStore) Allow(ctx context.Context, chatID int64) (allowed, shouldNotify bool) {
	key := fmt.Sprintf("tg:support:rl:%d", chatID)
	if err := s.client.SetNX(ctx, key, 0, time.Minute).Err(); err != nil {
		return true, false
	}
	count, err := s.client.Incr(ctx, key).Result()
	if err != nil {
		return true, false
	}
	return count <= supportRelayPerMinute, count == supportRelayPerMinute+1
}
