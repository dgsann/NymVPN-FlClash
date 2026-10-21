package pathselect

import (
	"context"
	"fmt"
	"sync"
	"testing"

	C "github.com/metacubex/mihomo/constant"
)

func TestVerifiedStartupDoesNotQueueWorkingRouteBehindBlackholes(t *testing.T) {
	for _, count := range []int{6, 12} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			var candidates []C.Proxy
			for i := range count {
				candidates = append(candidates, fake(fmt.Sprint(i)))
			}
			good := candidates[len(candidates)-1]
			a := New("verified", Config{Version: 2}, candidates)
			stalled := make(chan struct{})
			var first sync.Once
			a.check = func(ctx context.Context, p C.Proxy, _ string) error {
				if p == good {
					select {
					case <-stalled:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				first.Do(func() { close(stalled) })
				<-ctx.Done()
				return ctx.Err()
			}
			a.SetActive(true)
			defer a.Close()
			if choose(t, a) != good {
				t.Fatal("working route was not selected")
			}
		})
	}
}
