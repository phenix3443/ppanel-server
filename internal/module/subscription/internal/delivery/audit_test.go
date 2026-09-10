package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

type deliveryAuditRepo struct {
	repository.LogRepo
	row *log.SystemLog
	err error
}

func (r *deliveryAuditRepo) Insert(_ context.Context, row *log.SystemLog) error {
	if r.err != nil {
		return r.err
	}
	copy := *row
	r.row = &copy
	return nil
}

func TestSubscriptionAuditIsRedactedAndFailClosed(t *testing.T) {
	repo := &deliveryAuditRepo{}
	logic := newSubscribeLogic(context.Background(), Deps{Logs: repo}, RequestMeta{ClientIP: "192.0.2.1", UserAgent: "risk-client/1.0"})
	sub := &usersub.Subscribe{Id: 9, UserId: 7}

	if err := logic.logSubscribeActivity(sub); err != nil {
		t.Fatal(err)
	}
	if repo.row == nil || strings.Contains(repo.row.Content, "subscription-secret") || !strings.Contains(repo.row.Content, logger.RedactedValue) || !strings.Contains(repo.row.Content, "192.0.2.1") || !strings.Contains(repo.row.Content, "risk-client/1.0") {
		t.Fatalf("unsafe subscription audit: %+v", repo.row)
	}

	repo.err = errors.New("audit unavailable")
	if err := logic.logSubscribeActivity(sub); err == nil {
		t.Fatal("subscription audit failure was swallowed")
	}
}
