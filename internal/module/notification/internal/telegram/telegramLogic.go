package telegram

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type TelegramLogic struct {
	logger.Logger
	ctx  context.Context
	deps TelegramLogicDependencies
}

func NewTelegramLogic(ctx context.Context, deps TelegramLogicDependencies) *TelegramLogic {
	return &TelegramLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

// TelegramLogic routes one update. User commands live in the private chat;
// everything administrative — commands, support topics, ticket topics —
// lives in the configured admin group. Messages from other groups are
// ignored entirely.
func (l *TelegramLogic) TelegramLogic(req *models.Update) {
	msg := req.Message
	if msg == nil {
		return
	}
	group := l.groupChatID()
	switch {
	case msg.Chat.Type == models.ChatTypePrivate:
		l.handlePrivate(msg)
	case group != 0 && msg.Chat.ID == group:
		l.handleGroup(msg)
	}
}

func (l *TelegramLogic) groupChatID() int64 {
	if l.deps.GroupChatID == nil {
		return 0
	}
	return l.deps.GroupChatID()
}

func (l *TelegramLogic) handlePrivate(msg *models.Message) {
	if msg.From != nil && msg.From.IsBot {
		return
	}
	cmd := messageCommand(msg)
	// /help is in the public menu, so in the private chat it must answer
	// with the user-facing help — the admin help lives in the group.
	if cmd == "help" || cmd == "h" {
		_ = l.sendMessage("🤖 可用命令：\n/start <令牌> 或 /bind <令牌> —— 绑定面板账号\n\n绑定后直接发送消息即可联系人工客服。", msg.Chat.ID)
		return
	}
	if isAdminCommand(cmd) {
		_ = l.sendMessage("管理员命令只能在管理群中使用。", msg.Chat.ID)
		return
	}
	switch cmd {
	case "traffic":
		if err := l.traffic(msg.Chat.ID); err != nil {
			l.Logger.Error("[TelegramLogic] Traffic Error: ", logger.Field("error", err.Error()), logger.Field("command", cmd), logger.Field("chat_id", msg.Chat.ID))
		}
	case "bind":
		if err := l.bind(msg.Chat.ID, commandArguments(msg)); err != nil {
			l.Logger.Error("[TelegramLogic] Bind Error: ", logger.Field("error", err.Error()), logger.Field("command", cmd), logger.Field("chat_id", msg.Chat.ID))
		}
	case "start":
		if err := l.start(msg); err != nil {
			l.Logger.Error("[TelegramLogic] Start Error: ", logger.Field("error", err.Error()), logger.Field("command", cmd), logger.Field("chat_id", msg.Chat.ID), logger.Field("text", msg.Text))
		}
	case "":
		// A plain message is a support request: relay it into the user's
		// live-chat topic in the admin group.
		l.relaySupport(msg)
	}
}

func isAdminCommand(cmd string) bool {
	switch cmd {
	case "dash", "tickets", "tickets_waiting", "tk", "rp", "close", "reopen",
		"user", "user_sub", "user_log", "reset", "toggle", "ban", "help", "h":
		return true
	}
	if strings.HasPrefix(cmd, "confirm_") || strings.HasPrefix(cmd, "cancel_") {
		return true
	}
	return false
}

func (l *TelegramLogic) sendMessage(message string, userID int64) error {
	return l.deps.Messenger.Send(userID, 0, message)
}

func (l *TelegramLogic) sendMarkdown(message string, userID int64) error {
	return l.deps.Messenger.SendMarkdown(userID, 0, message)
}

type telegramBotMessenger struct {
	bot *tgbot.Bot
}

// Send delivers plain text: command replies and administrator output carry
// no formatting, and plain text cannot be broken by the data inside it.
func (m telegramBotMessenger) Send(chatID, threadID int64, message string) error {
	_, err := m.bot.SendMessage(context.Background(), &tgbot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: int(threadID),
		Text:            message,
	})
	return err
}

// SendMarkdown delivers MarkdownV2 built by RenderMarkdownV2.
func (m telegramBotMessenger) SendMarkdown(chatID, threadID int64, message string) error {
	_, err := m.bot.SendMessage(context.Background(), &tgbot.SendMessageParams{
		ChatID:          chatID,
		MessageThreadID: int(threadID),
		Text:            message,
		ParseMode:       models.ParseModeMarkdown,
	})
	return err
}

// SetCommands publishes a command menu. A zero chatID targets the default
// scope every user sees; otherwise the menu applies to that chat alone, which
// is how administrator commands stay hidden from ordinary users.
func (m telegramBotMessenger) SetCommands(chatID int64, commands []Command) error {
	botCommands := make([]models.BotCommand, 0, len(commands))
	for _, command := range commands {
		botCommands = append(botCommands, models.BotCommand{
			Command:     command.Command,
			Description: command.Description,
		})
	}

	params := &tgbot.SetMyCommandsParams{Commands: botCommands}
	if chatID != 0 {
		params.Scope = &models.BotCommandScopeChat{ChatID: chatID}
	}
	_, err := m.bot.SetMyCommands(context.Background(), params)
	return err
}

