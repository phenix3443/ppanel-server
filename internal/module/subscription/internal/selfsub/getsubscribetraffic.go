package selfsub

import (
	"context"
	"sort"
	"time"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// hourKeyLayout matches the wall-clock key produced by the traffic repository.
const hourKeyLayout = "2006-01-02 15"

const (
	intervalHour = "hour"
	intervalDay  = "day"
)

type GetSubscribeTrafficLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

func newGetSubscribeTrafficLogic(ctx context.Context, deps Deps) *GetSubscribeTrafficLogic {
	return &GetSubscribeTrafficLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetSubscribeTrafficLogic) Overview(req *dto.GetSubscribeTrafficOverviewRequest) (*dto.GetSubscribeTrafficOverviewResponse, error) {
	userId, err := l.authorize(req.UserSubscribeId)
	if err != nil {
		return nil, err
	}

	window := req.Window
	if window == "" {
		window = dto.TrafficWindow24h
	}
	start, end, interval := resolveWindow(window, timeutil.Now())
	scope := trafficEntity.SubscribeTrafficScope{
		UserId:      userId,
		SubscribeId: req.UserSubscribeId,
		Start:       start,
		End:         end,
	}

	summary, err := l.deps.Traffic.QuerySubscribeTrafficSummary(l.ctx, scope)
	if err != nil {
		l.Errorw("QuerySubscribeTrafficSummary failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QuerySubscribeTrafficSummary failed: %v", err.Error())
	}
	hourly, err := l.deps.Traffic.QuerySubscribeHourlyTraffic(l.ctx, scope)
	if err != nil {
		l.Errorw("QuerySubscribeHourlyTraffic failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QuerySubscribeHourlyTraffic failed: %v", err.Error())
	}
	ranking, err := l.deps.Traffic.QuerySubscribeServerRanking(l.ctx, scope)
	if err != nil {
		l.Errorw("QuerySubscribeServerRanking failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QuerySubscribeServerRanking failed: %v", err.Error())
	}

	names := l.serverNames(collectServerIds(ranking))
	nodes := make([]dto.TrafficNodeUsageItem, 0, len(ranking))
	for _, row := range ranking {
		nodes = append(nodes, dto.TrafficNodeUsageItem{
			ServerId: row.ServerId,
			Name:     names[row.ServerId],
			Upload:   row.Upload,
			Download: row.Download,
			Total:    row.Total,
		})
	}

	return &dto.GetSubscribeTrafficOverviewResponse{
		Window:   window,
		Start:    start.UnixMilli(),
		End:      end.UnixMilli(),
		Upload:   summary.Upload,
		Download: summary.Download,
		Interval: interval,
		Series:   buildSeries(hourly, interval),
		Nodes:    nodes,
	}, nil
}

func (l *GetSubscribeTrafficLogic) Details(req *dto.GetSubscribeTrafficDetailsRequest) (*dto.GetSubscribeTrafficDetailsResponse, error) {
	userId, err := l.authorize(req.UserSubscribeId)
	if err != nil {
		return nil, err
	}

	start, end := time.UnixMilli(req.Start), time.UnixMilli(req.End)
	if req.Start == 0 || req.End == 0 {
		start, end, _ = resolveWindow(dto.TrafficWindow24h, timeutil.Now())
	}

	list, total, err := l.deps.Traffic.QueryTrafficLogDetails(l.ctx, &trafficEntity.TrafficLogDetailsFilter{
		UserId:      userId,
		SubscribeId: req.UserSubscribeId,
		Start:       start,
		End:         end,
		Page:        req.Page,
		Size:        req.Size,
	})
	if err != nil {
		l.Errorw("QueryTrafficLogDetails failed", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryTrafficLogDetails failed: %v", err.Error())
	}

	ids := make([]int64, 0, len(list))
	for _, row := range list {
		ids = append(ids, row.ServerId)
	}
	names := l.serverNames(ids)

	items := make([]dto.TrafficDetailItem, 0, len(list))
	for _, row := range list {
		items = append(items, dto.TrafficDetailItem{
			Timestamp: row.Timestamp.UnixMilli(),
			ServerId:  row.ServerId,
			Name:      names[row.ServerId],
			Upload:    row.Upload,
			Download:  row.Download,
		})
	}
	return &dto.GetSubscribeTrafficDetailsResponse{Total: total, List: items}, nil
}

// authorize resolves the caller and refuses a subscription owned by anybody
// else. The repository also filters on the returned user id, so a miss here
// cannot turn into a cross-user read.
func (l *GetSubscribeTrafficLogic) authorize(userSubscribeId int64) (int64, error) {
	u, ok := l.ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		l.Error("current user is not found in context")
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	userSub, err := l.deps.UserSubs.FindOneUserSubscribe(l.ctx, userSubscribeId)
	if err != nil {
		l.Errorw("FindOneUserSubscribe failed", logger.Field("error", err.Error()))
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "FindOneUserSubscribe failed: %v", err.Error())
	}
	if userSub.UserId != u.Id {
		l.Errorw("UserSubscribeId does not belong to the current user")
		return 0, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "UserSubscribeId does not belong to the current user")
	}
	return u.Id, nil
}

