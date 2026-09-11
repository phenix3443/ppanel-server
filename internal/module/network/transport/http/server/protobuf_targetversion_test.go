package server

import (
	"testing"

	serverv1 "github.com/perfect-panel/server/api/server/v1"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
)

// 节点拉配置时【无条件】发 Accept: application/protobuf
// （ppanel-node/api/panel/server.go 的 setProtobufResponseAccept），
// 所以真实部署里走的永远是 protobuf 分支。只往 JSON 的 DTO 加字段，
// 节点一个字都读不到——控制台下发成功、库里存了、前端提示「已下发」，
// 而节点什么都不做，且没有任何日志能看出为什么。
//
// 这个测试锁住：期望版本必须能过 protobuf 这一层。
func TestQueryServerProtocolConfigCarriesTargetVersionOverProtobuf(t *testing.T) {
	resp, err := queryServerProtocolConfigResponseToProtobuf(&dto.QueryServerConfigResponse{
		PullInterval:  60,
		TargetVersion: "v1.1.14",
	})
	if err != nil {
		t.Fatalf("转换报错: %v", err)
	}
	if got := resp.GetData().GetTargetVersion(); got != "v1.1.14" {
		t.Fatalf("protobuf 里的 target_version = %q, want v1.1.14", got)
	}
}

// 节点走 protobuf 上报状态（UseProtobuf 由服务端响应的 content-type 决定，
// 而 /v2/server 返回 protobuf）。给 proto 和 DTO 都加了字段还不够——
// 中间这个绑定函数不拷贝的话，版本信息在这里被静默丢弃，面板永远显示「未上报」。
// 这是本次实测才暴露的：节点确实装上了 v1.1.15，面板却还是 version=None。
func TestBindServerStatusCarriesVersionFromProtobuf(t *testing.T) {
	var req dto.ServerPushStatusRequest
	msg := &serverv1.PushServerStatusRequest{
		Cpu: 1, Mem: 2, Disk: 3, UpdatedAt: 4,
		Version: "v1.1.15", LatestVersion: "v1.1.16",
	}
	copyStatusFromProtobuf(msg, &req)
	if req.Version != "v1.1.15" {
		t.Errorf("Version = %q, want v1.1.15", req.Version)
	}
	if req.LatestVersion != "v1.1.16" {
		t.Errorf("LatestVersion = %q, want v1.1.16", req.LatestVersion)
	}
	if req.Cpu != 1 || req.Mem != 2 || req.Disk != 3 || req.UpdatedAt != 4 {
		t.Errorf("原有字段被改坏了: %+v", req)
	}
}
