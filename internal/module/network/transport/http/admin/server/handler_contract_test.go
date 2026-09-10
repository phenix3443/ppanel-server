package server

import (
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/module/network"
)

func TestHandlerFactories_return_native_hertz_handlers(t *testing.T) {
	var _ func(network.Service) app.HandlerFunc = CreateNodeHandler
	var _ func(network.Service) app.HandlerFunc = CreateServerHandler
	var _ func(network.Service) app.HandlerFunc = DeleteNodeHandler
	var _ func(network.Service) app.HandlerFunc = DeleteServerHandler
	var _ func(network.Service) app.HandlerFunc = FilterNodeListHandler
	var _ func(network.Service) app.HandlerFunc = FilterServerListHandler
	var _ func(network.Service) app.HandlerFunc = GetServerNodeConfigHandler
	var _ func(network.Service) app.HandlerFunc = GetServerProtocolsHandler
	var _ func(network.Service) app.HandlerFunc = QueryNodeTagHandler
	var _ func(network.Service) app.HandlerFunc = ResetSortWithNodeHandler
	var _ func(network.Service) app.HandlerFunc = ResetSortWithServerHandler
	var _ func(network.Service) app.HandlerFunc = ToggleNodeStatusHandler
	var _ func(network.Service) app.HandlerFunc = UpdateNodeHandler
	var _ func(network.Service) app.HandlerFunc = UpdateServerHandler
	var _ func(network.Service) app.HandlerFunc = UpdateServerNodeConfigHandler
}
