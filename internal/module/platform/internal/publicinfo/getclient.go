package publicinfo

import (
	"context"
	"encoding/json"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetClientLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Client
func newGetClientLogic(ctx context.Context, deps Deps) *GetClientLogic {
	return &GetClientLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetClientLogic) GetClient() (resp *dto.GetSubscribeClientResponse, err error) {
	data, err := l.deps.Store.Client().List(l.ctx)
	if err != nil {
		l.Errorf("Failed to get subscribe application list: %v", err)
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Failed to get subscribe application list")
	}
	var list []dto.SubscribeClient
	for _, item := range data {
		var temp dto.PlatformDownloadLinkSnapshot
		if item.DownloadLink != "" {
			_ = json.Unmarshal([]byte(item.DownloadLink), &temp)
		}
		list = append(list, dto.SubscribeClient{
			Id:           item.Id,
			Name:         item.Name,
			Description:  item.Description,
			Icon:         item.Icon,
			Scheme:       item.Scheme,
			IsDefault:    item.IsDefault,
			DownloadLink: temp,
		})
	}
	resp = &dto.GetSubscribeClientResponse{
		Total: int64(len(list)),
		List:  list,
	}
	return
}
