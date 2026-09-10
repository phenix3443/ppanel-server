package dashboard

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
)

type QueryTicketWaitReplyLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewQueryTicketWaitReplyLogic Query ticket wait reply
func newQueryTicketWaitReplyLogic(ctx context.Context, deps Deps) *QueryTicketWaitReplyLogic {
	return &QueryTicketWaitReplyLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *QueryTicketWaitReplyLogic) QueryTicketWaitReply() (resp *dto.TicketWaitRelpyResponse, err error) {
	count, err := l.deps.Tickets.QueryWaitReplyTotal(l.ctx)
	if err != nil {
		l.Errorw("[QueryTicketWaitReply] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, err
	}
	return &dto.TicketWaitRelpyResponse{
		Count: count,
	}, nil
}
