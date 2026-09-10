package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type UpdatePrivacyPolicyConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Update Privacy Policy Config
func newUpdatePrivacyPolicyConfigLogic(ctx context.Context, deps Deps) *UpdatePrivacyPolicyConfigLogic {
	return &UpdatePrivacyPolicyConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdatePrivacyPolicyConfigLogic) UpdatePrivacyPolicyConfig(req *dto.PrivacyPolicyConfig) error {
	err := updateConfigFields(l.ctx, l.deps, "tos", convertedConfigFields(*req))
	if err != nil {
		l.Errorw("[UpdateTosConfigLogic] update tos config error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update tos config error: %v", err)
	}

	return nil
}
