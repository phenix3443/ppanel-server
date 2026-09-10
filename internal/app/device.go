package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/transport/devicesocket"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

func NewDeviceManager(srv *Application) *devicesocket.DeviceManager {
	ctx := context.Background()
	manager := devicesocket.NewDeviceManager(30, 30)

	//设备离线处理
	manager.OnDeviceOffline = func(userID int64, deviceID, session string, createAt time.Time) {
		oneDevice, err := srv.Store.UserDevice().FindOneDeviceByIdentifier(ctx, deviceID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				logger.Errorw("failed to find device", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
			}
			return
		}

		//更新设备状态为离线
		err = srv.Store.UserDevice().SetDeviceOnline(ctx, oneDevice.Id, false)
		if err != nil {
			logger.Errorw("[DeviceManager] failed to update device", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
		}

		//当前时间为设备离线时间
		currentTime := timeutil.Now()
		endTime := currentTime.Format("2006-01-02 00:00:00")
		parseStart, _ := time.Parse(time.DateTime, endTime)
		startTime := parseStart.Add(time.Hour * 24).Format(time.DateTime)
		deviceOnlineRecord := user.DeviceOnlineRecord{
			UserId:        userID,
			Identifier:    deviceID,
			OnlineTime:    createAt,
			OfflineTime:   currentTime,
			OnlineSeconds: int64(currentTime.Sub(createAt).Seconds()),
		}

		//获取设备昨日在线记录
		onlineRecord, err := srv.Store.UserDevice().FindDeviceOnlineRecord(ctx, userID, startTime, endTime)
		if err != nil {
			//昨日未在线，连续在线天数为1
			deviceOnlineRecord.DurationDays = 1
		} else {
			//昨日在线，连续在线天数为，昨天连续在线天数+1，等于当前连续在线天数
			deviceOnlineRecord.DurationDays = onlineRecord.DurationDays + 1
		}

		if err := srv.Store.UserDevice().InsertDeviceOnlineRecord(ctx, &deviceOnlineRecord); err != nil {
			logger.Errorw("[DeviceOnlineRecord] failed to DeviceOnlineRecord", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
		}
	}

	//设备上线处理
	manager.OnDeviceOnline = func(userID int64, deviceID, session string) {
		oneDevice, err := srv.Store.UserDevice().FindOneDeviceByIdentifier(ctx, deviceID)
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				logger.Errorw("failed to find device", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
			}
			return
		}
		err = srv.Store.UserDevice().SetDeviceOnline(ctx, oneDevice.Id, true)
		if err != nil {
			logger.Errorw("[DeviceManager] failed to update device", logger.Field("error", err.Error()), logger.Field("device_id", deviceID))
			return
		}
	}

	manager.OnDeviceKicked = func(userID int64, deviceID, session string, operator devicesocket.Operator) {
		//管理员踢下线
		if operator == devicesocket.Admin {
			message := DeviceMessage{Method: DeviceKickedAdmin}
			_ = manager.SendToDevice(userID, deviceID, message.Json())
			//将登陆凭证从缓存中删除
			srv.Redis.Del(ctx, fmt.Sprintf("%v:%v", config.SessionIdKey, session))
			return
		}

		//登陆设备超过限制踢下线
		if operator == devicesocket.MaxDevices {
			message := DeviceMessage{Method: DeviceKickedMax}
			_ = manager.SendToDevice(userID, deviceID, message.Json())
			//将登陆凭证从缓存中删除
			srv.Redis.Del(ctx, fmt.Sprintf("%v:%v", config.SessionIdKey, session))
			return
		}

	}

	manager.OnMessage = func(userID int64, deviceID, session string, message string) {
		logger.Infof("userid: %d ,device_number: %s,session: %s, message: %v", userID, deviceID, session, message)
	}
	return manager
}

type DeviceMessage struct {
	Method DeviceMessageMethod `json:"method"`
}

func (dm *DeviceMessage) Json() string {
	jsonData, _ := json.Marshal(dm)
	return string(jsonData)
}

type DeviceMessageMethod string

const (
	// DeviceKickedMax 设备数量超出限制
	DeviceKickedMax DeviceMessageMethod = "kicked_device"
	// DeviceKickedAdmin 管理员踢下线
	DeviceKickedAdmin DeviceMessageMethod = "kicked_admin"
	// SubscribeUpdate 订阅有更新
	SubscribeUpdate DeviceMessageMethod = "subscribe_update"
)
