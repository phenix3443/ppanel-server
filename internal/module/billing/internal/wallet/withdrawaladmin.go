package wallet

import (
	"context"
	"math"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type WithdrawalAdminLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newWithdrawalAdminLogic(ctx context.Context, deps Deps) *WithdrawalAdminLogic {
	return &WithdrawalAdminLogic{Logger: logger.WithContext(ctx), ctx: ctx, deps: deps}
}

func (l *WithdrawalAdminLogic) GetWithdrawalList(req *dto.GetWithdrawalListRequest) (*dto.GetWithdrawalListResponse, error) {
	data, total, err := l.deps.Withdrawals.QueryWithdrawalList(l.ctx, req.UserId, req.Status, req.Page, req.Size)
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query withdrawals failed: %v", err)
	}
	list := make([]dto.WithdrawalLog, 0, len(data))
	for _, item := range data {
		list = append(list, withdrawalDTO(item))
	}
	return &dto.GetWithdrawalListResponse{List: list, Total: total}, nil
}

func (l *WithdrawalAdminLogic) ReviewWithdrawal(req *dto.ReviewWithdrawalRequest) error {
	reason := strings.TrimSpace(req.Reason)
	if req.Status == walletEntity.WithdrawalStatusRejected && reason == "" {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "rejection reason is required")
	}
	if req.Status != walletEntity.WithdrawalStatusApproved && req.Status != walletEntity.WithdrawalStatusRejected {
		return errors.Wrap(xerr.NewErrCode(xerr.InvalidParams), "withdrawal status must be approved or rejected")
	}
	if req.Status == walletEntity.WithdrawalStatusApproved {
		reason = ""
	}

	return l.deps.Tx.InBillingTx(l.ctx, func(store repository.BillingStore) error {
		withdrawal, err := store.UserWithdrawal().FindWithdrawalForUpdate(l.ctx, req.Id)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find withdrawal failed: %v", err)
		}
		if withdrawal.Status != walletEntity.WithdrawalStatusPending {
			return errors.Wrap(xerr.NewErrCodeMsg(409, "WITHDRAWAL_ALREADY_REVIEWED"), "withdrawal is no longer pending")
		}

		if req.Status == walletEntity.WithdrawalStatusRejected {
			account, err := store.Wallet().FindOneForUpdate(l.ctx, withdrawal.UserId)
			if err != nil {
				return err
			}
			if withdrawal.Amount > math.MaxInt64-account.Commission {
				return errors.Wrap(xerr.NewErrCode(xerr.DatabaseUpdateError), "withdrawal refund would overflow commission balance")
			}
			account.Commission += withdrawal.Amount
			if err := store.Wallet().UpdateCommission(l.ctx, account); err != nil {
				return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "refund withdrawal commission failed: %v", err)
			}
			entry := log.Commission{Type: log.CommissionTypeWithdraw, Amount: withdrawal.Amount, Timestamp: timeutil.Now().UnixMilli()}
			content, err := entry.Marshal()
			if err != nil {
				return err
			}
			if err := store.Log().Insert(l.ctx, &log.SystemLog{
				Type: log.TypeCommission.Uint8(), Date: timeutil.Now().Format("2006-01-02"),
				ObjectID: withdrawal.UserId, Content: string(content), CreatedAt: timeutil.Now(),
			}); err != nil {
				return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "record withdrawal refund failed: %v", err)
			}
		}

		updated, err := store.UserWithdrawal().UpdateWithdrawalStatus(
			l.ctx, withdrawal.Id, walletEntity.WithdrawalStatusPending, req.Status, reason,
		)
		if err != nil {
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update withdrawal failed: %v", err)
		}
		if !updated {
			return errors.Wrap(xerr.NewErrCodeMsg(409, "WITHDRAWAL_ALREADY_REVIEWED"), "withdrawal changed concurrently")
		}
		return nil
	})
}

func withdrawalDTO(item *walletEntity.Withdrawal) dto.WithdrawalLog {
	if item == nil {
		return dto.WithdrawalLog{}
	}
	return dto.WithdrawalLog{
		Id: item.Id, UserId: item.UserId, Amount: item.Amount, Content: item.Content,
		Status: item.Status, Reason: item.Reason,
		CreatedAt: item.CreatedAt.UnixMilli(), UpdatedAt: item.UpdatedAt.UnixMilli(),
	}
}
