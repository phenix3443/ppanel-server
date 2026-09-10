package telegram

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type fakeBindStore struct {
	values  map[string]string
	deleted []string
}

func (s *fakeBindStore) Get(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", redis.Nil
	}
	return value, nil
}

func (s *fakeBindStore) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	delete(s.values, key)
	return nil
}

func (s *fakeBindStore) Set(_ context.Context, _, _ string, _ interface{}) error { return nil }

type fakeBindAuthRepo struct {
	repository.UserAuthRepo
	inserted []*user.AuthMethods
}

func (r *fakeBindAuthRepo) FindUserAuthMethodByOpenID(_ context.Context, _, _ string) (*user.AuthMethods, error) {
	return &user.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *fakeBindAuthRepo) FindUserAuthMethodByPlatform(_ context.Context, _ int64, _ string) (*user.AuthMethods, error) {
	return &user.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *fakeBindAuthRepo) InsertUserAuthMethods(_ context.Context, data *user.AuthMethods, _ ...*gorm.DB) error {
	r.inserted = append(r.inserted, data)
	return nil
}

type fakeBindUserCache struct {
	repository.UserCacheRepo
}

func (fakeBindUserCache) UpdateUserCache(_ context.Context, _ *user.User) error { return nil }

func newBindHarness(tokens map[string]string) (*TelegramLogic, *fakeBindStore, *fakeBindAuthRepo, *recordingTelegramMessenger) {
	store := &fakeBindStore{values: tokens}
	auths := &fakeBindAuthRepo{}
	messenger := &recordingTelegramMessenger{}
	logic := NewTelegramLogic(context.Background(), TelegramLogicDependencies{
		Messenger: messenger,
		Sessions:  store,
		UserAuth:  auths,
		UserCache: fakeBindUserCache{},
	})
	return logic, store, auths, messenger
}

// A session id must no longer work as a bind token: the deep link travels
// through Telegram chats, and accepting session ids let whoever saw one bind
// their own Telegram account — and therefore log in — as that user.
func TestBindRejectsSessionIdAsToken(t *testing.T) {
	token := "leaked-session-id"
	logic, _, auths, messenger := newBindHarness(map[string]string{
		fmt.Sprintf("%v:%v", config.SessionIdKey, token): "7",
	})

	if err := logic.bind(1001, token); err != nil {
		t.Fatalf("bind error = %v", err)
	}
	if len(auths.inserted) != 0 {
		t.Fatalf("session id was accepted as a bind token: %+v", auths.inserted[0])
	}
	if messenger.message != "Bind token is invalid or expired. Please request a new one." {
		t.Fatalf("message = %q", messenger.message)
	}
}

func TestStartRejectsSessionIdAsToken(t *testing.T) {
	token := "leaked-session-id"
	logic, _, auths, messenger := newBindHarness(map[string]string{
		fmt.Sprintf("%v:%v", config.SessionIdKey, token): "7",
	})

	msg := &models.Message{
		Text:     "/start " + token,
		Chat:     models.Chat{ID: 1001, Type: models.ChatTypePrivate},
		From:     &models.User{ID: 1001},
		Entities: []models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 6}},
	}
	if err := logic.start(msg); err != nil {
		t.Fatalf("start error = %v", err)
	}
	if len(auths.inserted) != 0 {
		t.Fatalf("session id was accepted as a bind token: %+v", auths.inserted[0])
	}
	if messenger.message != "Session expired. Please request a new bind link." {
		t.Fatalf("message = %q", messenger.message)
	}
}

// A dedicated bind token binds the account once and is invalidated, so a link
// that leaks after use cannot rebind the account.
func TestBindConsumesDedicatedTokenExactlyOnce(t *testing.T) {
	token := "bind-token"
	key := fmt.Sprintf("%v:%v", config.TelegramBindKey, token)
	logic, store, auths, messenger := newBindHarness(map[string]string{key: "7"})

	if err := logic.bind(1001, token); err != nil {
		t.Fatalf("bind error = %v", err)
	}
	if len(auths.inserted) != 1 {
		t.Fatalf("bindings = %d, want 1", len(auths.inserted))
	}
	if got := auths.inserted[0]; got.UserId != 7 || got.AuthIdentifier != strconv.FormatInt(1001, 10) {
		t.Fatalf("binding = %+v, want user 7 bound to chat 1001", got)
	}
	if len(store.deleted) != 1 || store.deleted[0] != key {
		t.Fatalf("deleted keys = %v, want [%s]", store.deleted, key)
	}
	if !messenger.markdown {
		t.Fatal("bind confirmation must be sent as MarkdownV2, not plain text")
	}

	// Replaying the same link finds nothing to redeem.
	auths.inserted = nil
	if err := logic.bind(2002, token); err != nil {
		t.Fatalf("second bind error = %v", err)
	}
	if len(auths.inserted) != 0 {
		t.Fatal("a consumed bind token was accepted again")
	}
	if messenger.message != "Bind token is invalid or expired. Please request a new one." {
		t.Fatalf("message = %q", messenger.message)
	}
}
