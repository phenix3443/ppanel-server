// Package nodeversion 缓存 ppanel-node 的上游最新版本号。
//
// 【为什么由面板查而不是节点查】节点数量会增长，而 GitHub 未认证 API 是
// 60 次/小时；让每个节点自己查，装到十几台就会被限流。面板查一次、所有节点
// 共用，同时节点也不必具备访问 GitHub 的能力。
package nodeversion

import (
	"encoding/json"
	"net/http"
	"os"
	"sync"
	"time"
)

// DefaultRepo 是我们自己的 fork。上游仓库的 release 不含我们的修复。
const DefaultRepo = "phenix3443/ppanel-node"

type Cache struct {
	TTL time.Duration
	// FailureBackoff 是失败后的重试间隔。为 0 时用 TTL。
	// 没有它的话，GitHub 一挂，每次刷新节点列表都要多等一个 HTTP 超时。
	FailureBackoff time.Duration

	mu        sync.Mutex
	value     string
	nextFetch time.Time
	fetch     func() (string, error)
}

func New(ttl time.Duration) *Cache {
	return &Cache{TTL: ttl, FailureBackoff: 10 * time.Minute, fetch: fetchLatest}
}

// Latest 返回上游最新版本号；取不到时返回上次的好值，从未成功过则返回空串。
// 不返回 error：调用方是页面渲染路径，版本查不到不该让页面失败。
func (c *Cache) Latest() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.nextFetch) {
		return c.value
	}
	v, err := c.fetch()
	if err != nil {
		backoff := c.FailureBackoff
		if backoff == 0 {
			backoff = c.TTL
		}
		c.nextFetch = time.Now().Add(backoff)
		return c.value // 沿用上次的好值
	}
	c.value = v
	c.nextFetch = time.Now().Add(c.TTL)
	return v
}

func fetchLatest() (string, error) {
	repo := os.Getenv("PPANEL_NODE_REPO")
	if repo == "" {
		repo = DefaultRepo
	}
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET",
		"https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &statusError{resp.Status}
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	return release.TagName, nil
}

type statusError struct{ status string }

func (e *statusError) Error() string { return "github returned " + e.status }
