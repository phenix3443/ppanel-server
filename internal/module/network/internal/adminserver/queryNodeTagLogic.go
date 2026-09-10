package adminserver

import (
	"context"
	"strings"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type QueryNodeTagLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryNodeTagLogic Query all node tags
func newQueryNodeTagLogic(ctx context.Context, deps Deps) *QueryNodeTagLogic {
	return &QueryNodeTagLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryNodeTagLogic) QueryNodeTag() (resp *dto.QueryNodeTagResponse, err error) {

	nodeTags, err := l.deps.Store.Node().QueryNodeTags(l.ctx)
	if err != nil {
		l.Errorw("[QueryNodeTag] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[QueryNodeTag] Query Database Error")
	}
	var tags []string
	for _, item := range nodeTags {
		tags = append(tags, strings.Split(item, ",")...)
	}

	return &dto.QueryNodeTagResponse{
		Tags: slicesx.RemoveDuplicateElements(tags...),
	}, nil
}
