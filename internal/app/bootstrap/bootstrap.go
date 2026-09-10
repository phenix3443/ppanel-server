package bootstrap

import (
	"context"
	"errors"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/perfect-panel/server/internal/app/migration/schema"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/billing"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
)

// Dependencies is the startup/reconfiguration boundary. It owns only mutable
// runtime configuration and the services needed to load or publish it.
type Dependencies struct {
	Config                   func() config.Config
	UpdateConfig             func(func(*config.Config))
	Store                    repository.Store
	ExchangeRate             *billing.CurrencyRateCache
	Notification             notification.Service
	SetTelegramBot           func(*tgbot.Bot)
	SetNodeMultiplierManager func(*network.MultiplierManager)
}

func (d *Dependencies) currentConfig() config.Config {
	if d == nil || d.Config == nil {
		return config.Config{}
	}
	return d.Config()
}

func (d *Dependencies) updateConfig(update func(*config.Config)) {
	if d != nil && d.UpdateConfig != nil {
		d.UpdateConfig(update)
	}
}

// Start loads startup state in dependency order. Migration and node-secret
// provisioning must precede every node configuration read.
func Start(deps *Dependencies) {
	Migrate(deps)
	Site(deps)
	NodeSecret(deps)
	Node(deps)
	Email(deps)
	Device(deps)
	Invite(deps)
	Verify(deps)
	Subscribe(deps)
	Register(deps)
	Mobile(deps)
	Currency(deps)
	Telegram(deps)
}

// Reload refreshes the subsystem changed by an administrator. Startup-only
// migration and node-secret provisioning are deliberately excluded.
func Reload(deps *Dependencies, subsystem string) {
	switch subsystem {
	case "verify":
		Verify(deps)
	case "node":
		Node(deps)
	case "telegram":
		Telegram(deps)
	case "currency":
		Currency(deps)
	case "register":
		Register(deps)
	case "site":
		Site(deps)
	case "invite":
		Invite(deps)
	case "subscribe":
		Subscribe(deps)
	case "email":
		Email(deps)
	case "mobile":
		Mobile(deps)
	case "device":
		Device(deps)
	}
}

func Migrate(ctx *Dependencies) {
	current := ctx.currentConfig()
	mc := orm.Mysql{
		Config: current.DatabaseConfig(),
	}
	now := time.Now()
	if err := schema.Up(mc.Driver(), mc.MigrationDsn()); err != nil {
		if errors.Is(err, schema.NoChange) {
			logger.Info("[Migrate] database not change")
			return
		}
		logger.Errorf("[Migrate] Up error: %v", err.Error())
		panic(err)
	} else {
		logger.Info("[Migrate] Database change, took " + time.Since(now).String())
	}
	// if not found admin user
	err := ctx.Store.InTx(context.Background(), func(store repository.Store) error {
		count, err := store.User().QueryResisterUserTotal(context.Background())
		if err != nil {
			return err
		}
		if count == 0 {
			enable := true
			admin := &user.User{
				Password:  password.EncodePassWord(current.Administrator.Password),
				Algo:      password.PasswordAlgoArgon2id,
				IsAdmin:   &enable,
				ReferCode: user.GenerateInviteCode(time.Now().Unix()),
			}
			if err := store.User().Insert(context.Background(), admin); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			if err := store.UserAuth().InsertUserAuthMethods(context.Background(), &user.AuthMethods{
				UserId:         admin.Id,
				AuthType:       "email",
				AuthIdentifier: current.Administrator.Email,
				Verified:       true,
			}); err != nil {
				logger.Errorf("[Migrate] CreateAdminUser error: %v", err.Error())
				return err
			}
			logger.Info("[Migrate] Create admin user success")
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
}
