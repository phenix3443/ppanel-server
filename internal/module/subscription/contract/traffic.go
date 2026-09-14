package dto

// TrafficWindow names the preset ranges the user panel offers. The chart is
// bucketed hourly for the short window and daily for the longer ones, so a
// 30-day view stays at 30 points instead of 720.
//
// How far back data actually exists is capped by the log retention setting
// (Log.ClearDays, 7 days by default): a longer window can legitimately come
// back with fewer buckets than it asked for.
const (
	TrafficWindow24h = "24h"
	TrafficWindow7d  = "7d"
	TrafficWindow30d = "30d"
)

type GetSubscribeTrafficOverviewRequest struct {
	UserSubscribeId int64  `form:"user_subscribe_id" validate:"required"`
	Window          string `form:"window" validate:"omitempty,oneof=24h 7d 30d"`
}

type GetSubscribeTrafficOverviewResponse struct {
	// Window echoes the resolved window, Start/End the range actually queried
	// (unix milliseconds), so the client never has to recompute them.
	Window   string `json:"window"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	// Interval is "hour" or "day" and tells the client how to label buckets.
	Interval string                 `json:"interval"`
	Series   []TrafficSeriesPoint   `json:"series"`
	Nodes    []TrafficNodeUsageItem `json:"nodes"`
}

type TrafficSeriesPoint struct {
	// Timestamp is the bucket start in unix milliseconds.
	Timestamp int64 `json:"timestamp"`
	Upload    int64 `json:"upload"`
	Download  int64 `json:"download"`
}

type TrafficNodeUsageItem struct {
	ServerId int64  `json:"server_id"`
	Name     string `json:"name"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
	Total    int64  `json:"total"`
}

type GetSubscribeTrafficDetailsRequest struct {
	UserSubscribeId int64 `form:"user_subscribe_id" validate:"required"`
	// Start/End are unix milliseconds. Both empty means the last 24 hours.
	Start int64 `form:"start"`
	End   int64 `form:"end"`
	Page  int   `form:"page"`
	Size  int   `form:"size"`
}

type GetSubscribeTrafficDetailsResponse struct {
	Total int64               `json:"total"`
	List  []TrafficDetailItem `json:"list"`
}

type TrafficDetailItem struct {
	// Timestamp is the minute bucket the node reported, in unix milliseconds.
	Timestamp int64  `json:"timestamp"`
	ServerId  int64  `json:"server_id"`
	Name      string `json:"name"`
	Upload    int64  `json:"upload"`
	Download  int64  `json:"download"`
}
