package arch_test

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/billing/transport/http/admin/coupon"
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/module/identity/transport/http/admin/authMethod"
	"github.com/perfect-panel/server/internal/module/platform"
	"github.com/perfect-panel/server/internal/module/platform/transport/http/admin/console"
	adminlog "github.com/perfect-panel/server/internal/module/platform/transport/http/admin/log"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/module/subscription/transport/http/admin/application"
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/module/support/transport/http/admin/ads"
	"github.com/perfect-panel/server/internal/module/support/transport/http/admin/announcement"
	"github.com/perfect-panel/server/internal/module/support/transport/http/admin/document"
)

func TestHandlerFactories_returnNativeHertzHandlers(t *testing.T) {
	// Given all owned admin handler factories
	// When their factory signatures are checked at compile time
	// Then each factory returns Hertz's native handler type.
	_ = t
	var _ func(support.Service) app.HandlerFunc = ads.CreateAdsHandler
	var _ func(support.Service) app.HandlerFunc = ads.DeleteAdsHandler
	var _ func(support.Service) app.HandlerFunc = ads.GetAdsDetailHandler
	var _ func(support.Service) app.HandlerFunc = ads.GetAdsListHandler
	var _ func(support.Service) app.HandlerFunc = ads.UpdateAdsHandler
	var _ func(support.Service) app.HandlerFunc = announcement.CreateAnnouncementHandler
	var _ func(support.Service) app.HandlerFunc = announcement.DeleteAnnouncementHandler
	var _ func(support.Service) app.HandlerFunc = announcement.GetAnnouncementHandler
	var _ func(support.Service) app.HandlerFunc = announcement.GetAnnouncementListHandler
	var _ func(support.Service) app.HandlerFunc = announcement.UpdateAnnouncementHandler
	var _ func(subscription.Service) app.HandlerFunc = application.CreateSubscribeApplicationHandler
	var _ func(subscription.Service) app.HandlerFunc = application.DeleteSubscribeApplicationHandler
	var _ func(subscription.Service) app.HandlerFunc = application.GetSubscribeApplicationListHandler
	var _ func(subscription.Service) app.HandlerFunc = application.PreviewSubscribeTemplateHandler
	var _ func(subscription.Service) app.HandlerFunc = application.UpdateSubscribeApplicationHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.GetAuthMethodConfigHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.GetAuthMethodListHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.GetEmailPlatformHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.GetSmsPlatformHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.TestEmailSendHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.TestSmsSendHandler
	var _ func(identity.Service) app.HandlerFunc = authMethod.UpdateAuthMethodConfigHandler
	var _ func(platform.Service) app.HandlerFunc = console.QueryRevenueStatisticsHandler
	var _ func(platform.Service) app.HandlerFunc = console.QueryServerTotalDataHandler
	var _ func(platform.Service) app.HandlerFunc = console.QueryTicketWaitReplyHandler
	var _ func(platform.Service) app.HandlerFunc = console.QueryUserStatisticsHandler
	var _ func(billing.Service) app.HandlerFunc = coupon.BatchDeleteCouponHandler
	var _ func(billing.Service) app.HandlerFunc = coupon.CreateCouponHandler
	var _ func(billing.Service) app.HandlerFunc = coupon.DeleteCouponHandler
	var _ func(billing.Service) app.HandlerFunc = coupon.GetCouponListHandler
	var _ func(billing.Service) app.HandlerFunc = coupon.UpdateCouponHandler
	var _ func(support.Service) app.HandlerFunc = document.BatchDeleteDocumentHandler
	var _ func(support.Service) app.HandlerFunc = document.CreateDocumentHandler
	var _ func(support.Service) app.HandlerFunc = document.DeleteDocumentHandler
	var _ func(support.Service) app.HandlerFunc = document.GetDocumentDetailHandler
	var _ func(support.Service) app.HandlerFunc = document.GetDocumentListHandler
	var _ func(support.Service) app.HandlerFunc = document.UpdateDocumentHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterBalanceLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterCommissionLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterEmailLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterGiftLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterLoginLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterMobileLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterOrderLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterRegisterLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterResetSubscribeLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterServerTrafficLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterSubscribeLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterTrafficLogDetailsHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.FilterUserSubscribeTrafficLogHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.GetLogSettingHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.GetMessageLogListHandler
	var _ func(platform.Service) app.HandlerFunc = adminlog.UpdateLogSettingHandler
}
