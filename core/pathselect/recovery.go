package pathselect

import (
	"context"
	"time"

	C "github.com/metacubex/mihomo/constant"
)

const refreshLead = 15 * time.Second

type preferredProbe struct {
	result <-chan error
	cancel context.CancelFunc
}

func (a *Adapter) raceCandidates(ctx context.Context, preferred C.Proxy, parallelism int) C.Proxy {
	jobs := make(chan C.Proxy, len(a.candidates))
	results := make(chan C.Proxy, len(a.candidates))
	count := 0
	for _, candidate := range a.candidates {
		if candidate != preferred {
			jobs <- candidate
			count++
		}
	}
	close(jobs)
	for range min(parallelism, count) {
		go func() {
			for candidate := range jobs {
				if ctx.Err() != nil {
					return
				}
				if a.check(ctx, candidate, a.endpoint) == nil {
					results <- candidate
				} else {
					results <- nil
				}
			}
		}()
	}
	for range count {
		select {
		case winner := <-results:
			if winner != nil {
				return winner
			}
		case <-ctx.Done():
			return nil
		}
	}
	return nil
}

func (a *Adapter) selectCandidate(g *round, preferred C.Proxy) (C.Proxy, bool, *preferredProbe) {
	if !a.verified {
		if preferred != nil && a.check(g.ctx, preferred, a.endpoint) == nil {
			return preferred, false, nil
		}
		return a.raceCandidates(g.ctx, preferred, 4), true, nil
	}
	if preferred == nil {
		return a.raceCandidates(g.ctx, nil, maxVerifiedCandidates), false, nil
	}
	// Keep this probe alive after a backup wins to distinguish slow live sessions
	// from dead sessions that must be closed so their applications reconnect.
	ctx, cancel := context.WithTimeout(g.life, 12*time.Second)
	result := make(chan error, 1)
	pending := &preferredProbe{result: result, cancel: cancel}
	go func() { result <- a.check(ctx, preferred, a.endpoint) }()
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-result:
		cancel()
		if err == nil {
			return preferred, false, nil
		}
		return a.raceCandidates(g.ctx, preferred, maxVerifiedCandidates), true, nil
	case <-g.ctx.Done():
		cancel()
		return nil, false, nil
	case <-timer.C:
	}
	backups := make(chan C.Proxy, 1)
	go func() { backups <- a.raceCandidates(g.ctx, preferred, maxVerifiedCandidates-1) }()
	var preferredFailed bool
	var backupResult <-chan C.Proxy = backups
	var preferredResult <-chan error = result
	for preferredResult != nil || backupResult != nil {
		select {
		case err := <-preferredResult:
			cancel()
			pending, preferredResult = nil, nil
			if err == nil {
				return preferred, false, nil
			}
			preferredFailed = true
		case winner := <-backupResult:
			backupResult = nil
			if winner != nil {
				return winner, preferredFailed, pending
			}
		case <-g.ctx.Done():
			cancel()
			return nil, false, nil
		}
	}
	return nil, preferredFailed, pending
}

func (a *Adapter) watchPreferred(life context.Context, preferred C.Proxy, pending *preferredProbe) {
	defer pending.cancel()
	select {
	case err := <-pending.result:
		if err == nil {
			return
		}
	case <-life.Done():
		return
	}
	a.mu.Lock()
	var retired []*session
	if a.life == life && a.selected != nil && a.selected != preferred {
		for s := range a.sessions {
			if s.life == life && s.route == preferred {
				retired = append(retired, s)
			}
		}
	}
	a.mu.Unlock()
	for _, s := range retired {
		_ = s.Close()
	}
}

func (a *Adapter) backoffLocked() {
	delay := 15 * time.Second
	if a.verified {
		a.failures = min(a.failures+1, 5)
		delay = min(30*time.Second, time.Duration(max(2, len(a.candidates)/2))*time.Second<<(a.failures-1))
	}
	a.retryAfter = time.Now().Add(delay)
}

func (a *Adapter) invalidate(life context.Context, route C.Proxy) {
	a.mu.Lock()
	if a.life == life && a.selected == route {
		a.validUntil = time.Time{}
	}
	a.mu.Unlock()
}

func (a *Adapter) monitor(life context.Context) {
	timer := time.NewTimer(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-life.Done():
			return
		case <-timer.C:
			a.recheck(life)
			a.mu.Lock()
			if a.life != life {
				a.mu.Unlock()
				return
			}
			delay := 30 * time.Second
			if len(a.sessions) > 0 || time.Since(a.lastUse) < 2*time.Minute {
				due := a.validUntil.Add(-refreshLead)
				if a.retryAfter.After(due) {
					due = a.retryAfter
				}
				delay = max(time.Second, min(delay, time.Until(due)))
			}
			a.mu.Unlock()
			timer.Reset(delay)
		}
	}
}

func (a *Adapter) recheck(life context.Context) {
	a.mu.Lock()
	active := a.life == life && (len(a.sessions) > 0 || time.Since(a.lastUse) < 2*time.Minute)
	due := time.Now().After(a.validUntil.Add(-refreshLead)) && time.Now().After(a.retryAfter)
	a.mu.Unlock()
	if active && due {
		_, _, _ = a.selectRoute(life, false)
	}
}
