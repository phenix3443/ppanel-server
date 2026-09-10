// Package announcement implements the announcement subdomain of the support
// module. Only the module facade (internal/module/support) may reach it.
package announcement

import (
	"context"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/announcement"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type Service struct {
	repo repository.AnnouncementRepo
}

func NewService(repo repository.AnnouncementRepo) *Service {
	return &Service{repo: repo}
}

func (s *Service) Create(ctx context.Context, req *dto.CreateAnnouncementRequest) error {
	if err := s.repo.Insert(ctx, &entity.Announcement{
		Title:   req.Title,
		Content: req.Content,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateAnnouncement] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create announcement failed: %v", err.Error())
	}
	return nil
}

func (s *Service) Update(ctx context.Context, req *dto.UpdateAnnouncementRequest) error {
	info, err := s.repo.FindOne(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[UpdateAnnouncement] Query Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get announcement error: %v", err.Error())
	}
	info.Title = req.Title
	info.Content = req.Content
	if req.Show != nil {
		info.Show = req.Show
	}
	if req.Pinned != nil {
		info.Pinned = req.Pinned
	}
	if req.Popup != nil {
		info.Popup = req.Popup
	}
	if err := s.repo.Update(ctx, info); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateAnnouncement] Update Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update announcement error: %v", err.Error())
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, req *dto.DeleteAnnouncementRequest) error {
	if err := s.repo.Delete(ctx, req.Id); err != nil {
		logger.WithContext(ctx).Errorw("[DeleteAnnouncement] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "delete announcement failed: %v", err.Error())
	}
	return nil
}

func (s *Service) Get(ctx context.Context, req *dto.GetAnnouncementRequest) (*dto.Announcement, error) {
	info, err := s.repo.FindOne(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetAnnouncement] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get announcement error: %v", err.Error())
	}
	resp := &dto.Announcement{}
	mapping.DeepCopy(resp, info)
	return resp, nil
}

func (s *Service) List(ctx context.Context, req *dto.GetAnnouncementListRequest) (*dto.GetAnnouncementListResponse, error) {
	total, list, err := s.repo.GetAnnouncementListByPage(ctx, int(req.Page), int(req.Size), entity.Filter{
		Show:   req.Show,
		Pinned: req.Pinned,
		Popup:  req.Popup,
		Search: req.Search,
	})
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetAnnouncementListByPage error: %v", err.Error())
	}
	resp := &dto.GetAnnouncementListResponse{}
	resp.Total = total
	resp.List = make([]dto.Announcement, 0)
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}

func (s *Service) QueryVisible(ctx context.Context, req *dto.QueryAnnouncementRequest) (*dto.QueryAnnouncementResponse, error) {
	enable := true
	total, list, err := s.repo.GetAnnouncementListByPage(ctx, req.Page, req.Size, entity.Filter{
		Show:   &enable,
		Pinned: req.Pinned,
		Popup:  req.Popup,
	})
	if err != nil {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "GetAnnouncementListByPage error: %v", err.Error())
	}
	resp := &dto.QueryAnnouncementResponse{}
	resp.Total = total
	resp.List = make([]dto.Announcement, 0)
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}
