package nodeversion

import (
	"errors"
	"sync"
	"testing"
	"time"
)

// 节点列表页每次刷新都会读这个值，不能每次都打 GitHub——未认证 API 是
// 60 次/小时，管理员多点几下就 403 了。
func TestCacheHitsUpstreamOncePerTTL(t *testing.T) {
	var calls int
	var mu sync.Mutex
	c := &Cache{TTL: time.Hour, fetch: func() (string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return "v1.1.14", nil
	}}
	for i := 0; i < 10; i++ {
		if got := c.Latest(); got != "v1.1.14" {
			t.Fatalf("第 %d 次得到 %q", i, got)
		}
	}
	if calls != 1 {
		t.Fatalf("打了上游 %d 次，TTL 内应当只打 1 次", calls)
	}
}

// 查不到时返回空串，而不是报错或阻塞——版本信息是锦上添花，
// 不能让节点列表页因为 GitHub 不可达而打不开。
func TestCacheReturnsEmptyWhenUpstreamUnavailable(t *testing.T) {
	c := &Cache{TTL: time.Hour, fetch: func() (string, error) {
		return "", errors.New("network down")
	}}
	if got := c.Latest(); got != "" {
		t.Fatalf("应返回空串，得到 %q", got)
	}
}

// 上游短暂不可用时沿用上次的好值，比突然显示「未知」有用。
func TestCacheKeepsLastGoodValue(t *testing.T) {
	var fail bool
	c := &Cache{TTL: time.Nanosecond, fetch: func() (string, error) {
		if fail {
			return "", errors.New("boom")
		}
		return "v1.1.14", nil
	}}
	c.Latest()
	fail = true
	time.Sleep(2 * time.Millisecond)
	if got := c.Latest(); got != "v1.1.14" {
		t.Fatalf("应沿用上次的好值，得到 %q", got)
	}
}

// 失败后不能每次调用都重打上游——那样 GitHub 一挂，面板每次刷新都多等一个超时。
func TestCacheBacksOffAfterFailure(t *testing.T) {
	var calls int
	c := &Cache{TTL: time.Hour, FailureBackoff: time.Hour, fetch: func() (string, error) {
		calls++
		return "", errors.New("down")
	}}
	c.Latest()
	c.Latest()
	c.Latest()
	if calls != 1 {
		t.Fatalf("失败后仍重打了 %d 次，应当退避", calls)
	}
}
