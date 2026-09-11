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
	"strconv"
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

	mu         sync.Mutex
	value      string
	nextFetch  time.Time
	refreshing bool
	fetch      func() (string, error)

	listMu         sync.Mutex
	list           []Release
	listNextFetch  time.Time
	listRefreshing bool
	fetchList      func() ([]Release, error)
}

// Release 是版本下拉需要的最小信息。
type Release struct {
	Version     string `json:"version"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
}

func New(ttl time.Duration) *Cache {
	return &Cache{
		TTL:            ttl,
		FailureBackoff: 10 * time.Minute,
		fetch:          fetchLatest,
		fetchList:      fetchReleases,
	}
}

// Default 是进程内共享的那一份。
//
// 【必须只有一份】节点列表渲染、版本下拉、下发时把 "latest" 解析成具体 tag
// ——三条路径读的是同一个上游。各建各的 cache 就是把 GitHub 的 60 次/小时
// 按路径数除一遍，而且三处看到的版本可能还不一致。
var Default = New(time.Hour)

// Latest 返回上游最新版本号，【立即返回，永不阻塞】。
//
// 调用方是节点列表的渲染路径。过期时只在后台起一次刷新，本次仍返回旧值
// （首次调用返回空串）——让管理员多等一个 HTTP 往返、甚至并发全堵在锁上，
// 换一个「版本号新鲜一点」不值得。
//
// 不返回 error：版本查不到不该让页面失败。
func (c *Cache) Latest() string {
	c.mu.Lock()
	defer c.mu.Unlock()

	if time.Now().Before(c.nextFetch) || c.refreshing {
		return c.value
	}
	// 先把下次允许刷新的时间推后，避免刷新失败后每次调用都再起一个 goroutine。
	c.refreshing = true
	go c.refresh()
	return c.value
}

func (c *Cache) refresh() {
	v, err := c.fetch()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshing = false
	if err != nil {
		backoff := c.FailureBackoff
		if backoff == 0 {
			backoff = c.TTL
		}
		c.nextFetch = time.Now().Add(backoff)
		return // 沿用上次的好值
	}
	c.value = v
	c.nextFetch = time.Now().Add(c.TTL)
}

// List 返回可选版本列表，语义和 Latest 一致：立即返回，永不阻塞，
// 过期时后台刷新。列表为空表示还没拉到——前端此时应允许手输版本号。
func (c *Cache) List() []Release {
	c.listMu.Lock()
	defer c.listMu.Unlock()

	if time.Now().Before(c.listNextFetch) || c.listRefreshing {
		return c.list
	}
	c.listRefreshing = true
	go c.refreshList()
	return c.list
}

func (c *Cache) refreshList() {
	v, err := c.fetchList()

	c.listMu.Lock()
	defer c.listMu.Unlock()
	c.listRefreshing = false
	if err != nil {
		backoff := c.FailureBackoff
		if backoff == 0 {
			backoff = c.TTL
		}
		c.listNextFetch = time.Now().Add(backoff)
		return
	}
	c.list = v
	c.listNextFetch = time.Now().Add(c.TTL)
}

// releasesPerPage 取 30：够覆盖最近一年的发版，又不至于让下拉长到没法用。
const releasesPerPage = 30

func fetchReleases() ([]Release, error) {
	resp, err := githubGet("/releases?per_page=" + strconv.Itoa(releasesPerPage))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var releases []struct {
		TagName     string `json:"tag_name"`
		Prerelease  bool   `json:"prerelease"`
		Draft       bool   `json:"draft"`
		PublishedAt string `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, err
	}
	out := make([]Release, 0, len(releases))
	for _, r := range releases {
		// 草稿没有对外的下载地址，节点拉不到它的二进制。
		if r.Draft {
			continue
		}
		out = append(out, Release{Version: r.TagName, Prerelease: r.Prerelease, PublishedAt: r.PublishedAt})
	}
	return out, nil
}

// Repo 返回当前配置的 ppanel-node 仓库。
func Repo() string {
	if repo := os.Getenv("PPANEL_NODE_REPO"); repo != "" {
		return repo
	}
	return DefaultRepo
}

func githubGet(path string) (*http.Response, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", "https://api.github.com/repos/"+Repo()+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, &statusError{resp.Status}
	}
	return resp, nil
}

func fetchLatest() (string, error) {
	resp, err := githubGet("/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
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

// AutoLatest 是控制台里「跟随最新」这一档的存储值。
const AutoLatest = "latest"

// Resolve 把节点的版本策略翻译成真正下发给它的 tag。
//
// 三种取值，没有第四种，也没有继承：
//   - ""         不自动升级，节点保持现状
//   - AutoLatest 跟随最新，这里解析成具体 tag
//   - 具体 tag   钉死在这个版本
//
// 【"latest" 绝不下发给节点】节点收到它就得自己去问 GitHub：升级时机变成
// 「节点哪一刻去拉的」，既不可复现也不可控，而且每个节点都要有访问 GitHub
// 的能力。由面板解析成具体 tag 再下发，节点那边永远只看到确定的版本号。
//
// 还没拉到上游版本时返回空串——空串的语义是「这一轮不干预」，比下发一个
// 猜的版本号安全。
func (c *Cache) Resolve(v string) string {
	if v != AutoLatest {
		return v
	}
	return c.Latest()
}
