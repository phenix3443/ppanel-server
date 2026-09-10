package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type ResetSortWithNodeLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewResetSortWithNodeLogic Reset node sort
func newResetSortWithNodeLogic(ctx context.Context, deps Deps) *ResetSortWithNodeLogic {
	return &ResetSortWithNodeLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *ResetSortWithNodeLogic) ResetSortWithNode(req *dto.ResetSortRequest) error {
	err := l.deps.Store.InNetworkTx(l.ctx, func(store repository.NetworkStore) error {
		nodeStore := store.Node()
		currentItems, err := nodeStore.QueryNodeSorts(l.ctx)
		if err != nil {
			return err
		}
		currentSortMap := make(map[int64]int64)
		for _, item := range currentItems {
			currentSortMap[item.Id] = item.Sort
		}

		var itemsToUpdate []dto.NetworkSortItem
		for _, item := range req.Sort {
			if oldSort, exists := currentSortMap[item.Id]; exists && oldSort != item.Sort {
				itemsToUpdate = append(itemsToUpdate, item)
			}
		}
		for _, item := range itemsToUpdate {
			if err := nodeStore.UpdateNodeSort(l.ctx, item.Id, item.Sort); err != nil {
				l.Errorw("[NodeSort] Update Database Error: ", logger.Field("error", err.Error()), logger.Field("id", item.Id), logger.Field("sort", item.Sort))
				return err
			}
		}
		return nil
	})
	if err != nil {
		l.Errorw("[NodeSort] Update Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrap(xerr.NewErrCode(xerr.DatabaseUpdateError), err.Error())
	}
	return nil
}
