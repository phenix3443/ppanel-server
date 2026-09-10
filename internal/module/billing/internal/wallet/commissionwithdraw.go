package wallet

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type CommissionWithdrawLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Commission Withdraw
func newCommissionWithdrawLogic(ctx context.Context, deps Deps) *CommissionWithdrawLogic {
	return &CommissionWithdrawLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CommissionWithdrawLogic) CommissionWithdraw(req *dto.CommissionWithdrawRequest) (resp *dto.WithdrawalLog, err error) {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}

	if req.Amount <= 0 {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "withdraw amount must be positive")
	}
	content := strings.TrimSpace(req.Content)
	if content == "" || utf8.RuneCountInString(content) > 4000 {
		return nil, errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "withdrawal content is required and must not exceed 4000 characters")
	}

	var withdrawal walletEntity.Withdrawal
	err = l.deps.Tx.InBillingTx(l.ctx, func(store repository.BillingStore) error {
		// Do not rely on the user object placed in the request context: it can
		// be stale while another withdrawal or commission credit is committed.
		// The row lock serializes the balance check and debit.
		lockedUser, txErr := store.Wallet().FindOneForUpdate(l.ctx, u.Id)
		if txErr != nil {
			return txErr
		}
		if lockedUser.Commission < req.Amount {
			return errors.Wrapf(xerr.NewErrCode(xerr.UserCommissionNotEnough), "User %d has insufficient commission balance", u.Id)
		}
		lockedUser.Commission -= req.Amount
		if err = store.Wallet().UpdateCommission(l.ctx, lockedUser); err != nil {
			l.Errorf("Failed to update user %d commission balance: %v", u.Id, err)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "Failed to update user %d commission balance: %v", u.Id, err)
		}
		// Use negative amount to reflect the balance decrease, so that
		// SumAmountByTypeAndObjectID produces the correct net total.
		logInfo := log.Commission{
			Type:      log.CommissionTypeWithdraw,
			Amount:    -req.Amount,
			Timestamp: timeutil.Now().UnixMilli(),
		}
		b, marshalErr := logInfo.Marshal()
		if marshalErr != nil {
			return marshalErr
		}

		if err = store.Log().Insert(l.ctx, &log.SystemLog{
			Type:      log.TypeCommission.Uint8(),
			Date:      timeutil.Now().Format("2006-01-02"),
			ObjectID:  u.Id,
			Content:   string(b),
			CreatedAt: timeutil.Now(),
		}); err != nil {
			l.Errorf("Failed to create commission log for user %d: %v", u.Id, err)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "Failed to create commission log for user %d: %v", u.Id, err)
		}

		withdrawal = walletEntity.Withdrawal{
			UserId:  u.Id,
			Amount:  req.Amount,
			Content: content,
			Status:  walletEntity.WithdrawalStatusPending,
			Reason:  "",
		}
		if err = store.UserWithdrawal().InsertWithdrawal(l.ctx, &withdrawal); err != nil {
			l.Errorf("Failed to create withdrawal log for user %d: %v", u.Id, err)
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "Failed to create withdrawal log for user %d: %v", u.Id, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := withdrawalDTO(&withdrawal)
	return &result, nil
}
