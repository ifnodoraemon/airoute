//go:build integration

package distributed

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRedisFailedPinCannotDeleteConcurrentSuccessfulPin(t *testing.T) {
	client := InitRedis(testRedisURL(t))
	defer InitRedis("")
	if client == nil {
		t.Fatal("Redis is unavailable")
	}
	ctx := context.Background()
	session := fmt.Sprintf("pin-cas-%d", time.Now().UnixNano())
	if err := client.SetSessionChannel(ctx, "it-pin", session, "b", time.Minute); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if e := client.SetSessionChannel(ctx, "it-pin", session, "b", time.Minute); e != nil {
				t.Error(e)
			}
		}()
		go func() {
			defer wg.Done()
			if e := client.DeleteSessionChannelIfMatches(ctx, "it-pin", session, "a"); e != nil {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	// A delayed failure arriving after every successful writer must also be a
	// no-op, independently of the goroutine schedule above.
	if err := client.DeleteSessionChannelIfMatches(ctx, "it-pin", session, "a"); err != nil {
		t.Fatal(err)
	}
	if got, e := client.GetSessionChannel(ctx, "it-pin", session); e != nil || got != "b" {
		t.Fatalf("late failure deleted a successful pin: %q %v", got, e)
	}
	if e := client.DeleteSessionChannelIfMatches(ctx, "it-pin", session, "b"); e != nil {
		t.Fatal(e)
	}
	if got, e := client.GetSessionChannel(ctx, "it-pin", session); e != nil || got != "" {
		t.Fatalf("matching failed pin survived: %q %v", got, e)
	}
}
