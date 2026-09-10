package publicinfo

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetStatLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

var (
	statHTTPClient = &http.Client{Timeout: 8 * time.Second}
	statRefreshMu  sync.Mutex
)

func (l *GetStatLogic) cachedStat() *dto.GetStatResponse {
	respJSON, err := l.deps.Redis.Get(l.ctx, config.CommonStatCacheKey).Result()
	if err != nil {
		return nil
	}
	var cached dto.GetStatResponse
	if json.Unmarshal([]byte(respJSON), &cached) != nil {
		return nil
	}
	return &cached
}

// Get Tos
func newGetStatLogic(ctx context.Context, deps Deps) *GetStatLogic {
	return &GetStatLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetStatLogic) GetStat() (resp *dto.GetStatResponse, err error) {
	if cached := l.cachedStat(); cached != nil {
		return cached, nil
	}
	// Collapse concurrent hourly cache misses inside one process. The second
	// read prevents queued requests from repeating DNS and geolocation work.
	statRefreshMu.Lock()
	defer statRefreshMu.Unlock()
	if cached := l.cachedStat(); cached != nil {
		return cached, nil
	}
	userStore := l.deps.Store.User()
	nodeStore := l.deps.Store.Node()
	u, err := userStore.CountEnabledUsers(l.ctx)
	if err != nil {
		l.Logger.Error("[GetStatLogic] get user count failed: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get user count failed: %v", err.Error())
	}
	if u > 100 {
		u -= u % 100
	} else if u > 10 {
		u -= u % 10
	} else {
		u = 1
	}
	n, err := nodeStore.CountEnabledNodes(l.ctx)
	if err != nil {
		l.Logger.Error("[GetStatLogic] get server count failed: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get server count failed: %v", err.Error())
	}
	nodeaddr, err := nodeStore.QueryServerAddresses(l.ctx)
	if err != nil {
		l.Logger.Error("[GetStatLogic] get server_addr failed: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get server_addr failed: %v", err.Error())
	}
	type apireq struct {
		Query  string `json:"query"`
		Fields string `json:"fields"`
	}
	type apiret struct {
		CountryCode string `json:"countryCode"`
	}
	//map as dict
	type void struct{}
	var v void
	country := make(map[string]void)
	for c := range slices.Chunk(nodeaddr, 100) {
		resolved := make([]string, len(c))
		resolveCtx, cancelResolve := context.WithTimeout(l.ctx, 5*time.Second)
		var resolveWG sync.WaitGroup
		resolveSlots := make(chan struct{}, 8)
		for index, addr := range c {
			if parsed := net.ParseIP(addr); parsed != nil {
				resolved[index] = parsed.String()
				continue
			}
			resolveWG.Add(1)
			go func(index int, host string) {
				defer resolveWG.Done()
				select {
				case resolveSlots <- struct{}{}:
					defer func() { <-resolveSlots }()
				case <-resolveCtx.Done():
					return
				}
				addresses, lookupErr := net.DefaultResolver.LookupIPAddr(resolveCtx, host)
				if lookupErr == nil && len(addresses) > 0 {
					resolved[index] = addresses[0].IP.String()
				}
			}(index, addr)
		}
		resolveWG.Wait()
		cancelResolve()
		var batchreq []apireq
		for _, addr := range resolved {
			if addr != "" {
				batchreq = append(batchreq, apireq{Query: addr, Fields: "countryCode"})
			}
		}
		if len(batchreq) == 0 {
			continue
		}
		reqBody, _ := json.Marshal(batchreq)
		requestCtx, cancel := context.WithTimeout(l.ctx, 8*time.Second)
		httpReq, requestErr := http.NewRequestWithContext(requestCtx, http.MethodPost, "http://ip-api.com/batch", bytes.NewReader(reqBody))
		if requestErr == nil {
			httpReq.Header.Set("Content-Type", "application/json")
		}
		var ret *http.Response
		if requestErr == nil {
			ret, requestErr = statHTTPClient.Do(httpReq)
		}
		err := requestErr
		if err == nil {
			retBytes, err := io.ReadAll(io.LimitReader(ret.Body, 1<<20))
			_ = ret.Body.Close()
			if err == nil && ret.StatusCode >= http.StatusOK && ret.StatusCode < http.StatusMultipleChoices {
				var retStruct []apiret
				err := json.Unmarshal(retBytes, &retStruct)
				if err == nil {
					for _, dat := range retStruct {
						if dat.CountryCode != "" {
							country[dat.CountryCode] = v
						}
					}
				}
			}
		}
		cancel()
	}
	protocolDict := make(map[string]void)
	protocol, err := nodeStore.QueryEnabledNodeProtocols(l.ctx)
	if err != nil {
		l.Logger.Error("[GetStatLogic] get protocol failed: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get protocol failed: %v", err.Error())
	}

	for _, p := range protocol {
		if p == "" {
			continue
		}
		protocolDict[p] = v
	}
	protocol = nil
	for p := range protocolDict {
		protocol = append(protocol, p)
	}
	resp = &dto.GetStatResponse{
		User:     u,
		Node:     n,
		Country:  int64(len(country)),
		Protocol: protocol,
	}
	val, _ := json.Marshal(*resp)
	_ = l.deps.Redis.Set(l.ctx, config.CommonStatCacheKey, string(val), time.Duration(3600)*time.Second).Err()
	return resp, nil
}
