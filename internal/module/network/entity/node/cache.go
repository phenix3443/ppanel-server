package node

import (
	"encoding/json"
	"time"
)

type (
	Status struct {
		Cpu       float64 `json:"cpu"`
		Mem       float64 `json:"mem"`
		Disk      float64 `json:"disk"`
		UpdatedAt int64   `json:"updated_at"`
		// Version 是节点正在跑的版本，LatestVersion 是节点自己查到的上游最新版。
		// 由节点查而不是面板查，面板就不必访问外网。旧版本节点不会上报这两项，
		// 此时为空——消费方必须容忍空值，不能据此判断「没有升级」。
		Version       string `json:"version,omitempty"`
		LatestVersion string `json:"latest_version,omitempty"`
	}

	OnlineUserSubscribe map[int64][]string
)

// Marshal  to json string
func (s *Status) Marshal() string {
	type Alias Status
	data, _ := json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	})
	return string(data)
}

// Unmarshal from json string
func (s *Status) Unmarshal(data string) error {
	type Alias Status
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(s),
	}
	return json.Unmarshal([]byte(data), &aux)
}

const (
	Expiry                                = 300 * time.Second              // Cache expiry time in seconds
	StatusCacheKey                        = "node:status:%d"               // Node status cache key format (Server ID and protocol) Example: node:status:1:shadowsocks
	OnlineUserCacheKeyWithSubscribe       = "node:online:subscribe:%d:%s"  // Online user subscribe cache key format (Server ID and protocol) Example: node:online:subscribe:1:shadowsocks
	OnlineUserSubscribeCacheKeyWithGlobal = "node:online:subscribe:global" // Online user global subscribe cache key
)
