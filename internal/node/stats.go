package node

import (
	"sync"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/node/forwarder"
)

// TrafficCollector aggregates traffic stats from all forwarders
type TrafficCollector struct {
	mu          sync.RWMutex
	tcpFwds     map[int64]*forwarder.TCPForwarder
	udpFwds     map[int64]*forwarder.UDPForwarder
	hopFwds     map[int64]*forwarder.HopChainForwarder
	sniRoutes   map[int64]*forwarder.SNIRoute // rule_id -> SNIRoute
}

// NewTrafficCollector creates a new traffic collector
func NewTrafficCollector() *TrafficCollector {
	return &TrafficCollector{
		tcpFwds:   make(map[int64]*forwarder.TCPForwarder),
		udpFwds:   make(map[int64]*forwarder.UDPForwarder),
		hopFwds:   make(map[int64]*forwarder.HopChainForwarder),
		sniRoutes: make(map[int64]*forwarder.SNIRoute),
	}
}

// RegisterTCP registers a TCP forwarder for traffic collection
func (tc *TrafficCollector) RegisterTCP(id int64, fwd *forwarder.TCPForwarder) {
	tc.mu.Lock()
	tc.tcpFwds[id] = fwd
	tc.mu.Unlock()
}

// RegisterUDP registers a UDP forwarder for traffic collection
func (tc *TrafficCollector) RegisterUDP(id int64, fwd *forwarder.UDPForwarder) {
	tc.mu.Lock()
	tc.udpFwds[id] = fwd
	tc.mu.Unlock()
}

// UnregisterTCP removes a TCP forwarder from collection
func (tc *TrafficCollector) UnregisterTCP(id int64) {
	tc.mu.Lock()
	delete(tc.tcpFwds, id)
	tc.mu.Unlock()
}

// UnregisterUDP removes a UDP forwarder from collection
func (tc *TrafficCollector) UnregisterUDP(id int64) {
	tc.mu.Lock()
	delete(tc.udpFwds, id)
	tc.mu.Unlock()
}

// RegisterHopChain registers a hop-chain forwarder for traffic collection
func (tc *TrafficCollector) RegisterHopChain(id int64, fwd *forwarder.HopChainForwarder) {
	tc.mu.Lock()
	tc.hopFwds[id] = fwd
	tc.mu.Unlock()
}

// UnregisterHopChain removes a hop-chain forwarder from collection
func (tc *TrafficCollector) UnregisterHopChain(id int64) {
	tc.mu.Lock()
	delete(tc.hopFwds, id)
	tc.mu.Unlock()
}

// RegisterSNIRoute registers an SNI route for traffic collection
func (tc *TrafficCollector) RegisterSNIRoute(ruleID int64, route *forwarder.SNIRoute) {
	tc.mu.Lock()
	tc.sniRoutes[ruleID] = route
	tc.mu.Unlock()
}

// UnregisterSNIRoute removes an SNI route from collection
func (tc *TrafficCollector) UnregisterSNIRoute(ruleID int64) {
	tc.mu.Lock()
	delete(tc.sniRoutes, ruleID)
	tc.mu.Unlock()
}

// Collect gathers traffic reports from all forwarders
func (tc *TrafficCollector) Collect() []common.TrafficReport {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	var reports []common.TrafficReport

	for id, fwd := range tc.tcpFwds {
		in, out := fwd.GetAndResetTraffic()
		if in > 0 || out > 0 {
			reports = append(reports, common.TrafficReport{
				RuleID:     id,
				TrafficIn:  in,
				TrafficOut: out,
			})
		}
	}

	for id, fwd := range tc.udpFwds {
		in, out := fwd.GetAndResetTraffic()
		if in > 0 || out > 0 {
			// Check if there's already a report for this rule (tcp+udp)
			found := false
			for i, r := range reports {
				if r.RuleID == id {
					reports[i].TrafficIn += in
					reports[i].TrafficOut += out
					found = true
					break
				}
			}
			if !found {
				reports = append(reports, common.TrafficReport{
					RuleID:     id,
					TrafficIn:  in,
					TrafficOut: out,
				})
			}
		}
	}

	for id, fwd := range tc.hopFwds {
		in, out := fwd.GetAndResetTraffic()
		if in > 0 || out > 0 {
			reports = append(reports, common.TrafficReport{
				RuleID:     id,
				TrafficIn:  in,
				TrafficOut: out,
			})
		}
	}

	for id, route := range tc.sniRoutes {
		in, out := route.GetAndResetTraffic()
		if in > 0 || out > 0 {
			reports = append(reports, common.TrafficReport{
				RuleID:     id,
				TrafficIn:  in,
				TrafficOut: out,
			})
		}
	}

	return reports
}

// GetTotalConnections returns total active connections
func (tc *TrafficCollector) GetTotalConnections() int {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	total := 0
	for _, fwd := range tc.tcpFwds {
		total += fwd.GetConnections()
	}
	for _, fwd := range tc.hopFwds {
		total += fwd.GetConnections()
	}
	for _, route := range tc.sniRoutes {
		total += route.GetConnections()
	}
	return total
}
