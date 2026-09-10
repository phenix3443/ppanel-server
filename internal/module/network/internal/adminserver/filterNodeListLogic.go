package adminserver

import (
	"context"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type FilterNodeListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewFilterNodeListLogic Filter Node List
func newFilterNodeListLogic(ctx context.Context, deps Deps) *FilterNodeListLogic {
	return &FilterNodeListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *FilterNodeListLogic) FilterNodeList(req *dto.FilterNodeListRequest) (resp *dto.FilterNodeListResponse, err error) {
	total, data, err := l.deps.Store.Node().FilterNodeList(l.ctx, &node.FilterNodeParams{
		Page:   req.Page,
		Size:   req.Size,
		Search: req.Search,
	})

	if err != nil {
		l.Errorw("[FilterNodeList] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[FilterNodeList] Query Database Error")
	}

	list := make([]dto.Node, 0)
	for _, datum := range data {
		list = append(list, dto.Node{
			Id:        datum.Id,
			Name:      datum.Name,
			Tags:      slicesx.RemoveDuplicateElements(strings.Split(datum.Tags, ",")...),
			Port:      datum.Port,
			Address:   datum.Address,
			ServerId:  datum.ServerId,
			Protocol:  datum.Protocol,
			Enabled:   datum.Enabled,
			Sort:      datum.Sort,
			CreatedAt: datum.CreatedAt.UnixMilli(),
			UpdatedAt: datum.UpdatedAt.UnixMilli(),
		})
	}

	return &dto.FilterNodeListResponse{
		List:  list,
		Total: total,
	}, nil
}
