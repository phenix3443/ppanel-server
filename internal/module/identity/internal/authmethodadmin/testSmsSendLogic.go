package authmethodadmin

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/sms"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type TestSmsSendLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Test sms send
func newTestSmsSendLogic(ctx context.Context, deps Deps) *TestSmsSendLogic {
	return &TestSmsSendLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TestSmsSendLogic) TestSmsSend(req *dto.TestSmsSendRequest) error {
	client, err := sms.NewSender(l.deps.Config().MobilePlatform, l.deps.Config().MobilePlatformConfig)
	if err != nil {
		l.Errorw("new sms sender err", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "new sms sender err: %v", err.Error())
	}
	err = client.SendCode(req.AreaCode, req.Telephone, "123456")
	if err != nil {
		l.Errorw("send sms err", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCodeMsg(500, fmt.Sprintf("send sms err: %v", err.Error())), "send sms err: %v", err.Error())
	}
	return nil
}
