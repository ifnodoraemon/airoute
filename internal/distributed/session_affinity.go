package distributed

import (
	"context"
	"fmt"
)

// DeleteSessionChannelIfMatches invalidates a failed channel without deleting a
// newer pin written by another request or gateway replica.
func (c *Client) DeleteSessionChannelIfMatches(ctx context.Context, model, sessionID, channel string) error {
	if !c.IsActive() || sessionID == "" || channel == "" {
		return nil
	}
	key := fmt.Sprintf("airoute:session:%s:%s", model, sessionID)
	return c.rdb.Eval(ctx, `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`, []string{key}, channel).Err()
}
