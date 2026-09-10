package telegram

import (
	"context"
	"time"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// TelegramMessenger sends a response to a Telegram chat. A non-zero
// threadID addresses one forum topic inside a group; zero addresses the
// chat itself (private chats, or a group's General topic). Send delivers
// plain text; SendMarkdown delivers MarkdownV2 produced by RenderMarkdownV2,
// and must never receive unescaped dynamic data — Telegram rejects the
// whole message over one stray reserved character.
type TelegramMessenger interface {
	Send(chatID, threadID int64, message string) error
	SendMarkdown(chatID, threadID int64, message string) error
}

// TelegramAdminActionStore persists short-lived confirmations for destructive
// administrator commands.
type TelegramAdminActionStore interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
}

// TelegramAdminDependencies contains only the collaborators used by Telegram
// administrator commands. It intentionally does not accept Application.
type TelegramAdminDependencies struct {
	Messenger TelegramMessenger
	Actions   TelegramAdminActionStore
	// MirrorTicketStatus and MirrorTicketReply keep the ticket's forum
	// topic in step with ticket mutations done through bot commands
	// (/close, /reopen, /rp), which bypass the support module's notifier.
	// Optional and best-effort.
	MirrorTicketStatus func(ticketID int64, status uint8)
	MirrorTicketReply  func(ticketID int64, content string)
	Tickets            repository.TicketRepo
	Orders             repository.OrderRepo
	Users              repository.UserRepo
	UserAuth           repository.UserAuthRepo
	Subscriptions      repository.UserSubscriptionRepo
	UserCache          repository.UserCacheRepo
	Plans              repository.SubscribeRepo
	Logs               repository.LogRepo
	// Wallet is the billing-domain read port for balance display.
	Wallet repository.WalletRepo
}

// TelegramAdmin handles administrative Telegram commands independently from
// the general Telegram bot flow.
type TelegramAdmin struct {
	logger.Logger
	ctx  context.Context
	deps TelegramAdminDependencies
}

func NewTelegramAdmin(ctx context.Context, deps TelegramAdminDependencies) *TelegramAdmin {
	return &TelegramAdmin{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// reply answers in the chat — and, inside the admin group, the same forum
// topic — the command came from.
func (a *TelegramAdmin) reply(msg *models.Message, message string) error {
	return a.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), message)
}
