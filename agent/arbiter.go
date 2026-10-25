package main

import (
	"encoding/json"
	"net"
	"strconv"
	"sync"
	"time"
)

// arbiter elects a single station for actions that the cloud cannot arbitrate,
// such as the short "Слушаю." acknowledgement. Every station broadcasts a
// claim and the highest priority wins; ties are broken by station id so all
// stations reach the same conclusion without a leader.
type arbiter struct {
	self     string
	priority int
	peers    []string
	port     int
	logf     func(string, ...any)

	mu     sync.Mutex
	claims []arbClaim
}

type arbClaim struct {
	Station  string    `json:"s"`
	Priority int       `json:"p"`
	At       time.Time `json:"-"`
}

func newArbiter(self string, priority int, peers []string, port int, logf func(string, ...any)) *arbiter {
	if port == 0 {
		port = 9876
	}
	if self == "" {
		self = "station"
	}
	a := &arbiter{self: self, priority: priority, peers: peers, port: port, logf: logf}
	go a.listen()
	return a
}

func (a *arbiter) listen() {
	buf := make([]byte, 512)
	for {
		conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: a.port})
		if err != nil {
			// A restart can overlap with the previous process still holding
			// the port, so keep trying instead of staying deaf forever.
			a.logf("arbiter: listen: %v; retrying", err)
			time.Sleep(2 * time.Second)
			continue
		}
		a.logf("arbiter: listening on udp/%d as %q priority=%d", a.port, a.self, a.priority)
		for {
			n, _, err := conn.ReadFromUDP(buf)
			if err != nil {
				break
			}
			var c arbClaim
			if err := json.Unmarshal(buf[:n], &c); err != nil || c.Station == "" || c.Station == a.self {
				continue
			}
			c.At = time.Now()
			a.logf("arbiter: claim from %q priority=%d", c.Station, c.Priority)
			a.mu.Lock()
			a.claims = append(a.claims, c)
			if len(a.claims) > 64 {
				a.claims = a.claims[len(a.claims)-64:]
			}
			a.mu.Unlock()
		}
		conn.Close()
	}
}

func (a *arbiter) claim() {
	if len(a.peers) == 0 {
		return
	}
	body, err := json.Marshal(arbClaim{Station: a.self, Priority: a.priority})
	if err != nil {
		return
	}
	for _, p := range a.peers {
		conn, err := net.Dial("udp", net.JoinHostPort(p, strconv.Itoa(a.port)))
		if err != nil {
			continue
		}
		conn.Write(body)
		conn.Close()
	}
}

// wins broadcasts a claim, waits for the collection window and reports
// whether this station is the elected one. Claims are accepted a bit earlier
// than the local call because stations recognise the same phrase at slightly
// different moments.
func (a *arbiter) wins(window time.Duration) bool {
	if len(a.peers) == 0 {
		return true
	}
	const lookback = 1500 * time.Millisecond
	start := time.Now()
	a.claim()
	time.Sleep(window)

	a.mu.Lock()
	defer a.mu.Unlock()
	mine := arbClaim{Station: a.self, Priority: a.priority}
	fresh := 0
	won := true
	for _, c := range a.claims {
		if c.At.Before(start.Add(-lookback)) {
			continue
		}
		fresh++
		if beats(c, mine) {
			won = false
		}
	}
	a.logf("arbiter: %d peer claim(s), elected=%v", fresh, won)
	return won
}

// beats reports whether a is ordered after b: higher priority wins, and on a
// tie the lexicographically smaller station id wins, so the result is stable
// on every station.
func beats(a, b arbClaim) bool {
	if a.Priority != b.Priority {
		return a.Priority > b.Priority
	}
	return a.Station < b.Station
}
