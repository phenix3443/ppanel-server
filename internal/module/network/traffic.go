package network

import "github.com/perfect-panel/server/internal/module/network/internal/trafficagg"

// TrafficAggregator is the network-owned pipeline shared by node reports and
// scheduled flush tasks. Queue adapters enter through this facade.
type TrafficAggregator = trafficagg.Aggregator
type TrafficAggregatorDeps = trafficagg.Deps
type UserTraffic = trafficagg.UserTraffic

func NewTrafficAggregator(deps TrafficAggregatorDeps) *TrafficAggregator {
	return trafficagg.New(deps)
}
