package application

import (
	"context"
	"encoding/json"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetSubscribeApplicationListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetSubscribeApplicationListLogic Get subscribe application list
func newGetSubscribeApplicationListLogic(ctx context.Context, deps Deps) *GetSubscribeApplicationListLogic {
	return &GetSubscribeApplicationListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSubscribeApplicationListLogic) GetSubscribeApplicationList(req *dto.GetSubscribeApplicationListRequest) (resp *dto.GetSubscribeApplicationListResponse, err error) {
	data, err := l.deps.Clients.List(l.ctx)
	if err != nil {
		l.Errorf("Failed to get subscribe application list: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Failed to get subscribe application list")
	}
	var list []dto.SubscribeApplication
	for _, item := range data {
		var temp dto.DownloadLink
		if item.DownloadLink != "" {
			_ = json.Unmarshal([]byte(item.DownloadLink), &temp)
		}
		list = append(list, dto.SubscribeApplication{
			Id:                item.Id,
			Name:              item.Name,
			Description:       item.Description,
			Icon:              item.Icon,
			Scheme:            item.Scheme,
			UserAgent:         item.UserAgent,
			IsDefault:         item.IsDefault,
			SubscribeTemplate: item.SubscribeTemplate,
			OutputFormat:      item.OutputFormat,
			DefaultParams:     item.DefaultParams,
			DownloadLink:      temp,
			CreatedAt:         item.CreatedAt.UnixMilli(),
			UpdatedAt:         item.UpdatedAt.UnixMilli(),
		})
	}
	resp = &dto.GetSubscribeApplicationListResponse{
		Total: int64(len(list)),
		List:  list,
	}
	return
}
