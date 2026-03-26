package forwarder

import (
	"fmt"
	"hash/fnv"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowgate/flowgate/internal/common"
)

// Balancer selects a target from a set of RouteTargets.
type Balancer interface {
	// Pick selects a target. clientAddr is used by ip_hash; may be nil.
	Pick(clientAddr net.Addr) common.RouteTarget
	// OnConnect tracks a new active connection to the target.
	OnConnect(targetKey string)
	// OnDisconnect tracks a closed connection to the target.
	OnDisconnect(targetKey string)
	// OnDialError records a dial failure for the target.
	OnDialError(targetKey string)
	// OnDialSuccess records a successful dial with measured latency.
	OnDialSuccess(targetKey string, latency time.Duration)
}

// TargetKey returns a canonical "host:port" key for a RouteTarget.
func TargetKey(t common.RouteTarget) string {
	return fmt.Sprintf("%s:%d", t.Host, t.Port)
}

// NewBalancer creates a Balancer for the given strategy and targets.
func NewBalancer(strategy string, targets []common.RouteTarget) Balancer {
	if len(targets) == 0 {
		return nil
	}
	if len(targets) == 1 {
		return &singleTarget{target: targets[0]}
	}

	switch common.NormalizedLoadBalanceStrategy(strategy) {
	case common.LBStrategyRoundRobin:
		return &roundRobin{targets: targets}
	case common.LBStrategyWeightedRoundRobin:
		return newWeightedRR(targets)
	case common.LBStrategyLeastConnections:
		return newLeastConns(targets)
	case common.LBStrategyLeastLatency:
		return newLeastLatency(targets)
	case common.LBStrategyIPHash:
		return &ipHash{targets: targets}
	case common.LBStrategyFailover:
		return newFailover(targets)
	default: // "none" or unrecognised → round-robin
		return &roundRobin{targets: targets}
	}
}

// ---------------------------------------------------------------------------
// singleTarget — fast path for exactly one target
// ---------------------------------------------------------------------------

type singleTarget struct {
	target common.RouteTarget
}

func (s *singleTarget) Pick(net.Addr) common.RouteTarget              { return s.target }
func (s *singleTarget) OnConnect(string)                               {}
func (s *singleTarget) OnDisconnect(string)                            {}
func (s *singleTarget) OnDialError(string)                             {}
func (s *singleTarget) OnDialSuccess(string, time.Duration)            {}

// ---------------------------------------------------------------------------
// roundRobin
// ---------------------------------------------------------------------------

type roundRobin struct {
	targets []common.RouteTarget
	idx     uint64
}

func (rr *roundRobin) Pick(net.Addr) common.RouteTarget {
	n := atomic.AddUint64(&rr.idx, 1)
	return rr.targets[(n-1)%uint64(len(rr.targets))]
}

func (rr *roundRobin) OnConnect(string)                    {}
func (rr *roundRobin) OnDisconnect(string)                 {}
func (rr *roundRobin) OnDialError(string)                  {}
func (rr *roundRobin) OnDialSuccess(string, time.Duration) {}

// ---------------------------------------------------------------------------
// weightedRR — smooth weighted round-robin (Nginx algorithm)
// ---------------------------------------------------------------------------

type weightedRR struct {
	mu      sync.Mutex
	targets []common.RouteTarget
	weights []int
	cw      []int // current weights
	total   int
}

func newWeightedRR(targets []common.RouteTarget) *weightedRR {
	w := &weightedRR{
		targets: targets,
		weights: make([]int, len(targets)),
		cw:      make([]int, len(targets)),
	}
	for i, t := range targets {
		wt := t.Weight
		if wt <= 0 {
			wt = 1
		}
		w.weights[i] = wt
		w.total += wt
	}
	return w
}

func (w *weightedRR) Pick(net.Addr) common.RouteTarget {
	w.mu.Lock()
	defer w.mu.Unlock()

	best := 0
	for i := range w.targets {
		w.cw[i] += w.weights[i]
		if w.cw[i] > w.cw[best] {
			best = i
		}
	}
	w.cw[best] -= w.total
	return w.targets[best]
}

func (w *weightedRR) OnConnect(string)                    {}
func (w *weightedRR) OnDisconnect(string)                 {}
func (w *weightedRR) OnDialError(string)                  {}
func (w *weightedRR) OnDialSuccess(string, time.Duration) {}

// ---------------------------------------------------------------------------
// leastConns — picks the target with fewest active connections
// ---------------------------------------------------------------------------

type leastConns struct {
	mu      sync.Mutex
	targets []common.RouteTarget
	conns   map[string]int64
}

func newLeastConns(targets []common.RouteTarget) *leastConns {
	m := make(map[string]int64, len(targets))
	for _, t := range targets {
		m[TargetKey(t)] = 0
	}
	return &leastConns{targets: targets, conns: m}
}

