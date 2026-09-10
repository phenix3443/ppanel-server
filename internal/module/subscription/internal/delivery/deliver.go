// Package delivery implements the subscription delivery subdomain of the
// subscription module: token-authenticated rendering of client configs via
// the adapter, with notice placeholders for expired/exhausted subscriptions.
package delivery

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/platform/entity/client"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/module/subscription/internal/render"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type SubscribeLogic struct {
	ctx     context.Context
	deps    Deps
	cfg     Config
	request RequestMeta
	logger.Logger
}

// RequestMeta carries the raw transport details of the subscription request.
type RequestMeta struct {
	Host       string
	RequestURI string
	UserAgent  string
	ClientIP   string
}

func newSubscribeLogic(ctx context.Context, deps Deps, request RequestMeta) *SubscribeLogic {
	return &SubscribeLogic{
		ctx:     ctx,
		deps:    deps,
		cfg:     deps.config(),
		request: request,
		Logger:  logger.WithContext(ctx),
	}
}

func (l *SubscribeLogic) Handler(req *dto.SubscribeRequest) (resp *dto.SubscribeResponse, err error) {
	// query client list
	clients, err := l.deps.Clients.List(l.ctx)
	if err != nil {
		l.Errorw("[SubscribeLogic] Query client list failed", logger.Field("error", err.Error()))
		return nil, err
	}

	userAgent := strings.ToLower(l.request.UserAgent)

	var targetApp, defaultApp *client.SubscribeApplication

	for _, item := range clients {
		u := strings.ToLower(item.UserAgent)
		if item.IsDefault {
			defaultApp = item
		}

		if strings.Contains(userAgent, u) {
			// Special handling for Stash
			if strings.Contains(userAgent, "stash") && !strings.Contains(u, "stash") {
				continue
			}
			targetApp = item
			break
		}
	}
	if targetApp == nil {
		l.Debugw("[SubscribeLogic] No matching client found", logger.Field("userAgent", userAgent))
		if defaultApp == nil {
			return nil, errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "No matching client found for user agent: %s", userAgent)
		}
		targetApp = defaultApp
	}
	// Find user subscribe by token
	userSubscribe, err := l.getUserSubscribe(req.Token)
	if err != nil {
		l.Errorw("[SubscribeLogic] Get user subscribe failed", logger.Field("error", err.Error()))
		return nil, err
	}

	// find subscribe info
	subscribeInfo, err := l.deps.Plans.FindOne(l.ctx, userSubscribe.SubscribeId)
	if err != nil {
		l.Errorw("[SubscribeLogic] Find subscribe info failed", logger.Field("error", err.Error()), logger.Field("subscribeId", userSubscribe.SubscribeId))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "Find subscribe info failed: %v", err.Error())
	}

	// Find server list by user subscribe
	servers, err := l.getServers(userSubscribe, subscribeInfo)
	if err != nil {
		return nil, err
	}
	defaultParams, err := targetApp.DefaultParamValues()
	if err != nil {
		// A malformed default must not cost the user their subscription; fall back
		// to whatever the request carried.
		l.Errorw("[SubscribeLogic] Ignoring malformed default params",
			logger.Field("application", targetApp.Name),
			logger.Field("defaultParams", targetApp.DefaultParams),
			logger.Field("error", err.Error()))
	}

	a := render.NewAdapter(
		targetApp.SubscribeTemplate,
		render.WithServers(servers),
		render.WithSiteName(l.cfg.SiteName),
		render.WithSubscribeName(subscribeInfo.Name),
		render.WithOutputFormat(targetApp.OutputFormat),
		render.WithUserInfo(render.User{
			ID:           userSubscribe.Id,
			Password:     userSubscribe.UUID,
			ExpiredAt:    userSubscribe.ExpireTime,
			Download:     userSubscribe.Download,
			Upload:       userSubscribe.Upload,
			Traffic:      userSubscribe.Traffic,
			SubscribeURL: l.getSubscribeV2URL(),
		}),
		render.WithParams(mergeParams(defaultParams, req.Params)),
	)

	l.Debugw("[SubscribeLogic] Building client config",
		logger.Field("user_id", userSubscribe.UserId),
		logger.Field("application", targetApp.Name),
	)

	// Get client config
	adapterClient, err := a.Client()
	if err != nil {
		l.Errorw("[SubscribeLogic] Client error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(500), "Client error: %v", err.Error())
	}
	bytes, err := adapterClient.Build()
	if err != nil {
		l.Errorw("[SubscribeLogic] Build client config failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(500), "Build client config failed: %v", err.Error())
	}

	var formats = []string{"json", "yaml", "conf"}

	headers := make(map[string]string)
	for _, format := range formats {
		if format == strings.ToLower(targetApp.OutputFormat) {
			headers["Content-Disposition"] = fmt.Sprintf("attachment;filename*=UTF-8''%s", url.PathEscape(l.cfg.SiteName))
			headers["Content-Type"] = "application/octet-stream; charset=UTF-8"
			if l.cfg.ProfileUpdateInterval > 0 {
				headers["profile-update-interval"] = fmt.Sprintf("%d", l.cfg.ProfileUpdateInterval)
			}
			if profileURL := strings.TrimSpace(l.cfg.ProfileWebPageURL); profileURL != "" {
				headers["profile-web-page-url"] = profileURL
			}
		}
	}

	resp = &dto.SubscribeResponse{
		Config: bytes,
		Header: fmt.Sprintf(
			"upload=%d;download=%d;total=%d;expire=%d",
			userSubscribe.Upload, userSubscribe.Download, userSubscribe.Traffic, userSubscribe.ExpireTime.Unix(),
		),
		Headers: headers,
	}
	if err = l.logSubscribeActivity(userSubscribe); err != nil {
		return nil, err
	}
	return
}

func (l *SubscribeLogic) getSubscribeV2URL() string {
	uri := l.request.RequestURI
	// use custom domain if configured
	if l.cfg.SubscribeDomain != "" {
		domains := strings.Split(l.cfg.SubscribeDomain, "\n")
		return fmt.Sprintf("https://%s%s", domains[0], uri)
	}
	// use current request host
	return fmt.Sprintf("https://%s%s", l.request.Host, uri)
}

// getUserSubscribe 是本次修改的核心部分
func (l *SubscribeLogic) getUserSubscribe(token string) (*usersub.Subscribe, error) {
	userSub, err := l.deps.UserSubs.FindOneSubscribeByToken(l.ctx, token)
	if err != nil {
		l.Infow("[Generate Subscribe]find subscribe error: %v", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find subscribe error: %v", err.Error())
	}

	// =========================================================
	// 修复开始：添加空指针检查 (Fix start)
	// =========================================================
	if userSub == nil {
		l.Infow("[Generate Subscribe] token invalid or user not found")
		return nil, errors.New("subscribe token invalid")
	}
	// =========================================================
	// Check if user is enabled
	userInfo, err := l.deps.Users.FindAccountState(l.ctx, userSub.UserId)
	if err != nil {
		l.Infow("[Generate Subscribe] failed to get user info", logger.Field("error", err.Error()), logger.Field("userId", userSub.UserId))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "failed to get user info: %v", err.Error())
	}
	if userInfo.DeletedAt.Valid {
		l.Infow("[Generate Subscribe] user account is deleted", logger.Field("userId", userSub.UserId))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserNotExist), "User account does not exist")
	}
	if userInfo.Enable == nil || !*userInfo.Enable {
		l.Infow("[Generate Subscribe] user account is disabled", logger.Field("userId", userSub.UserId))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.UserDisabled), "User account is disabled")
	}
	// 修复结束 (Fix end)
	// =========================================================

	//  Ignore expiration check
	//if userSub.Status > 1 {
	// l.Infow("[Generate Subscribe]subscribe is not available", logger.Field("status", int(userSub.Status)), logger.Field("token", token))
	// return nil, errors.Wrapf(xerr.NewErrCode(xerr.SubscribeNotAvailable), "subscribe is not available")
	//}

	return userSub, nil
}

