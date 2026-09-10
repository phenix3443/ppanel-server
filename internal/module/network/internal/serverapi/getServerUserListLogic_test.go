package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"uuid"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

func TestPlaceholderServerUserUsesNewUUIDV7(t *testing.T) {
	seen := make(map[uuid.UUID]bool)
	for range 32 {
		user := placeholderServerUser()
		id, err := uuid.Parse(user.UUID)
		if err != nil {
			t.Fatal(err)
		}
		if user.Id != 1 || id[6]>>4 != 7 || id[8]>>6 != 2 {
			t.Fatalf("invalid V7 placeholder: %+v", user)
		}
		if seen[id] {
			t.Fatal("placeholder generation reused a UUID")
		}
		seen[id] = true
	}
}

func TestCachedPlaceholderKeepsUUIDAndETag(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	placeholder := placeholderServerUser()
	payload, err := json.Marshal(dto.GetServerUserListResponse{Users: []dto.ServerUser{placeholder}})
	if err != nil {
		t.Fatal(err)
	}
	req := &dto.GetServerUserListRequest{ServerId: 1, Protocol: "vless"}
	server.Set(fmt.Sprintf("%s%d:%s", node.ServerUserListCacheKey, req.ServerId, req.Protocol), string(payload))
	// No repositories are provided: a cache hit must not rebuild the list.
	for range 2 {
		logic := newGetServerUserListLogic(context.Background(), Deps{Redis: client}, RequestMeta{})
		resp, err := logic.GetServerUserList(req)
		if err != nil || len(resp.Users) != 1 || resp.Users[0].UUID != placeholder.UUID {
			t.Fatalf("cached user list changed: %+v, %v", resp, err)
		}
	}
	logic := newGetServerUserListLogic(context.Background(), Deps{Redis: client}, RequestMeta{IfNoneMatch: httpx.GenerateETag(payload)})
	if resp, err := logic.GetServerUserList(req); resp != nil || err != xerr.StatusNotModified {
		t.Fatalf("cached ETag no longer returns not-modified: %+v, %v", resp, err)
	}
}
