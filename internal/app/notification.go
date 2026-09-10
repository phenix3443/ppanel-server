package app

import (
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
)

// newNotificationModule wires the notification module against the legacy
// store; the bot client is runtime-recreated, so the module reads it per
// call.
func newNotificationModule(store repository.Store, srv *Application) notification.Service {
	return notification.New(notification.Deps{
		Bot:           srv.Runtime.TelegramBot,
		GroupChatID:   func() int64 { return srv.Runtime.Config().Telegram.GroupChatID },
		Topics:        store.TelegramTopic(),
		Redis:         srv.Redis,
		Users:         store.User(),
		UserAuth:      store.UserAuth(),
		UserCache:     store.UserCache(),
		Tickets:       store.Ticket(),
		Orders:        store.Order(),
		Subscriptions: store.UserSubscription(),
		Plans:         store.Subscribe(),
		Logs:          store.Log(),
		Wallet:        store.Wallet(),
	})
}
