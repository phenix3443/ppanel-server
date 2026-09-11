package server

import (
	"testing"

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