func (lc *leastConns) Pick(net.Addr) common.RouteTarget {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	best := 0
	bestC := lc.conns[TargetKey(lc.targets[0])]
	for i := 1; i < len(lc.targets); i++ {
		c := lc.conns[TargetKey(lc.targets[i])]
		if c < bestC {
			best = i
			bestC = c
		}
	}
	return lc.targets[best]
}

func (lc *leastConns) OnConnect(key string) {
	lc.mu.Lock()
	lc.conns[key]++
	lc.mu.Unlock()
}

func (lc *leastConns) OnDisconnect(key string) {
	lc.mu.Lock()
	if lc.conns[key] > 0 {
		lc.conns[key]--
	}
	lc.mu.Unlock()
}

func (lc *leastConns) OnDialError(string)                  {}
func (lc *leastConns) OnDialSuccess(string, time.Duration) {}

// ---------------------------------------------------------------------------
// leastLatency — picks the target with lowest EWMA dial latency
// ---------------------------------------------------------------------------

type leastLatency struct {
	mu      sync.Mutex
	targets []common.RouteTarget
	latency map[string]float64 // EWMA in ms; 0 = unknown (treated as fastest)
}

func newLeastLatency(targets []common.RouteTarget) *leastLatency {
	m := make(map[string]float64, len(targets))
	for _, t := range targets {
		m[TargetKey(t)] = 0
	}
	return &leastLatency{targets: targets, latency: m}
}

func (ll *leastLatency) Pick(net.Addr) common.RouteTarget {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	best := 0
	bestL := ll.latency[TargetKey(ll.targets[0])]
	for i := 1; i < len(ll.targets); i++ {
		l := ll.latency[TargetKey(ll.targets[i])]
		if l < bestL {
			best = i
			bestL = l
		}
	}
	return ll.targets[best]
}

func (ll *leastLatency) OnConnect(string)    {}
func (ll *leastLatency) OnDisconnect(string) {}

func (ll *leastLatency) OnDialError(key string) {
	ll.mu.Lock()
	ll.latency[key] = 999999 // penalise failed target
	ll.mu.Unlock()
}

func (ll *leastLatency) OnDialSuccess(key string, d time.Duration) {
	ll.mu.Lock()
	ms := float64(d.Microseconds()) / 1000.0
	old := ll.latency[key]
	if old == 0 {
		ll.latency[key] = ms
	} else {
		ll.latency[key] = old*0.7 + ms*0.3 // EWMA alpha=0.3
	}
	ll.mu.Unlock()
}

// ---------------------------------------------------------------------------
// ipHash — consistent target selection based on client IP
// ---------------------------------------------------------------------------

type ipHash struct {
	targets []common.RouteTarget
}

func (ih *ipHash) Pick(clientAddr net.Addr) common.RouteTarget {
	ip := ""
	if clientAddr != nil {
		ip = clientAddr.String()
		if h, _, err := net.SplitHostPort(ip); err == nil {
			ip = h
		}
	}
	h := fnv.New32a()
	h.Write([]byte(ip))
	return ih.targets[h.Sum32()%uint32(len(ih.targets))]
}

func (ih *ipHash) OnConnect(string)                    {}
func (ih *ipHash) OnDisconnect(string)                 {}
func (ih *ipHash) OnDialError(string)                  {}
func (ih *ipHash) OnDialSuccess(string, time.Duration) {}

// ---------------------------------------------------------------------------
// failover — ordered priority list with 30 s cooldown on failure
// ---------------------------------------------------------------------------

type failoverBalancer struct {
	mu       sync.Mutex
	targets  []common.RouteTarget
	failed   map[string]time.Time
	cooldown time.Duration
}

func newFailover(targets []common.RouteTarget) *failoverBalancer {
	return &failoverBalancer{
		targets:  targets,
		failed:   make(map[string]time.Time),
		cooldown: 30 * time.Second,
	}
}

func (fo *failoverBalancer) Pick(net.Addr) common.RouteTarget {
	fo.mu.Lock()
	defer fo.mu.Unlock()

	now := time.Now()
	for _, t := range fo.targets {
		key := TargetKey(t)
		if failTime, ok := fo.failed[key]; ok {
			if now.Sub(failTime) < fo.cooldown {
				continue
			}
			delete(fo.failed, key) // cooldown expired, retry
		}
		return t
	}
	return fo.targets[0] // all in cooldown — fall back to primary
}

func (fo *failoverBalancer) OnConnect(string)    {}
func (fo *failoverBalancer) OnDisconnect(string) {}

func (fo *failoverBalancer) OnDialError(key string) {
	fo.mu.Lock()
	fo.failed[key] = time.Now()
	fo.mu.Unlock()
}

func (fo *failoverBalancer) OnDialSuccess(key string, _ time.Duration) {
	fo.mu.Lock()
	delete(fo.failed, key)
	fo.mu.Unlock()
}
