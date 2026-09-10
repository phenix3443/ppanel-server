package authmethodadmin

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/mail"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type TestEmailSendLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Test email send
func newTestEmailSendLogic(ctx context.Context, deps Deps) *TestEmailSendLogic {
	return &TestEmailSendLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *TestEmailSendLogic) TestEmailSend(req *dto.TestEmailSendRequest) error {
	client, err := mail.NewSender(l.deps.Config().EmailPlatform, l.deps.Config().EmailPlatformConfig, l.deps.Config().SiteName)
	if err != nil {
		l.Errorw("new email sender err", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "new email sender err: %v", err.Error())
	}
	err = client.Send([]string{req.Email}, "Test Email Send", "this a test email send by ppanel")
	if err != nil {
		return errors.Wrapf(xerr.NewErrCodeMsg(500, fmt.Sprintf("send email err: %v", err.Error())), "send email err: %v", err.Error())
	}
	return nil
}
