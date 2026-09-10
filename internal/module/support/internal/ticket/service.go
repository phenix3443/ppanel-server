// Package ticket implements the ticket subdomain of the support module. Only
// the module facade (internal/module/support) may reach it.
package ticket

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Notifier mirrors ticket lifecycle into other channels — today the forum
// topic in the Telegram admin group. Implementations are best-effort by
// contract: they never fail the ticket operation, so the methods return
// nothing and do their own logging.
type Notifier interface {
	TicketCreated(ctx context.Context, t *entity.Ticket)
	TicketReplied(ctx context.Context, ticketID int64, from, content string)
	TicketStatusChanged(ctx context.Context, ticketID int64, status uint8)
}

type Service struct {
	repo   repository.TicketRepo
	notify Notifier
}

// NewService builds the ticket service; notify may be nil when no mirror
// channel is wired.
func NewService(repo repository.TicketRepo, notify Notifier) *Service {
	return &Service{repo: repo, notify: notify}
}

func currentUser(ctx context.Context) (*user.User, error) {
	u, ok := ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	return u, nil
}

// CreateFollow appends an admin reply and flips the ticket back to Waiting.
func (s *Service) CreateFollow(ctx context.Context, req *dto.CreateTicketFollowRequest) error {
	if _, err := s.repo.FindOne(ctx, req.TicketId); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] FindOne error", logger.Field("error", err.Error()), logger.Field("ticketId", req.TicketId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find ticket failed: %v", err.Error())
	}
	if err := s.repo.InsertTicketFollow(ctx, &entity.Follow{
		TicketId: req.TicketId,
		From:     req.From,
		Type:     req.Type,
		Content:  req.Content,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] Database insert error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create ticket follow failed: %v", err.Error())
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.TicketId, 0, entity.Waiting); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] Database update error", logger.Field("error", err.Error()), logger.Field("status", entity.Waiting))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket status failed: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketReplied(ctx, req.TicketId, req.From, req.Content)
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetTicketListRequest) (*dto.GetTicketListResponse, error) {
	total, list, err := s.repo.QueryTicketList(ctx, int(req.Page), int(req.Size), req.UserId, req.Status, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetTicketList] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryTicketList error: %v", err)
	}
	resp := &dto.GetTicketListResponse{
		Total: total,
		List:  make([]dto.Ticket, 0),
	}
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}

func (s *Service) GetDetail(ctx context.Context, req *dto.GetTicketRequest) (*dto.Ticket, error) {
	data, err := s.repo.QueryTicketDetail(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetTicket] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get ticket detail failed: %v", err.Error())
	}
	resp := &dto.Ticket{}
	mapping.DeepCopy(resp, data)
	return resp, nil
}

func (s *Service) UpdateStatus(ctx context.Context, req *dto.UpdateTicketStatusRequest) error {
	if err := s.repo.UpdateTicketStatus(ctx, req.Id, 0, *req.Status); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateTicketStatus] Update Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketStatusChanged(ctx, req.Id, *req.Status)
	}
	return nil
}

func (s *Service) CreateUserTicket(ctx context.Context, req *dto.CreateUserTicketRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	// Insert backfills the id, which the mirror channel needs for its topic.
	t := &entity.Ticket{
		Title:       req.Title,
		Description: req.Description,
		UserId:      u.Id,
		Status:      entity.Pending,
	}
	if err := s.repo.Insert(ctx, t); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketCreated(ctx, t)
	}
	return nil
}

// CreateUserFollow appends a user reply after verifying ticket ownership and
// flips the ticket to Pending.
func (s *Service) CreateUserFollow(ctx context.Context, req *dto.CreateUserTicketFollowRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	t, err := s.repo.FindOne(ctx, req.TicketId)
	if err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database query error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query ticket failed: %v", err.Error())
	}
	if u.Id != t.UserId {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Invalid access", logger.Field("user_id", u.Id), logger.Field("ticket_user_id", t.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "invalid access")
	}
	if err := s.repo.InsertTicketFollow(ctx, &entity.Follow{
		TicketId: req.TicketId,
		From:     req.From,
		Type:     req.Type,
		Content:  req.Content,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database insert error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create ticket follow failed: %v", err.Error())
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.TicketId, u.Id, entity.Pending); err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database update error", logger.Field("error", err.Error()), logger.Field("status", entity.Pending))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket status failed: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketReplied(ctx, req.TicketId, req.From, req.Content)
	}
	return nil
}

func (s *Service) GetUserDetail(ctx context.Context, req *dto.GetUserTicketDetailRequest) (*dto.Ticket, error) {
	data, err := s.repo.QueryTicketDetail(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetUserTicketDetailsLogic] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get ticket detail failed: %v", err.Error())
	}
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if data.UserId != u.Id {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "invalid access")
	}
	resp := &dto.Ticket{}
	mapping.DeepCopy(resp, data)
	return resp, nil
}

func (s *Service) GetUserList(ctx context.Context, req *dto.GetUserTicketListRequest) (*dto.GetUserTicketListResponse, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	logger.WithContext(ctx).Debugf("Current user: %v", u.Id)
	total, list, err := s.repo.QueryTicketList(ctx, req.Page, req.Size, u.Id, req.Status, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetUserTicketListLogic] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryTicketList error: %v", err)
	}
	resp := &dto.GetUserTicketListResponse{
		Total: total,
		List:  make([]dto.Ticket, 0),
	}
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}

func (s *Service) UpdateUserStatus(ctx context.Context, req *dto.UpdateUserTicketStatusRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.Id, u.Id, *req.Status); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateUserTicketStatusLogic] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketStatusChanged(ctx, req.Id, *req.Status)
	}
	return nil
}
