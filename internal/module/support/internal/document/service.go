// Package document implements the document subdomain of the support module.
// Only the module facade (internal/module/support) may reach it.
package document

import (
	"context"
	"regexp"
	"strings"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/document"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// SubscriptionReader is the support module's port onto the subscription
// domain. The composition root satisfies it today by wrapping the legacy
// repository; the subscription module facade will implement it once that
// module exists (ADR-001).
type SubscriptionReader interface {
	HasActiveSubscription(ctx context.Context, userID int64) (bool, error)
}

// Subscription-gated conditional blocks in document content. Stripped
// server-side so gated content (e.g. shared credentials) never reaches users
// without an active subscription.
var (
	reIfSubscribed    = regexp.MustCompile(`(?s)\{\{#if_subscribed\}\}(.*?)\{\{/if_subscribed\}\}`)
	reIfNotSubscribed = regexp.MustCompile(`(?s)\{\{#if_not_subscribed\}\}(.*?)\{\{/if_not_subscribed\}\}`)
)

type Service struct {
	repo repository.DocumentRepo
	subs SubscriptionReader
}

func NewService(repo repository.DocumentRepo, subs SubscriptionReader) *Service {
	return &Service{repo: repo, subs: subs}
}

func (s *Service) Create(ctx context.Context, req *dto.CreateDocumentRequest) error {
	if err := s.repo.Insert(ctx, &entity.Document{
		Title:   req.Title,
		Content: req.Content,
		Tags:    strings.Join(req.Tags, ","),
		Show:    req.Show,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateDocument] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert document error: %v", err.Error())
	}
	return nil
}

func (s *Service) Update(ctx context.Context, req *dto.UpdateDocumentRequest) error {
	if err := s.repo.Update(ctx, &entity.Document{
		Id:      req.Id,
		Title:   req.Title,
		Content: req.Content,
		Tags:    strings.Join(req.Tags, ","),
		Show:    req.Show,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateDocument] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "failed to update document: %v", err.Error())
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, req *dto.DeleteDocumentRequest) error {
	if err := s.repo.Delete(ctx, req.Id); err != nil {
		logger.WithContext(ctx).Errorw("[DeleteDocument] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "failed to delete document: %v", err.Error())
	}
	return nil
}

func (s *Service) BatchDelete(ctx context.Context, req *dto.BatchDeleteDocumentRequest) error {
	for _, id := range req.Ids {
		if err := s.repo.Delete(ctx, id); err != nil {
			logger.WithContext(ctx).Errorw("[BatchDeleteDocument] Database Error", logger.Field("error", err.Error()))
			return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseDeletedError), "failed to delete document: %v", err.Error())
		}
	}
	return nil
}

func (s *Service) GetDetail(ctx context.Context, req *dto.GetDocumentDetailRequest) (*dto.Document, error) {
	data, err := s.repo.QueryDocumentDetail(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetDocumentDetail] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryDocumentDetail error: %v", err.Error())
	}
	return &dto.Document{
		Id:        data.Id,
		Title:     data.Title,
		Tags:      slicesx.StringMergeAndRemoveDuplicates(data.Tags),
		Content:   data.Content,
		CreatedAt: data.CreatedAt.UnixMilli(),
		UpdatedAt: data.UpdatedAt.UnixMilli(),
	}, nil
}

func (s *Service) List(ctx context.Context, req *dto.GetDocumentListRequest) (*dto.GetDocumentListResponse, error) {
	total, data, err := s.repo.QueryDocumentList(ctx, int(req.Page), int(req.Size), req.Tag, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetDocumentList] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryDocumentList error: %v", err.Error())
	}
	resp := &dto.GetDocumentListResponse{
		Total: total,
		List:  make([]dto.Document, 0),
	}
	for _, v := range data {
		resp.List = append(resp.List, dto.Document{
			Id:        v.Id,
			Title:     v.Title,
			Tags:      slicesx.StringMergeAndRemoveDuplicates(v.Tags),
			Content:   v.Content,
			Show:      *v.Show,
			CreatedAt: v.CreatedAt.UnixMilli(),
			UpdatedAt: v.UpdatedAt.UnixMilli(),
		})
	}
	return resp, nil
}

func (s *Service) QueryDetail(ctx context.Context, req *dto.QueryDocumentDetailRequest) (*dto.Document, error) {
	data, err := s.repo.FindOne(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[QueryDocumentDetailLogic] FindOne error", logger.Field("id", req.Id), logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOne error: %s", err.Error())
	}
	resp := &dto.Document{}
	mapping.DeepCopy(resp, data)
	resp.Content = s.renderConditional(ctx, resp.Content)
	return resp, nil
}

// renderConditional keeps or drops {{#if_subscribed}}...{{/if_subscribed}} and
// {{#if_not_subscribed}}...{{/if_not_subscribed}} blocks based on whether the
// current user has an active subscription. Done here (server-side) so gated
// content is never sent to users who shouldn't see it.
func (s *Service) renderConditional(ctx context.Context, content string) string {
	if content == "" {
		return content
	}

	hasSubscription := false
	if u, ok := ctx.Value(requestctx.CtxKeyUser).(*user.User); ok && u != nil && s.subs != nil {
		active, err := s.subs.HasActiveSubscription(ctx, u.Id)
		if err != nil {
			logger.WithContext(ctx).Errorw("[QueryDocumentDetailLogic] QueryUserSubscribe error", logger.Field("error", err.Error()), logger.Field("user_id", u.Id))
		} else {
			hasSubscription = active
		}
	}

	if hasSubscription {
		content = reIfSubscribed.ReplaceAllString(content, "$1")
		content = reIfNotSubscribed.ReplaceAllString(content, "")
	} else {
		content = reIfSubscribed.ReplaceAllString(content, "")
		content = reIfNotSubscribed.ReplaceAllString(content, "$1")
	}
	return content
}

func (s *Service) QueryList(ctx context.Context) (*dto.QueryDocumentListResponse, error) {
	total, data, err := s.repo.GetDocumentListByAll(ctx)
	if err != nil {
		logger.WithContext(ctx).Errorw("[QueryDocumentList] error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryDocumentList error: %v", err.Error())
	}
	resp := &dto.QueryDocumentListResponse{
		Total: total,
		List:  make([]dto.Document, 0),
	}
	for _, item := range data {
		resp.List = append(resp.List, dto.Document{
			Id:        item.Id,
			Title:     item.Title,
			Tags:      slicesx.StringMergeAndRemoveDuplicates(item.Tags),
			UpdatedAt: item.UpdatedAt.UnixMilli(),
		})
	}
	return resp, nil
}
