package dto

type ServerPushUserTrafficRequest struct {
	ServerCommon
	Traffic []UserTraffic `json:"traffic"`
}

type NetworkServerTotalDataSnapshot struct {
	OnlineUsers                   int64                          `json:"online_users"`
	OnlineServers                 int64                          `json:"online_servers"`
	OfflineServers                int64                          `json:"offline_servers"`
	TodayUpload                   int64                          `json:"today_upload"`
	TodayDownload                 int64                          `json:"today_download"`
	MonthlyUpload                 int64                          `json:"monthly_upload"`
	MonthlyDownload               int64                          `json:"monthly_download"`
	UpdatedAt                     int64                          `json:"updated_at"`
	ServerTrafficRankingToday     []NetworkServerTrafficSnapshot `json:"server_traffic_ranking_today"`
	ServerTrafficRankingYesterday []NetworkServerTrafficSnapshot `json:"server_traffic_ranking_yesterday"`
	UserTrafficRankingToday       []NetworkUserTrafficSnapshot   `json:"user_traffic_ranking_today"`
	UserTrafficRankingYesterday   []NetworkUserTrafficSnapshot   `json:"user_traffic_ranking_yesterday"`
}

type NetworkServerTrafficSnapshot struct {
	ServerId int64  `json:"server_id"`
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}

type UserTraffic struct {
	SID      int64 `json:"uid"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}

type NetworkUserTrafficSnapshot struct {
	// SID identifies the user_subscribe row the traffic was billed to, UID the
	// user owning it. UID is carried separately so the console can still name
	// the user after the subscription row is gone.
	SID      int64 `json:"sid"`
	UID      int64 `json:"uid"`
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
}
