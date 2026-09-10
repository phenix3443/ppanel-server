package systemsetting

import (
	"context"
	"encoding/json"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type SetNodeMultiplierLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Set Node Multiplier
func newSetNodeMultiplierLogic(ctx context.Context, deps Deps) *SetNodeMultiplierLogic {
	return &SetNodeMultiplierLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *SetNodeMultiplierLogic) SetNodeMultiplier(req *dto.SetNodeMultiplierRequest) error {
	data, err := json.Marshal(req.Periods)
	if err != nil {
		l.Logger.Error("Marshal Node Multiplier Config Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "Marshal Node Multiplier Config Error: %s", err.Error())
	}
	if err = l.deps.System.UpdateNodeMultiplierConfig(l.ctx, string(data)); err != nil {
		l.Logger.Error("Update Node Multiplier Config Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Update Node Multiplier Config Error: %s", err.Error())
	}
	// update Node Multiplier
	l.deps.reinit("node")

	return nil
}
