package app

import (
	"github.com/perfect-panel/server/internal/module/identity"
	"github.com/perfect-panel/server/internal/repository"
)

// newIdentityModule wires the identity module against the legacy store;
// device kicking is a closure over the service context's device manager.
func newIdentityModule(store repository.Store, srv *Application) identity.Service {
	return identity.New(identity.Deps{
		Users:     store.User(),
		UserAuths: store.UserAuth(),
		Devices:   store.UserDevice(),
		Cache:     store.UserCache(),
		UserSubs:  store.UserSubscription(),
		Plans:     store.Subscribe(),
		Traffic:   store.TrafficLog(),
		Logs:      store.Log(),
		Store:     store,
		KickDevice: func(userID int64, identifier string) {
			if srv.DeviceManager != nil {
				srv.DeviceManager.KickDevice(userID, identifier)
			}
		},

		Wallet: store.Wallet(),
		Auths:  store.Auth(),
		Redis:  srv.Redis,
		EmailDomains: func() (string, bool) {
			current := srv.Runtime.Config().Email
			return current.DomainSuffixList, current.EnableDomainSuffix
		},
		TelegramBotName: func() string { return srv.Runtime.Config().Telegram.BotName },
		NotifyTelegramUnbind: func(userID, chatID int64) error {
			return srv.Notification.NotifyTelegramUnbind(userID, chatID)
		},
		AuthConfig: func() identity.AuthSnapshot {
			c := srv.Runtime.Config()
			return identity.AuthSnapshot{
				JWTAccessSecret: c.JwtAuth.AccessSecret,
				JWTAccessExpire: c.JwtAuth.AccessExpire,

				EmailEnabled:            c.Email.Enable,
				EmailVerifyEnabled:      c.Email.EnableVerify,
				EmailDomainSuffixList:   c.Email.DomainSuffixList,
				EmailEnableDomainSuffix: c.Email.EnableDomainSuffix,
				MobileEnabled:           c.Mobile.Enable,
				DeviceEnabled:           c.Device.Enable,
				DeviceOnlyReal:          c.Device.OnlyRealDevice,

				InviteForced:      c.Invite.ForcedInvite,
				OnlyFirstPurchase: c.Invite.OnlyFirstPurchase,
				TrialEnabled:      c.Register.EnableTrial,
				TrialSubscribeID:  c.Register.TrialSubscribe,
				TrialTime:         c.Register.TrialTime,
				TrialTimeUnit:     c.Register.TrialTimeUnit,

				StopRegister:            c.Register.StopRegister,
				RegisterVerify:          c.Verify.RegisterVerify,
				TurnstileSecret:         c.Verify.TurnstileSecret,
				EnableIpRegisterLimit:   c.Register.EnableIpRegisterLimit,
				IpRegisterLimit:         c.Register.IpRegisterLimit,
				IpRegisterLimitDuration: c.Register.IpRegisterLimitDuration,

				SiteHost: c.Site.Host,
			}
		},
		VerifyQueue: srv.Queue,
		SenderConfig: func() identity.SenderSnapshot {
			c := srv.Runtime.Config()
			return identity.SenderSnapshot{
				EmailPlatform:        c.Email.Platform,
				EmailPlatformConfig:  c.Email.PlatformConfig,
				MobilePlatform:       c.Mobile.Platform,
				MobilePlatformConfig: c.Mobile.PlatformConfig,
				SiteName:             c.Site.SiteName,
			}
		},
		Reinitialize: srv.Runtime.Reinitialize,
		VerifyCodeConfig: func() identity.VerifyCodeSnapshot {
			c := srv.Runtime.Config()
			return identity.VerifyCodeSnapshot{
				DomainSuffixList:   c.Email.DomainSuffixList,
				EnableDomainSuffix: c.Email.EnableDomainSuffix,
				VerifyCodeInterval: c.VerifyCode.Interval,
				VerifyCodeLimit:    c.VerifyCode.Limit,
				VerifyCodeExpire:   c.VerifyCode.ExpireTime,
				SiteLogo:           c.Site.SiteLogo,
				SiteName:           c.Site.SiteName,
			}
		},
	})
}
