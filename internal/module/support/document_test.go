package support_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/support"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	docEntity "github.com/perfect-panel/server/internal/module/support/entity/document"
)

type fakeDocumentRepo struct {
	findOne    *docEntity.Document
	deletedIDs []int64
	deleteErr  error
}

func (f *fakeDocumentRepo) Insert(_ context.Context, _ *docEntity.Document) error { return nil }

func (f *fakeDocumentRepo) FindOne(_ context.Context, _ int64) (*docEntity.Document, error) {
	return f.findOne, nil
}

func (f *fakeDocumentRepo) Update(_ context.Context, _ *docEntity.Document) error { return nil }

func (f *fakeDocumentRepo) Delete(_ context.Context, id int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deletedIDs = append(f.deletedIDs, id)
	return nil
}

func (f *fakeDocumentRepo) QueryDocumentDetail(_ context.Context, _ int64) (*docEntity.Document, error) {
	return f.findOne, nil
}

func (f *fakeDocumentRepo) QueryDocumentList(_ context.Context, _, _ int, _, _ string) (int64, []*docEntity.Document, error) {
	return 0, nil, nil
}

func (f *fakeDocumentRepo) GetDocumentListByAll(_ context.Context) (int64, []*docEntity.Document, error) {
	return 0, nil, nil
}

type fakeSubscriptionReader struct {
	active bool
	err    error
}

func (f fakeSubscriptionReader) HasActiveSubscription(_ context.Context, _ int64) (bool, error) {
	return f.active, f.err
}

const gatedContent = "public {{#if_subscribed}}secret{{/if_subscribed}}{{#if_not_subscribed}}subscribe first{{/if_not_subscribed}}"

func newDocService(repo *fakeDocumentRepo, subs support.SubscriptionReader) support.Service {
	return support.New(support.Deps{Documents: repo, Subscriptions: subs})
}

func ctxWithUser(id int64) context.Context {
	return context.WithValue(context.Background(), requestctx.CtxKeyUser, &user.User{Id: id})
}

func queryGated(t *testing.T, svc support.Service, ctx context.Context) string {
	t.Helper()
	resp, err := svc.QueryDocumentDetail(ctx, &dto.QueryDocumentDetailRequest{Id: 1})
	if err != nil {
		t.Fatalf("QueryDocumentDetail: %v", err)
	}
	return resp.Content
}

func TestQueryDocumentDetailShowsGatedContentToSubscribers(t *testing.T) {
	repo := &fakeDocumentRepo{findOne: &docEntity.Document{Id: 1, Content: gatedContent}}
	svc := newDocService(repo, fakeSubscriptionReader{active: true})

	content := queryGated(t, svc, ctxWithUser(9))
	if !strings.Contains(content, "secret") || strings.Contains(content, "subscribe first") {
		t.Fatalf("subscriber must see gated content only: %q", content)
	}
}

func TestQueryDocumentDetailHidesGatedContentWithoutSubscription(t *testing.T) {
	repo := &fakeDocumentRepo{findOne: &docEntity.Document{Id: 1, Content: gatedContent}}
	svc := newDocService(repo, fakeSubscriptionReader{active: false})

	content := queryGated(t, svc, ctxWithUser(9))
	if strings.Contains(content, "secret") || !strings.Contains(content, "subscribe first") {
		t.Fatalf("non-subscriber must not see gated content: %q", content)
	}
}

func TestQueryDocumentDetailHidesGatedContentForAnonymousUser(t *testing.T) {
	repo := &fakeDocumentRepo{findOne: &docEntity.Document{Id: 1, Content: gatedContent}}
	svc := newDocService(repo, fakeSubscriptionReader{active: true})

	content := queryGated(t, svc, context.Background())
	if strings.Contains(content, "secret") {
		t.Fatalf("anonymous user must not see gated content: %q", content)
	}
}

func TestQueryDocumentDetailTreatsPortErrorAsNoSubscription(t *testing.T) {
	repo := &fakeDocumentRepo{findOne: &docEntity.Document{Id: 1, Content: gatedContent}}
	svc := newDocService(repo, fakeSubscriptionReader{active: true, err: errors.New("boom")})

	content := queryGated(t, svc, ctxWithUser(9))
	if strings.Contains(content, "secret") {
		t.Fatalf("port failure must fail closed: %q", content)
	}
}

func TestBatchDeleteDocumentDeletesEachID(t *testing.T) {
	repo := &fakeDocumentRepo{}
	svc := newDocService(repo, nil)

	err := svc.BatchDeleteDocument(context.Background(), &dto.BatchDeleteDocumentRequest{Ids: []int64{1, 2, 3}})
	if err != nil {
		t.Fatalf("BatchDeleteDocument: %v", err)
	}
	if len(repo.deletedIDs) != 3 {
		t.Fatalf("deleted %v, want 3 ids", repo.deletedIDs)
	}
}