func (l *GetSubscribeTrafficLogic) serverNames(ids []int64) map[int64]string {
	names := make(map[int64]string, len(ids))
	if len(ids) == 0 {
		return names
	}
	servers, err := l.deps.Nodes.QueryServerList(l.ctx, dedupe(ids))
	if err != nil {
		// A missing name degrades the label only; the usage figures are still
		// correct, so this must not fail the request.
		l.Errorw("QueryServerList failed", logger.Field("error", err.Error()))
		return names
	}
	for _, server := range servers {
		if server != nil {
			names[server.Id] = server.Name
		}
	}
	return names
}

// resolveWindow maps a preset onto a half-open range plus the bucket size the
// chart should use. The hourly rows are rolled up in Go rather than grouped
// again in SQL so both bucket sizes come from one query.
func resolveWindow(window string, now time.Time) (time.Time, time.Time, string) {
	loc := timeutil.Location()
	now = now.In(loc)
	switch window {
	case dto.TrafficWindow7d:
		end := startOfDay(now, loc).AddDate(0, 0, 1)
		return end.AddDate(0, 0, -7), end, intervalDay
	case dto.TrafficWindow30d:
		end := startOfDay(now, loc).AddDate(0, 0, 1)
		return end.AddDate(0, 0, -30), end, intervalDay
	default:
		end := now.Truncate(time.Hour).Add(time.Hour)
		return end.Add(-24 * time.Hour), end, intervalHour
	}
}

func startOfDay(t time.Time, loc *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
}

// buildSeries turns wall-clock hour keys into instants, folding them into days
// when the window asked for daily buckets. Hours the database never returned
// stay absent: the client renders a gap rather than a fabricated zero.
func buildSeries(hourly []trafficEntity.HourlyTraffic, interval string) []dto.TrafficSeriesPoint {
	loc := timeutil.Location()
	totals := make(map[int64]*dto.TrafficSeriesPoint, len(hourly))
	for _, bucket := range hourly {
		parsed, err := time.ParseInLocation(hourKeyLayout, bucket.Hour, loc)
		if err != nil {
			continue
		}
		if interval == intervalDay {
			parsed = startOfDay(parsed, loc)
		}
		key := parsed.UnixMilli()
		point, ok := totals[key]
		if !ok {
			point = &dto.TrafficSeriesPoint{Timestamp: key}
			totals[key] = point
		}
		point.Upload += bucket.Upload
		point.Download += bucket.Download
	}

	series := make([]dto.TrafficSeriesPoint, 0, len(totals))
	for _, point := range totals {
		series = append(series, *point)
	}
	sort.Slice(series, func(i, j int) bool {
		return series[i].Timestamp < series[j].Timestamp
	})
	return series
}

func collectServerIds(rows []trafficEntity.ServerTrafficRanking) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ServerId)
	}
	return ids
}

func dedupe(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
