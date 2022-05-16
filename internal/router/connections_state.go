package router

import (
	"context"
	"math/rand"

	"github.com/ydb-platform/ydb-go-sdk/v3/internal/balancer"
	"github.com/ydb-platform/ydb-go-sdk/v3/internal/conn"
	"github.com/ydb-platform/ydb-go-sdk/v3/internal/xrand"
)

type connectionsState struct {
	PreferCount int

	connByNodeID map[uint32]conn.Conn
	prefer       []conn.Conn
	fallback     []conn.Conn
	lastAttempt  []conn.Conn

	rand xrand.Rand
}

func newConnectionsState(conns []conn.Conn, preferFunc balancer.PreferConnFunc, allowFallback bool) *connectionsState {
	res := &connectionsState{
		connByNodeID: connsToNodeIDMap(conns),
		rand: xrand.New(
			xrand.WithLock(),
			xrand.WithSource(rand.Int63()),
		),
	}

	res.prefer, res.fallback = sortPreferConnections(conns, preferFunc, allowFallback)
	if allowFallback {
		res.lastAttempt = conns
	} else {
		res.lastAttempt = res.prefer
	}
	res.PreferCount = len(res.prefer)
	return res
}

func (s *connectionsState) preferConnection(ctx context.Context) conn.Conn {
	if e, hasPreferEndpoint := ContextEndpoint(ctx); hasPreferEndpoint {
		c := s.connByNodeID[e.NodeID()]
		if isOkConnection(c, true) {
			return c
		}
	}

	return nil
}

func (s *connectionsState) Connection(ctx context.Context) (_ conn.Conn, failedCount int) {
	if c := s.preferConnection(ctx); c != nil {
		return c, 0
	}

	try := func(conns []conn.Conn) conn.Conn {
		c, tryFailed := s.selectRandomConnection(conns, false)
		failedCount += tryFailed
		return c
	}

	if c := try(s.prefer); c != nil {
		return c, failedCount
	}

	if c := try(s.fallback); c != nil {
		return c, failedCount
	}

	c, _ := s.selectRandomConnection(s.lastAttempt, true)
	return c, failedCount
}

func (s *connectionsState) selectRandomConnection(conns []conn.Conn, allowBanned bool) (c conn.Conn, failedConns int) {
	connCount := len(conns)
	if connCount == 0 {
		// return for empty list need for prevent panic in fast path
		return nil, 0
	}

	// fast path
	if c := conns[s.rand.Int(connCount)]; isOkConnection(c, allowBanned) {
		return c, 0
	}

	// shuffled indexes slices need for guarantee about every connection will check
	indexes := make([]int, connCount)
	for index := range indexes {
		indexes[index] = index
	}
	s.rand.Shuffle(connCount, func(i, j int) {
		indexes[i], indexes[j] = indexes[j], indexes[i]
	})

	for _, index := range indexes {
		c := conns[index]
		if isOkConnection(c, allowBanned) {
			return c, 0
		}
		failedConns++
	}

	return nil, failedConns
}

func connsToNodeIDMap(conns []conn.Conn) (res map[uint32]conn.Conn) {
	for _, c := range conns {
		nodeID := c.Endpoint().NodeID()

		if nodeID == 0 {
			continue
		}
		if res == nil {
			res = make(map[uint32]conn.Conn, len(conns))
		}
		res[nodeID] = c
	}
	return res
}

func sortPreferConnections(conns []conn.Conn, preferFunc balancer.PreferConnFunc, allowFallback bool) (prefer []conn.Conn, fallback []conn.Conn) {
	if preferFunc == nil {
		return conns, nil
	}

	prefer = make([]conn.Conn, 0, len(conns))
	if allowFallback {
		fallback = make([]conn.Conn, 0, len(conns))
	}

	for _, c := range conns {
		if preferFunc(c) {
			prefer = append(prefer, c)
		} else {
			if allowFallback {
				fallback = append(fallback, c)
			}
		}
	}
	return prefer, fallback
}

func isOkConnection(c conn.Conn, bannedIsOk bool) bool {
	switch c.GetState() {
	case conn.Online, conn.Created, conn.Offline:
		return true
	case conn.Banned:
		return bannedIsOk
	default:
		return false
	}
}
