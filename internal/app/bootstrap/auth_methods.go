package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/pkg/logger"
)

// Email get email smtp config
func Email(ctx *Dependencies) {
	logger.Debug("Email config initialization")
	method, err := ctx.Store.Auth().FindOneByMethod(context.Background(), "email")
	if err != nil {
		panic(fmt.Sprintf("[Error] Initialization Failed to find email auth method: %v", err.Error()))
	}
	var cfg config.EmailConfig
	var emailConfig = new(auth.EmailAuthConfig)
	emailConfig.Unmarshal(method.Config)
	mapping.DeepCopy(&cfg, emailConfig)
	cfg.Enable = *method.Enabled
	value, _ := json.Marshal(emailConfig.PlatformConfig)
	cfg.PlatformConfig = string(value)
	ctx.updateConfig(func(current *config.Config) { current.Email = cfg })
}

func Mobile(ctx *Dependencies) {
	logger.Debug("Mobile config initialization")
	method, err := ctx.Store.Auth().FindOneByMethod(context.Background(), "mobile")
	if err != nil {
		panic(err)
	}
	var cfg config.MobileConfig
	var mobileConfig auth.MobileAuthConfig
	mobileConfig.Unmarshal(method.Config)
	mapping.DeepCopy(&cfg, mobileConfig)
	cfg.Enable = *method.Enabled
	value, _ := json.Marshal(mobileConfig.PlatformConfig)
	cfg.PlatformConfig = string(value)
	ctx.updateConfig(func(current *config.Config) { current.Mobile = cfg })
}

func Device(ctx *Dependencies) {
	logger.Debug("device config initialization")
	method, err := ctx.Store.Auth().FindOneByMethod(context.Background(), "device")
	if err != nil {
		panic(err)
	}
	var cfg config.DeviceConfig
	var deviceConfig auth.DeviceConfig
	deviceConfig.Unmarshal(method.Config)
	mapping.DeepCopy(&cfg, deviceConfig)
	cfg.Enable = *method.Enabled
	ctx.updateConfig(func(current *config.Config) { current.Device = cfg })
}
