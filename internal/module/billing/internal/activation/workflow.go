// Package activation owns the paid-order business workflow. The task adapter
// only decodes a message and invokes the billing facade.
package activation

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/module/billing/entity/order"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/redis/go-redis/v9"
)

const OrderTypeResetTraffic = 3

type SubscriptionFulfiller interface {
	FulfillPaidOrder(context.Context, string) (*subscription.FulfillmentOutcome, error)
}

type Notifier interface {
	NotifyTelegramUser(context.Context, int64, string) error
	NotifyAdminsTelegram(context.Context, string) error
}

type LegacyGuestCache interface {
	Get(context.Context, string) *redis.StringCmd
}

type WorkflowDeps struct {
	Orders               repository.OrderRepo
	Profiles             ProfileReader
	GuestAccounts        identity.GuestAccounts
	Subscriptions        SubscriptionFulfiller
	LegacyGuestCache     LegacyGuestCache
	Notifications        Notifier
	NotificationsEnabled func() bool
}

type Workflow struct {
	deps   WorkflowDeps
	stages *Service
}

func NewWorkflow(deps WorkflowDeps, stages *Service) *Workflow {
	return &Workflow{deps: deps, stages: stages}
}

func (l *Workflow) ensureGuestAccount(ctx context.Context, orderInfo *order.Order) error {
	if l.deps.GuestAccounts == nil {
		return fmt.Errorf("guest account service is not configured")
	}
	userID, found, err := l.deps.GuestAccounts.FindGuestAccount(ctx, orderInfo.OrderNo)
	if err != nil {
		return err
	}
	if !found {
		guest, err := l.getGuestOrderInfo(ctx, orderInfo)
		if err != nil {
			return err
		}
		userID, err = l.deps.GuestAccounts.EnsureGuestAccount(ctx, identity.GuestAccountCommand{
			OrderNo: orderInfo.OrderNo, AuthType: guest.AuthType, Identifier: guest.Identifier,
			PasswordHash: guest.PasswordHash, LegacyPassword: guest.Password, InviteCode: guest.InviteCode,
		})
		if err != nil {
			return err
		}
	}
	orderInfo.UserId = userID
	return l.deps.Orders.Update(ctx, orderInfo)
}

func (l *Workflow) Activate(ctx context.Context, orderNo string) error {
	orderInfo, err := l.deps.Orders.FindOneByOrderNo(ctx, orderNo)
	if err != nil {
		return err
	}
	if orderInfo.Status == OrderStatusFinished {
		return nil
	}
	if orderInfo.Status != OrderStatusPaid {
		return ErrInvalidOrderStatus
	}

	if orderInfo.Type == OrderTypeSubscribe && orderInfo.UserId == 0 {
		if err := l.ensureGuestAccount(ctx, orderInfo); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Guest account stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
	}

	if orderInfo.Type == OrderTypeRecharge {
		balance, err := l.stages.ActivateRecharge(ctx, orderInfo.OrderNo)
		if err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Recharge stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
		// Load the notification context BEFORE the finalize CAS: once the
		// order is Finished a retry short-circuits, so failing here (all
		// prior stages are idempotent) keeps the notice at-least-once.
		userInfo, err := l.deps.Profiles.FindOne(ctx, orderInfo.UserId)
		if err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Load user for recharge notify failed", logger.Field("error", err.Error()))
			return err
		}
		if err := l.stages.FinalizeOrder(ctx, orderInfo.OrderNo); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Finalize stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
		l.sendRechargeNotifications(ctx, orderInfo, userInfo, balance)
		return nil
	}

	outcome, err := l.deps.Subscriptions.FulfillPaidOrder(ctx, orderInfo.OrderNo)
	if err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Fulfillment stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	if orderInfo.Type == OrderTypeSubscribe || orderInfo.Type == OrderTypeRenewal {
		if err := l.stages.SettleOrderCommission(ctx, orderInfo.OrderNo, outcome.UserID); err != nil {
			logger.WithContext(ctx).Error("[ActivateOrderLogic] Commission stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
			return err
		}
	}

	// Load the notification context BEFORE the finalize CAS (see the
	// recharge branch above for why).
	userInfo, err := l.deps.Profiles.FindOne(ctx, orderInfo.UserId)
	if err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Load user for notify failed", logger.Field("error", err.Error()))
		return err
	}

	if err := l.stages.FinalizeOrder(ctx, orderInfo.OrderNo); err != nil {
		logger.WithContext(ctx).Error("[ActivateOrderLogic] Finalize stage failed", logger.Field("error", err.Error()), logger.Field("order_no", orderInfo.OrderNo))
		return err
	}

	l.notifyFulfillment(ctx, orderInfo, userInfo, outcome)
	return nil
}

// notifyFulfillment dispatches the post-activation notices using the
// fulfillment outcome's notification context.
func (l *Workflow) notifyFulfillment(ctx context.Context, orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome) {
	if outcome == nil {
		return
	}
	notifyType := ""
	switch outcome.NotifyKind {
	case subscription.NotifyKindPurchase:
		notifyType = notification.PurchaseNotify
	case subscription.NotifyKindRenewal:
		notifyType = notification.RenewalNotify
	case subscription.NotifyKindResetTraffic:
		notifyType = notification.ResetTrafficNotify
	default:
		return
	}
	l.sendNotifications(ctx, orderInfo, userInfo, outcome, notifyType)
}

