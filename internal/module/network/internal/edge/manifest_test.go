package edge

import (
	"context"
	"errors"
	"testing"
	"time"

	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type edgeManifestUserSubs struct {
	repository.UserSubscriptionRepo
	sub *usersub.Subscribe
}

func (r edgeManifestUserSubs) FindOneSubscribeByToken(context.Context, string) (*usersub.Subscribe, error) {
	return r.sub, nil
}

type edgeManifestUsers struct {
	repository.UserRepo
	user *userEntity.User
}

func (r edgeManifestUsers) FindOne(context.Context, int64) (*userEntity.User, error) {
	return r.user, nil
}

func (r edgeManifestUsers) FindAccountState(context.Context, int64) (*userEntity.AccountState, error) {
	return &userEntity.AccountState{
		Id: r.user.Id, Enable: r.user.Enable, UpdatedAt: r.user.UpdatedAt, DeletedAt: r.user.DeletedAt,
	}, nil
}

type edgeManifestStore struct {
	repository.Store
	userSubs repository.UserSubscriptionRepo
	users    repository.UserRepo
}

func (s edgeManifestStore) UserSubscription() repository.UserSubscriptionRepo { return s.userSubs }
func (s edgeManifestStore) User() repository.UserRepo                         { return s.users }

func TestProxyFromProtocol(t *testing.T) {
	item := &node.Node{Id: 7, Name: "Tokyo", Address: "jp.example.com", Port: 443, Protocol: "vless", Tags: "asia, premium", Sort: 3}
	protocol := node.Protocol{
		Type:      "vless",
		Enable:    true,
		Security:  "tls",
		SNI:       "jp.example.com",
		Transport: "ws",
		Host:      "cdn.example.com",
		Path:      "/ws",
		Flow:      "xtls-rprx-vision",
	}

	proxy, supported, reason := proxyFromProtocol(item, protocol, "00000000-0000-4000-8000-000000000001")
	if !supported || reason != "" {
		t.Fatalf("expected proxy to be supported, got supported=%v reason=%q", supported, reason)
	}
	if proxy.UUID == "" || proxy.TLS == nil || proxy.Transport == nil {
		t.Fatalf("expected credentials, tls and transport, got %#v", proxy)
	}
	if proxy.Transport.Type != "ws" || proxy.Transport.Host != "cdn.example.com" {
		t.Fatalf("unexpected transport: %#v", proxy.Transport)
	}
}

func TestProxyFromProtocolRejectsUnsupportedWorkerFeatures(t *testing.T) {
	item := &node.Node{Name: "Reality", Address: "example.com", Port: 443}
	_, supported, reason := proxyFromProtocol(item, node.Protocol{Type: "vless", Enable: true, Security: "reality"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected reality node to be rejected, got supported=%v reason=%q", supported, reason)
	}

	_, supported, reason = proxyFromProtocol(item, node.Protocol{Type: "shadowsocks", Enable: true, Cipher: "2022-blake3-aes-128-gcm"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected shadowsocks 2022 node to be rejected, got supported=%v reason=%q", supported, reason)
	}

	_, supported, reason = proxyFromProtocol(item, node.Protocol{Type: "shadowsocks", Enable: true, Cipher: "aes-128-gcm", Security: "tls"}, "user-secret")
	if supported || reason == "" {
		t.Fatalf("expected Shadowsocks TLS node to be rejected, got supported=%v reason=%q", supported, reason)
	}
}

func TestSubscriptionState(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if state := subscriptionState(&usersub.Subscribe{Status: 1, Traffic: 100, Upload: 40, Download: 60}, now); state != "traffic_exhausted" {
		t.Fatalf("expected traffic exhaustion, got %q", state)
	}
	if state := subscriptionState(&usersub.Subscribe{Status: 5}, now); state != "suspended" {
		t.Fatalf("expected suspended, got %q", state)
	}
	if state := subscriptionState(&usersub.Subscribe{Status: 255}, now); state != "disabled" {
		t.Fatalf("expected unknown status to be disabled, got %q", state)
	}
}

func TestManifestHidesDeletedAccount(t *testing.T) {
	enabled := true
	store := edgeManifestStore{
		userSubs: edgeManifestUserSubs{sub: &usersub.Subscribe{UserId: 9}},
		users: edgeManifestUsers{user: &userEntity.User{
			Id: 9, Enable: &enabled, DeletedAt: gorm.DeletedAt{Valid: true},
		}},
	}
	logic := newManifestLogic(context.Background(), Deps{Store: store})

	if _, err := logic.Manifest("deleted-user-token"); !errors.Is(err, ErrManifestNotFound) {
		t.Fatalf("Manifest error = %v, want ErrManifestNotFound", err)
	}
}
