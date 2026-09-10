package traffic

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

type ServerDataLogic struct {
	deps Dependencies
}

func NewServerDataLogic(deps Dependencies) *ServerDataLogic {
	return &ServerDataLogic{
		deps: deps,
	}
}

func (l *ServerDataLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	serverData := dto.NetworkServerTotalDataSnapshot{}

	top10ServerToday, top10ServerYesterday, top10UserToday, top10UserYesterday := l.getRanking(ctx)
	if len(top10ServerToday) == 0 {
		top10ServerToday = make([]dto.NetworkServerTrafficSnapshot, 0)
	}
	if len(top10ServerYesterday) == 0 {
		top10ServerYesterday = make([]dto.NetworkServerTrafficSnapshot, 0)
	}
	if len(top10UserToday) == 0 {
		top10UserToday = make([]dto.NetworkUserTrafficSnapshot, 0)
	}
	if len(top10UserYesterday) == 0 {
		top10UserYesterday = make([]dto.NetworkUserTrafficSnapshot, 0)
	}
	serverData.ServerTrafficRankingToday = top10ServerToday
	serverData.ServerTrafficRankingYesterday = top10ServerYesterday
	serverData.UserTrafficRankingToday = top10UserToday
	serverData.UserTrafficRankingYesterday = top10UserYesterday
	totalUploadToday, totalDownloadToday, totalDownloadMonthly, totalUploadMonthly := l.trafficCount(ctx)
	serverData.TodayUpload = totalUploadToday
	serverData.TodayDownload = totalDownloadToday
	serverData.MonthlyUpload = totalUploadMonthly
	serverData.MonthlyDownload = totalDownloadMonthly
	serverData.UpdatedAt = timeutil.Now().UnixMilli()
	data, err := json.Marshal(serverData)
	if err != nil {
		logger.Error("[ServerDataLogic] Marshal server data failed", logger.Field("error", err.Error()), logger.Field("data", serverData))
		return err
	}
	if err := l.deps.Redis.Set(ctx, config.ServerCountCacheKey, data, -1).Err(); err != nil {
		logger.Error("[ServerDataLogic] Set server data failed", logger.Field("error", err.Error()))
		return err
	}
	logger.Info("[ServerDataLogic] Update server data success")
	return nil
}

func (l *ServerDataLogic) getRanking(ctx context.Context) (top10ServerToday, top10ServerYesterday []dto.NetworkServerTrafficSnapshot, top10UserToday, top10UserYesterday []dto.NetworkUserTrafficSnapshot) {
	now := timeutil.Now()
	// 获取服务器流量排行榜
	serverToday, err := l.deps.Store.TrafficLog().TopServersTrafficByDay(ctx, now, 10)
	if err != nil {
		logger.Error("[ServerDataLogic] Get top servers traffic by day failed", logger.Field("error", err.Error()))
	} else {
		for _, s := range serverToday {
			if s.ServerId == 0 {
				continue
			}
			serverInfo, err := l.deps.Store.Node().FindOneServer(ctx, s.ServerId)
			if err != nil {
				logger.Error("[ServerDataLogic] Find server failed", logger.Field("error", err.Error()))
				continue
			}
			top10ServerToday = append(top10ServerToday, dto.NetworkServerTrafficSnapshot{
				ServerId: s.ServerId,
				Name:     serverInfo.Name,
				Upload:   s.Upload,
				Download: s.Download,
			})
		}
	}

	serverYesterday, err := l.deps.Store.TrafficLog().TopServersTrafficByDay(ctx, now.AddDate(0, 0, -1), 10)
	if err != nil {
		logger.Error("[ServerDataLogic] Get top servers traffic by day failed", logger.Field("error", err.Error()))
	} else {
		for _, s := range serverYesterday {
			serverInfo, err := l.deps.Store.Node().FindOneServer(ctx, s.ServerId)
			if err != nil {
				logger.Error("[ServerDataLogic] Find server failed", logger.Field("error", err.Error()))
				continue
			}
			top10ServerYesterday = append(top10ServerYesterday, dto.NetworkServerTrafficSnapshot{
				ServerId: s.ServerId,
				Name:     serverInfo.Name,
				Upload:   s.Upload,
				Download: s.Download,
			})
		}
	}

	// 获取用户流量排行榜
	userToday, err := l.deps.Store.TrafficLog().TopUsersTrafficByDay(ctx, now, 10)
	if err != nil {
		logger.Error("[ServerDataLogic] Get top users traffic by day failed", logger.Field("error", err.Error()))
	} else {
		for _, u := range userToday {
			top10UserToday = append(top10UserToday, dto.NetworkUserTrafficSnapshot{
				SID:      u.SubscribeId,
				UID:      u.UserId,
				Upload:   u.Upload,
				Download: u.Download,
			})
		}
	}

	userYesterday, err := l.deps.Store.TrafficLog().TopUsersTrafficByDay(ctx, now.AddDate(0, 0, -1), 10)
	if err != nil {
		logger.Error("[ServerDataLogic] Get top users traffic by day failed", logger.Field("error", err.Error()))
	} else {
		for _, u := range userYesterday {
			top10UserYesterday = append(top10UserYesterday, dto.NetworkUserTrafficSnapshot{
				SID:      u.SubscribeId,
				UID:      u.UserId,
				Upload:   u.Upload,
				Download: u.Download,
			})
		}
	}
	return
}

func (l *ServerDataLogic) trafficCount(ctx context.Context) (totalUploadToday, totalDownloadToday, totalDownloadMonthly, totalUploadMonthly int64) {
	now := timeutil.Now()
	today, err := l.deps.Store.TrafficLog().QueryTrafficByDay(ctx, now)
	if err != nil {
		logger.Error("[ServerDataLogic] Query traffic by day failed", logger.Field("error", err.Error()))
	} else {
		totalUploadToday = today.Upload
		totalDownloadToday = today.Download
	}

	monthly, err := l.deps.Store.TrafficLog().QueryTrafficByMonthly(ctx, now)
	if err != nil {
		logger.Error("[ServerDataLogic] Query traffic by monthly failed", logger.Field("error", err.Error()))
	} else {
		totalUploadMonthly = monthly.Upload
		totalDownloadMonthly = monthly.Download
	}
	return
}
