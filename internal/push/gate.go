package push

import (
	"context"
	"math/rand/v2"
	"net"
	"net/url"
	"sync"
	"time"
)

// netGate turns "no response at all" into a global waiting state instead of a
// pile of per-file failures (F8: network loss pauses, recovery continues).
//
// A transport error first probes reachability. Reachable: the error was
// specific to that request and counts against the file's retry budget.
// Unreachable: every worker waits on the gate, which re-probes until the
// network is back, without consuming retries.
type netGate struct {
	probe    func(ctx context.Context) bool
	onChange func(offline bool)
	sleep    func(ctx context.Context, d time.Duration) error

	mu      sync.Mutex
	offline bool
	wake    chan struct{}
}

// check returns true when the caller waited for the network to come back.
func (g *netGate) check(ctx context.Context) (bool, error) {
	g.mu.Lock()
	if g.offline {
		ch := g.wake
		g.mu.Unlock()
		return true, waitOn(ctx, ch)
	}
	g.mu.Unlock()

	if g.probe(ctx) {
		return false, nil
	}

	g.mu.Lock()
	if !g.offline {
		g.offline = true
		g.wake = make(chan struct{})
		go g.watch(ctx, g.wake)
		if g.onChange != nil {
			g.onChange(true)
		}
	}
	ch := g.wake
	g.mu.Unlock()
	return true, waitOn(ctx, ch)
}

func (g *netGate) watch(ctx context.Context, wake chan struct{}) {
	delay := 2 * time.Second
	for {
		if g.sleep(ctx, delay) != nil {
			break
		}
		if g.probe(ctx) {
			break
		}
		delay = min(delay*2, 30*time.Second)
	}
	g.mu.Lock()
	g.offline = false
	close(wake)
	g.mu.Unlock()
	if ctx.Err() == nil && g.onChange != nil {
		g.onChange(false)
	}
}

func waitOn(ctx context.Context, ch chan struct{}) error {
	select {
	case <-ch:
		return ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// dialProbe reports whether any of the host:port addresses accepts a TCP
// connection. It sends nothing, so it is not a read against the bucket.
func dialProbe(addrs func() []string) func(ctx context.Context) bool {
	return func(ctx context.Context) bool {
		for _, a := range addrs() {
			d := net.Dialer{Timeout: 5 * time.Second}
			c, err := d.DialContext(ctx, "tcp", a)
			if err == nil {
				c.Close()
				return true
			}
		}
		return false
	}
}

// HostPort turns a URL into a dialable "host:port"; "" when it has no host.
func HostPort(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	if u.Port() != "" {
		return u.Host
	}
	if u.Scheme == "http" {
		return net.JoinHostPort(u.Hostname(), "80")
	}
	return net.JoinHostPort(u.Hostname(), "443")
}

func hostPorts(urls ...string) []string {
	var out []string
	for _, u := range urls {
		if hp := HostPort(u); hp != "" {
			out = append(out, hp)
		}
	}
	return out
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// backoff is 1, 2, 4, 8, 16, 32 s with ±20% jitter (F8).
func backoff(attempt int) time.Duration {
	base := time.Second << attempt
	jitter := 0.8 + 0.4*rand.Float64()
	return time.Duration(float64(base) * jitter)
}

// rateLimiter spaces requests so the average stays under bytesPerSec.
type rateLimiter struct {
	mu   sync.Mutex
	rate float64 // bytes per second; 0 = unlimited
	next time.Time
}

func (r *rateLimiter) wait(ctx context.Context, n int) error {
	r.mu.Lock()
	if r.rate <= 0 {
		r.mu.Unlock()
		return nil
	}
	now := time.Now()
	if r.next.Before(now) {
		r.next = now
	}
	at := r.next
	r.next = r.next.Add(time.Duration(float64(n) / r.rate * float64(time.Second)))
	r.mu.Unlock()
	return sleepCtx(ctx, time.Until(at))
}
