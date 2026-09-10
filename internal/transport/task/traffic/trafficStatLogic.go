package traffic

import (
	"context"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type StatLogic struct {
	deps Dependencies
}

func NewStatLogic(deps Dependencies) *StatLogic {
	return &StatLogic{
		deps: deps,
	}
}

func (l *StatLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	now := timeutil.Now()

	// 获取全部有效订阅
	// 获取统计时间范围
	start := time.Date(now.Year(), now.Month(), now.Day()-1, 0, 0, 0, 0, timeutil.Location())
	end := start.Add(24 * time.Hour)

	// Historical traffic is read outside the write transaction. Once the two
	// aggregate result sets are ready, all daily log rows are persisted with a
	// batched INSERT instead of one INSERT per user/server.
	userTraffic, err := l.deps.Store.TrafficLog().QueryUserTrafficRanking(ctx, start, end)
	if err != nil {
		logger.Errorf("[Traffic Stat Queue] Query user traffic failed: %v", err.Error())
		return err
	}
	serverTraffic, err := l.deps.Store.TrafficLog().QueryServerTrafficRanking(ctx, start, end)
	if err != nil {
		logger.Errorf("[Traffic Stat Queue] Query server traffic failed: %v", err.Error())
		return err
	}

	date := start.Format(time.DateOnly)
	logs := make([]*log.SystemLog, 0, len(userTraffic)+len(serverTraffic)+3)
	userTop10 := log.UserTrafficRank{Rank: make(map[uint8]log.UserTraffic)}
	stat := log.TrafficStat{}
	for i, trafficData := range userTraffic {
		item := log.UserTraffic{SubscribeId: trafficData.SubscribeId, UserId: trafficData.UserId, Upload: trafficData.Upload, Download: trafficData.Download, Total: trafficData.Total}
		if i < 10 {
			userTop10.Rank[uint8(i+1)] = item
		}
		stat.Upload += item.Upload
		stat.Download += item.Download
		content, _ := item.Marshal()
		logs = append(logs, &log.SystemLog{Type: log.TypeSubscribeTraffic.Uint8(), Date: date, ObjectID: item.SubscribeId, Content: string(content)})
	}
	stat.Total = stat.Upload + stat.Download
	userTop10Content, _ := userTop10.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeUserTrafficRank.Uint8(), Date: date, Content: string(userTop10Content)})

	serverTop10 := log.ServerTrafficRank{Rank: make(map[uint8]log.ServerTraffic)}
	for i, trafficData := range serverTraffic {
		item := log.ServerTraffic{ServerId: trafficData.ServerId, Upload: trafficData.Upload, Download: trafficData.Download, Total: trafficData.Total}
		if i < 10 {
			serverTop10.Rank[uint8(i+1)] = item
		}
		content, _ := item.Marshal()
		logs = append(logs, &log.SystemLog{Type: log.TypeServerTraffic.Uint8(), Date: date, ObjectID: item.ServerId, Content: string(content)})
	}
	serverTop10Content, _ := serverTop10.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeServerTrafficRank.Uint8(), Date: date, Content: string(serverTop10Content)})
	statContent, _ := stat.Marshal()
	logs = append(logs, &log.SystemLog{Type: log.TypeTrafficStat.Uint8(), Date: date, Content: string(statContent)})

	err = l.deps.Store.Log().InsertBatch(ctx, logs, 1000)
	if err != nil {
		logger.Errorf("[Traffic Stat Queue] Process task failed: %v", err.Error())
		return err
	}
	logger.Infof("[Traffic Stat Queue] Process task completed successfully, consuming: %s", time.Since(now).String())
	return nil
}