// SetGroupAdminCommands publishes a menu that only the group's
// administrators see in their composer.
func (m telegramBotMessenger) SetGroupAdminCommands(chatID int64, commands []Command) error {
	botCommands := make([]models.BotCommand, 0, len(commands))
	for _, command := range commands {
		botCommands = append(botCommands, models.BotCommand{
			Command:     command.Command,
			Description: command.Description,
		})
	}
	_, err := m.bot.SetMyCommands(context.Background(), &tgbot.SetMyCommandsParams{
		Commands: botCommands,
		Scope:    &models.BotCommandScopeChatAdministrators{ChatID: chatID},
	})
	return err
}

func (l *TelegramLogic) traffic(userId int64) error {
	return nil
}

// bindTokenKey addresses a single-use account-binding token. Binding tokens
// live under their own prefix: the account's session id must never double as
// a binding capability, because the deep link carrying it is shared through
// Telegram chats.
func bindTokenKey(token string) string {
	return fmt.Sprintf("%v:%v", config.TelegramBindKey, token)
}

// consumeBindToken invalidates a binding token once it has been redeemed, so
// a link that leaks afterwards cannot rebind the account.
func (l *TelegramLogic) consumeBindToken(token string) {
	if err := l.deps.Sessions.Delete(context.Background(), bindTokenKey(token)); err != nil {
		l.Errorw("TelegramLogic failed to invalidate bind token", logger.Field("error", err.Error()))
	}
}

func (l *TelegramLogic) bind(userId int64, token string) error {
	if token == "" {
		return l.sendMessage("Please provide a bind token. Usage: /bind <token>", userId)
	}

	// Resolve the single-use binding token issued by the panel
	value, err := l.deps.Sessions.Get(context.Background(), bindTokenKey(token))
	if err != nil {
		if errors.Is(err, redis.Nil) {
			l.Errorw("TelegramLogic bind token not found or expired")
			return l.sendMessage("Bind token is invalid or expired. Please request a new one.", userId)
		}
		l.Errorw("TelegramLogic bind Redis Get Error", logger.Field("error", err.Error()))
		return l.sendMessage("Bind failed. Please try again later.", userId)
	}

	bindUserId, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		l.Errorw("TelegramLogic bind ParseInt Error", logger.Field("error", err.Error()), logger.Field("value", value))
		return l.sendMessage("Bind failed. Invalid session data.", userId)
	}

	chatIdStr := strconv.FormatInt(userId, 10)

	// Check if this Chat ID is already bound to another user
	existingByChatId, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "telegram", chatIdStr)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("TelegramLogic bind FindUserAuthMethodByOpenID Error", logger.Field("error", err.Error()), logger.Field("chatId", userId))
		return l.sendMessage("Bind failed. Please try again later.", userId)
	}
	if existingByChatId.Id > 0 && existingByChatId.UserId != bindUserId {
		l.Infow("Telegram account already bound to another user",
			logger.Field("chatId", userId),
			logger.Field("existingUserId", existingByChatId.UserId),
		)
		return l.sendMessage("This Telegram account is already bound to another user.", userId)
	}

	// Check if the target user already has Telegram bound
	existingByUser, err := l.deps.UserAuth.FindUserAuthMethodByPlatform(l.ctx, bindUserId, "telegram")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("TelegramLogic bind FindUserAuthMethodByPlatform Error", logger.Field("error", err.Error()), logger.Field("bindUserId", bindUserId))
		return l.sendMessage("Bind failed. Please try again later.", userId)
	}
	if err == nil && existingByUser.Id > 0 {
		// Same chat ID, already bound — nothing to do
		if existingByUser.AuthIdentifier == chatIdStr {
			return l.sendMessage("This account is already bound to your Telegram.", userId)
		}
		l.Infow("User already bound to a different Telegram account",
			logger.Field("bindUserId", bindUserId),
			logger.Field("existingChatId", existingByUser.AuthIdentifier),
			logger.Field("newChatId", userId),
		)
		return l.sendMessage("Your account is already bound to a different Telegram account. Please unbind it first.", userId)
	}

	// Create the binding
	if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, &user.AuthMethods{
		UserId:         bindUserId,
		AuthType:       "telegram",
		AuthIdentifier: chatIdStr,
		Verified:       true,
		CreatedAt:      timeutil.Now(),
		UpdatedAt:      timeutil.Now(),
	}); err != nil {
		l.Errorw("TelegramLogic bind InsertUserAuthMethod Error", logger.Field("error", err.Error()), logger.Field("bindUserId", bindUserId))
		return l.sendMessage("Bind failed. Please try again later.", userId)
	}
	l.consumeBindToken(token)

	// Update user cache
	err = l.deps.UserCache.UpdateUserCache(l.ctx, &user.User{
		Id: bindUserId,
	})
	if err != nil {
		l.Errorw("TelegramLogic bind UpdateUserCache Error", logger.Field("error", err.Error()), logger.Field("bindUserId", bindUserId))
	}

	text, err := RenderMarkdownV2(BindNotify, map[string]string{
		"Id":   strconv.FormatInt(bindUserId, 10),
		"Time": timeutil.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		l.Errorw("TelegramLogic bind RenderTemplate Error", logger.Field("error", err.Error()))
		return l.sendMessage("Bound successfully!", userId)
	}
	return l.sendMarkdown(text, userId)
}

