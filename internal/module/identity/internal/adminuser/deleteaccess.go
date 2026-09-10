package adminuser

import (
	"context"
	"strings"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
)

// clearDeletedUserAccessCaches makes account deletion effective immediately
// for subscription tokens and the node-facing credential list. The database
// rows remain untouched; normal account-state checks remain authoritative.
func (l *DeleteUserLogic) clearDeletedUserAccessCaches(userIDs []int64) {
	clearUserAccessCaches(l.ctx, l.deps, userIDs)
}

func (l *BatchDeleteUserLogic) clearDeletedUserAccessCaches(userIDs []int64) {
	clearUserAccessCaches(l.ctx, l.deps, userIDs)
}

func clearUserAccessCaches(ctx context.Context, deps Deps, userIDs []int64) {
	if deps.UserSubs == nil || deps.Cache == nil || deps.Store == nil {
		return
	}
	log := logger.WithContext(ctx)
	serverIDs := make(map[int64]struct{})
	for _, userID := range slicesx.RemoveDuplicateElements(userIDs...) {
		details, err := deps.UserSubs.QueryUserSubscribe(ctx, userID)
		if err != nil {
			log.Errorw("query subscriptions while deleting user", logger.Field("user_id", userID), logger.Field("error", err.Error()))
			continue
		}
		subs := make([]*usersub.Subscribe, 0, len(details))
		for _, item := range details {
			if item == nil {
				continue
			}
			subs = append(subs, &usersub.Subscribe{Id: item.Id, UserId: item.UserId, Token: item.Token, SubscribeId: item.SubscribeId})
			if item.Subscribe != nil {
				collectPlanServerIDs(ctx, deps, item.Subscribe.Nodes, item.Subscribe.NodeTags, serverIDs)
			}
		}
		if err := deps.Cache.ClearSubscribeCache(ctx, subs...); err != nil {
			log.Errorw("clear deleted user subscription cache", logger.Field("user_id", userID), logger.Field("error", err.Error()))
		}
	}
	for serverID := range serverIDs {
		if err := deps.Store.Node().ClearServerCache(ctx, serverID); err != nil {
			log.Errorw("clear deleted user node cache", logger.Field("server_id", serverID), logger.Field("error", err.Error()))
		}
	}
}

func collectPlanServerIDs(ctx context.Context, deps Deps, nodes, tags string, serverIDs map[int64]struct{}) {
	queries := make([]*node.FilterNodeParams, 0, 2)
	if value := strings.TrimSpace(nodes); value != "" {
		queries = append(queries, &node.FilterNodeParams{
			Page: 1, Size: 9999,
			NodeId: slicesx.StringSliceToInt64Slice(strings.Split(value, ",")),
		})
	}
	if value := strings.TrimSpace(tags); value != "" {
		queries = append(queries, &node.FilterNodeParams{
			Page: 1, Size: 9999, Tag: strings.Split(value, ","),
		})
	}
	for _, filter := range queries {
		_, list, err := deps.Store.Node().FilterNodeList(ctx, filter)
		if err != nil {
			logger.WithContext(ctx).Errorw("resolve node caches while deleting user", logger.Field("error", err.Error()))
			continue
		}
		for _, item := range list {
			if item != nil && item.ServerId != 0 {
				serverIDs[item.ServerId] = struct{}{}
			}
		}
	}
}
