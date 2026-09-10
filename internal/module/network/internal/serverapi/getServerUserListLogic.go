package serverapi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"uuid"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/xerr"
)

type GetServerUserListLogic struct {
	logger.Logger
	ctx      context.Context
	deps     Deps
	request  RequestMeta
	response ResponseMeta
}

// NewGetServerUserListLogic Get user list
func newGetServerUserListLogic(ctx context.Context, deps Deps, request RequestMeta) *GetServerUserListLogic {
	return &GetServerUserListLogic{
		Logger:   logger.WithContext(ctx),
		ctx:      ctx,
		deps:     deps,
		request:  request,
		response: NewResponseMeta(),
	}
}

func (l *GetServerUserListLogic) ResponseMeta() ResponseMeta {
	return l.response
}

// The placeholder is generated only while rebuilding an empty user list.
// Cache hits reuse the serialized UUID and ETag until the list expires/changes.
func placeholderServerUser() dto.ServerUser {
	return dto.ServerUser{
		Id:   1,
		UUID: uuid.NewV7().String(),
	}
}

func (l *GetServerUserListLogic) queryMatchedSubscribes(nodeIds []int64, nodeTags []string) ([]*subscribe.Subscribe, error) {
	return l.deps.Store.Subscribe().FindByNodeScope(l.ctx, nodeIds, nodeTags)
}

func (l *GetServerUserListLogic) GetServerUserList(req *dto.GetServerUserListRequest) (resp *dto.GetServerUserListResponse, err error) {
	cacheKey := fmt.Sprintf("%s%d:%s", node.ServerUserListCacheKey, req.ServerId, req.Protocol)
	cache, err := l.deps.Redis.Get(l.ctx, cacheKey).Result()
	if cache != "" {
		etag := httpx.GenerateETag([]byte(cache))
		resp = &dto.GetServerUserListResponse{}
		//  Check If-None-Match header
		if match := l.request.IfNoneMatch; match == etag {
			return nil, xerr.StatusNotModified
		}
		l.response.SetHeader("ETag", etag)
		err = json.Unmarshal([]byte(cache), resp)
		if err != nil {
			l.Errorw("[ServerUserListCacheKey] json unmarshal error", logger.Field("error", err.Error()))
			return nil, err
		}
		return resp, nil
	}
	generation, err := l.deps.Store.Node().ServerCacheGeneration(l.ctx, req.ServerId)
	if err != nil {
		return nil, err
	}
	server, err := l.deps.Store.Node().FindOneServer(l.ctx, req.ServerId)
	if err != nil {
		return nil, err
	}

	nodes, err := l.deps.Store.Node().ListNodes(l.ctx, &node.FilterNodeParams{
		ServerId: []int64{server.Id},
		Protocol: req.Protocol,
	})
	if err != nil {
		l.Errorw("FilterNodeList error", logger.Field("error", err.Error()))
		return nil, err
	}
	var nodeTag []string
	var nodeIds []int64
	for _, n := range nodes {
		nodeIds = append(nodeIds, n.Id)
		if n.Tags != "" {
			nodeTag = append(nodeTag, strings.Split(n.Tags, ",")...)
		}
	}

	subs, err := l.queryMatchedSubscribes(nodeIds, nodeTag)
	if err != nil {
		l.Errorw("QuerySubscribeIdsByServerIdAndServerGroupId error", logger.Field("error", err.Error()))
		return nil, err
	}
	if len(subs) == 0 {
		resp = &dto.GetServerUserListResponse{
			Users: []dto.ServerUser{placeholderServerUser()},
		}
		return l.storeUserListResponse(req.ServerId, generation, cacheKey, resp)
	}
	type candidate struct {
		userSub *usersub.Subscribe
		plan    *subscribe.Subscribe
	}
	planIDs := make([]int64, 0, len(subs))
	plansByID := make(map[int64]*subscribe.Subscribe, len(subs))
	for _, sub := range subs {
		planIDs = append(planIDs, sub.Id)
		plansByID[sub.Id] = sub
	}
	if err := l.deps.Store.UserSubscription().ActivatePendingSubscribesBySubscribeIds(l.ctx, planIDs); err != nil {
		return nil, err
	}
	data, err := l.deps.Store.UserSubscription().FindUsersSubscribeBySubscribeIds(l.ctx, planIDs)
	if err != nil {
		return nil, err
	}
	candidates := make([]candidate, 0, len(data))
	for _, datum := range data {
		if plan := plansByID[datum.SubscribeId]; plan != nil {
			candidates = append(candidates, candidate{userSub: datum, plan: plan})
		}
	}
	userIDs := make([]int64, 0, len(candidates))
	for _, item := range candidates {
		userIDs = append(userIDs, item.userSub.UserId)
	}
	enabledIDs, err := l.deps.Store.User().FindEnabledUserIDs(l.ctx, slicesx.RemoveDuplicateElements(userIDs...))
	if err != nil {
		return nil, err
	}
	enabled := make(map[int64]struct{}, len(enabledIDs))
	for _, id := range enabledIDs {
		enabled[id] = struct{}{}
	}
	users := make([]dto.ServerUser, 0, len(candidates))
	for _, item := range candidates {
		if _, ok := enabled[item.userSub.UserId]; !ok {
			continue
		}
		users = append(users, dto.ServerUser{
			Id: item.userSub.Id, UUID: item.userSub.UUID,
			SpeedLimit: item.plan.SpeedLimit, DeviceLimit: item.plan.DeviceLimit,
		})
	}
	if len(users) == 0 {
		users = append(users, placeholderServerUser())
	}
	resp = &dto.GetServerUserListResponse{
		Users: users,
	}
	return l.storeUserListResponse(req.ServerId, generation, cacheKey, resp)
}

func (l *GetServerUserListLogic) storeUserListResponse(serverID, generation int64, cacheKey string, resp *dto.GetServerUserListResponse) (*dto.GetServerUserListResponse, error) {
	val, _ := json.Marshal(resp)
	etag := httpx.GenerateETag(val)
	l.response.SetHeader("ETag", etag)
	if err := l.deps.Store.Node().SetServerCache(l.ctx, serverID, cacheKey, string(val), generation); err != nil {
		l.Errorw("[ServerUserListCacheKey] cache set error", logger.Field("error", err.Error()))
	}
	//  Check If-None-Match header
	if match := l.request.IfNoneMatch; match == etag {
		return nil, xerr.StatusNotModified
	}
	return resp, nil
}