func (l *TelegramLogic) start(msg *models.Message) error {
	bindToken := commandArguments(msg)
	if bindToken == "" {
		return l.sendMessage("Please bind account!", msg.Chat.ID)
	}

	chatIdStr := strconv.FormatInt(msg.Chat.ID, 10)

	// Resolve the single-use binding token issued by the panel
	value, err := l.deps.Sessions.Get(context.Background(), bindTokenKey(bindToken))
	if err != nil && !errors.Is(err, redis.Nil) {
		l.Errorw("TelegramLogic start Redis Get Error", logger.Field("error", err.Error()))
		return l.sendMessage("Bind failed!", msg.Chat.ID)
	}
	if value == "" {
		l.Errorw("TelegramLogic start bind token not found or expired")
		return l.sendMessage("Session expired. Please request a new bind link.", msg.Chat.ID)
	}

	userId, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		l.Errorw("TelegramLogic start ParseInt Error", logger.Field("error", err.Error()))
		return l.sendMessage("Bind failed!", msg.Chat.ID)
	}

	// Check if this Chat ID is already bound to another user
	existingByChatId, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "telegram", chatIdStr)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			l.Errorw("TelegramLogic start FindUserAuthMethodByOpenID Error", logger.Field("error", err.Error()), logger.Field("chatId", msg.Chat.ID))
			return l.sendMessage("Bind failed!", msg.Chat.ID)
		}
	}
	if existingByChatId.Id > 0 && existingByChatId.UserId != userId {
		l.Infow("Telegram account already bound to another user, cannot rebind",
			logger.Field("chatId", msg.Chat.ID),
			logger.Field("existingUserId", existingByChatId.UserId),
			logger.Field("newUserId", userId),
		)
		return l.sendMessage("This Telegram account is already bound to another user.", msg.Chat.ID)
	}

	// Check if the target user already has a Telegram binding
	method, err := l.deps.UserAuth.FindUserAuthMethodByPlatform(l.ctx, userId, "telegram")
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		l.Errorw("TelegramLogic start FindUserAuthMethodByPlatform Error", logger.Field("error", err.Error()), logger.Field("userId", userId))
		return l.sendMessage("Bind failed!", msg.Chat.ID)
	}

	if err == nil && method.Id > 0 {
		// Already bound to the same chat ID — nothing to do
		if method.AuthIdentifier == chatIdStr {
			return l.sendMessage("Your account is already bound to this Telegram account.", msg.Chat.ID)
		}
		// Already bound to a different chat ID — DON'T overwrite silently
		l.Infow("User already bound to a different Telegram account, cannot rebind via start",
			logger.Field("userId", userId),
			logger.Field("existingChatId", method.AuthIdentifier),
			logger.Field("newChatId", msg.Chat.ID),
		)
		return l.sendMessage("Your account is already bound to a different Telegram account. Please unbind it first.", msg.Chat.ID)
	}

	// No existing binding — create a new one
	if err := l.deps.UserAuth.InsertUserAuthMethods(l.ctx, &user.AuthMethods{
		UserId:         userId,
		AuthType:       "telegram",
		AuthIdentifier: chatIdStr,
		Verified:       true,
		CreatedAt:      timeutil.Now(),
		UpdatedAt:      timeutil.Now(),
	}); err != nil {
		l.Errorw("TelegramLogic start InsertUserAuthMethod Error", logger.Field("error", err.Error()), logger.Field("userId", userId))
		return l.sendMessage("Bind failed!", msg.Chat.ID)
	}
	l.consumeBindToken(bindToken)

	// Update user cache
	err = l.deps.UserCache.UpdateUserCache(l.ctx, &user.User{
		Id: userId,
	})
	if err != nil {
		l.Errorw("TelegramLogic start UpdateUserCache Error", logger.Field("error", err.Error()), logger.Field("userId", userId))
	}

	text, err := RenderMarkdownV2(BindNotify, map[string]string{
		"Id":   strconv.FormatInt(userId, 10),
		"Time": timeutil.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		l.Errorw("TelegramLogic start RenderTemplate Error", logger.Field("error", err.Error()))
		return l.sendMessage("Bound successfully!", msg.Chat.ID)
	}
	return l.sendMarkdown(text, msg.Chat.ID)
}