func (l *SubscribeLogic) logSubscribeActivity(userSub *usersub.Subscribe) error {
	subscribeLog := log.Subscribe{
		Token:           logger.RedactedValue,
		UserAgent:       l.request.UserAgent,
		ClientIP:        l.request.ClientIP,
		UserSubscribeId: userSub.Id,
	}

	content, err := subscribeLog.Marshal()
	if err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.ERROR), "marshal subscription audit log: %v", err)
	}

	err = l.deps.Logs.Insert(l.ctx, &log.SystemLog{
		Type:     log.TypeSubscribe.Uint8(),
		ObjectID: userSub.UserId, // log user id
		Date:     timeutil.Now().Format(time.DateOnly),
		Content:  string(content),
	})
	if err != nil {
		l.Errorw("[Generate Subscribe]insert subscribe log error: %v", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert subscription audit log: %v", err)
	}
	return nil
}

func (l *SubscribeLogic) getServers(userSub *usersub.Subscribe, subDetails *subscribe.Subscribe) ([]*node.Node, error) {
	if l.isSubscriptionExpired(userSub) {
		return l.createNoticeServers("订阅已过期 / Subscribe Expired"), nil
	}

	if l.isTrafficExhausted(userSub) {
		return l.createNoticeServers("流量已用尽 / Traffic Exhausted"), nil
	}

	nodeIds := slicesx.StringToInt64Slice(subDetails.Nodes)
	tags := slicesx.RemoveStringElement(strings.Split(subDetails.NodeTags, ","), "")

	l.Debugf("[Generate Subscribe]nodes: %v, NodeTags: %v", len(nodeIds), len(tags))
	if len(nodeIds) == 0 && len(tags) == 0 {
		logger.Infow("[Generate Subscribe]no subscribe nodes")
		return []*node.Node{}, nil
	}
	enable := true
	nodes, err := l.deps.Nodes.ListNodesByScope(l.ctx, nodeIds, slicesx.RemoveDuplicateElements(tags...), &enable, true)

	l.Debugf("[Query Subscribe]found servers: %v", len(nodes))

	if err != nil {
		l.Errorw("[Generate Subscribe]find server details error: %v", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find server details error: %v", err.Error())
	}
	logger.Debugf("[Generate Subscribe]found servers: %v", len(nodes))
	return nodes, nil
}

func (l *SubscribeLogic) isSubscriptionExpired(userSub *usersub.Subscribe) bool {
	return userSub.ExpireTime.Unix() < timeutil.Now().Unix() && userSub.ExpireTime.Unix() != 0
}

// isTrafficExhausted reports whether the subscription has used up its traffic
// quota. Traffic == 0 means unlimited. Mirrors the condition used by
// FindTrafficExceededSubscribes (upload + download >= traffic AND traffic > 0).
func (l *SubscribeLogic) isTrafficExhausted(userSub *usersub.Subscribe) bool {
	return userSub.Traffic > 0 && userSub.Download+userSub.Upload >= userSub.Traffic
}

// createNoticeServers returns placeholder (non-functional) nodes whose names
// carry a notice (e.g. expired / traffic exhausted) so clients display the
// reason instead of silently failing.
func (l *SubscribeLogic) createNoticeServers(message string) []*node.Node {
	enable := true
	host := l.getFirstHostLine()

	return []*node.Node{
		{
			Name:    message,
			Tags:    "",
			Port:    18080,
			Address: "127.0.0.1",
			Server: &node.Server{
				Id:        1,
				Name:      message,
				Protocols: "[{\"type\":\"shadowsocks\",\"cipher\":\"aes-256-gcm\",\"port\":1}]",
			},
			Protocol: "shadowsocks",
			Enabled:  &enable,
		},
		{
			Name:    host,
			Tags:    "",
			Port:    18080,
			Address: "127.0.0.1",
			Server: &node.Server{
				Id:        1,
				Name:      message,
				Protocols: "[{\"type\":\"shadowsocks\",\"cipher\":\"aes-256-gcm\",\"port\":1}]",
			},
			Protocol: "shadowsocks",
			Enabled:  &enable,
		},
	}
}

func (l *SubscribeLogic) getFirstHostLine() string {
	host := l.cfg.Host
	lines := strings.Split(host, "\n")
	if len(lines) > 0 {
		return lines[0]
	}
	return host
}