// getTempOrderInfo retrieves temporary order information from Redis cache
func (l *Workflow) getTempOrderInfo(ctx context.Context, orderNo string) (*order.TemporaryOrderInfo, error) {
	cacheKey := fmt.Sprintf(order.TempOrderCacheKey, orderNo)
	data, err := l.deps.LegacyGuestCache.Get(ctx, cacheKey).Result()
	if err != nil {
		logger.WithContext(ctx).Error("Get temp order cache failed",
			logger.Field("error", err.Error()),
			logger.Field("cache_key", cacheKey),
		)
		return nil, err
	}

	var tempOrder order.TemporaryOrderInfo
	if err = tempOrder.Unmarshal([]byte(data)); err != nil {
		logger.WithContext(ctx).Error("Unmarshal temp order cache failed",
			logger.Field("error", err.Error()),
			logger.Field("cache_key", cacheKey),
		)
		return nil, err
	}

	return &tempOrder, nil
}

func (l *Workflow) getGuestOrderInfo(ctx context.Context, orderInfo *order.Order) (*order.TemporaryOrderInfo, error) {
	if orderInfo.GuestAuthType != "" && orderInfo.GuestIdentifier != "" && orderInfo.GuestPasswordHash != "" {
		return &order.TemporaryOrderInfo{
			OrderNo:      orderInfo.OrderNo,
			Identifier:   orderInfo.GuestIdentifier,
			AuthType:     orderInfo.GuestAuthType,
			PasswordHash: orderInfo.GuestPasswordHash,
			InviteCode:   orderInfo.GuestInviteCode,
		}, nil
	}
	return l.getTempOrderInfo(ctx, orderInfo.OrderNo)
}

// sendNotifications sends both user and admin notifications for order completion
func (l *Workflow) sendNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome, notifyType string) {
	// Send user notification
	templateData := l.buildUserNotificationData(orderInfo, outcome)
	if text, err := notification.RenderTelegramMarkdown(notifyType, templateData); err == nil {
		l.sendUserNotifyWithTelegram(ctx, userInfo.Id, text)
	}

	// Send admin notification
	adminData := l.buildAdminNotificationData(orderInfo, userInfo, outcome)
	if text, err := notification.RenderTelegramMarkdown(notification.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// sendRechargeNotifications sends specific notifications for balance recharge orders
func (l *Workflow) sendRechargeNotifications(ctx context.Context, orderInfo *order.Order, userInfo *user.User, balance int64) {
	// Send user notification
	templateData := map[string]string{
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"PaymentMethod": orderInfo.Method,
		"Time":          orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"Balance":       fmt.Sprintf("%.2f", float64(balance)/100),
	}
	if text, err := notification.RenderTelegramMarkdown(notification.RechargeNotify, templateData); err == nil {
		l.sendUserNotifyWithTelegram(ctx, userInfo.Id, text)
	}

	// Send admin notification
	adminData := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"UserEmail":     findEmail(userInfo),
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"SubscribeName": "余额充值",
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
	if text, err := notification.RenderTelegramMarkdown(notification.AdminOrderNotify, adminData); err == nil {
		l.sendAdminNotifyWithTelegram(ctx, text)
	}
}

// buildUserNotificationData creates template data for user notifications
func (l *Workflow) buildUserNotificationData(orderInfo *order.Order, outcome *subscription.FulfillmentOutcome) map[string]string {
	data := map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"SubscribeName": outcome.PlanName,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
	}

	if outcome.HasSub {
		data["ExpireTime"] = outcome.ExpireAt.Format("2006-01-02 15:04:05")
		data["ResetTime"] = timeutil.Now().Format("2006-01-02 15:04:05")
	}

	return data
}

// buildAdminNotificationData creates template data for admin notifications
func (l *Workflow) buildAdminNotificationData(orderInfo *order.Order, userInfo *user.User, outcome *subscription.FulfillmentOutcome) map[string]string {
	subscribeName := outcome.PlanName
	if orderInfo.Type == OrderTypeResetTraffic {
		subscribeName = "流量重置"
	}

	return map[string]string{
		"OrderNo":       orderInfo.OrderNo,
		"TradeNo":       orderInfo.TradeNo,
		"UserEmail":     findEmail(userInfo),
		"SubscribeName": subscribeName,
		"OrderAmount":   fmt.Sprintf("%.2f", float64(orderInfo.Price)/100),
		"OrderStatus":   "已支付",
		"OrderTime":     orderInfo.CreatedAt.Format("2006-01-02 15:04:05"),
		"PaymentMethod": orderInfo.Method,
	}
}

// sendUserNotifyWithTelegram delivers rendered MarkdownV2 to the buyer's
// bound Telegram; "no binding" and "no bot" both just mean nothing to send.
func (l *Workflow) sendUserNotifyWithTelegram(ctx context.Context, userID int64, text string) {
	if l.deps.NotificationsEnabled == nil || !l.deps.NotificationsEnabled() {
		return
	}
	if err := l.deps.Notifications.NotifyTelegramUser(ctx, userID, text); err != nil {
		logger.WithContext(ctx).Info("Telegram user notice skipped",
			logger.Field("reason", err.Error()), logger.Field("user_id", userID))
	}
}

// sendAdminNotifyWithTelegram posts into the admin group's notification
// topic - the group is the only administrator channel, so an unconfigured
// group means the notice is skipped.
func (l *Workflow) sendAdminNotifyWithTelegram(ctx context.Context, text string) {
	if l.deps.NotificationsEnabled == nil || !l.deps.NotificationsEnabled() {
		return
	}
	if err := l.deps.Notifications.NotifyAdminsTelegram(ctx, text); err != nil {
		logger.WithContext(ctx).Info("Telegram admin notice skipped", logger.Field("reason", err.Error()))
	}
}

// findEmail returns the user's email auth identifier, falling back to the
// numeric id so the admin notification always names the buyer.
func findEmail(u *user.User) string {
	for _, item := range u.AuthMethods {
		if item.AuthType == "email" {
			return item.AuthIdentifier
		}
	}
	return fmt.Sprintf("ID:%d", u.Id)
}
