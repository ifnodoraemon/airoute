package nativestate

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ifnodoraemon/airoute/internal/storage"
)

type scopeKey struct{}

func WithOwner(ctx context.Context, owner string) context.Context {
	return context.WithValue(ctx, scopeKey{}, owner)
}
func Owner(ctx context.Context) string { s, _ := ctx.Value(scopeKey{}).(string); return s }
func NewID() string {
	var b [12]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}

var memory = struct {
	sync.Mutex
	rows   map[string]storage.NativeState
	claims map[string]bool
	bytes  int
}{rows: make(map[string]storage.NativeState), claims: make(map[string]bool)}

func stateKey(namespace, id, owner string) string { return namespace + "\x00" + id + "\x00" + owner }

func Save(ctx context.Context, s storage.NativeState) error {
	if s.Owner == "" || s.Identity == "" || s.Namespace == "" || s.ID == "" || len(s.Data) > 64<<20 {
		return fmt.Errorf("invalid native state scope or size")
	}
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.SaveNativeState(ctx, s)
	}
	memory.Lock()
	defer memory.Unlock()
	key := stateKey(s.Namespace, s.ID, s.Owner)
	if old, ok := memory.rows[key]; ok {
		if old.Identity != s.Identity || old.Model != s.Model {
			return fmt.Errorf("native state identity cannot change")
		}
		memory.bytes -= len(old.Data)
	}
	for len(memory.rows) >= 4096 || memory.bytes+len(s.Data) > 64<<20 {
		var oldest string
		var expiry time.Time
		for k, v := range memory.rows {
			if oldest == "" || v.ExpiresAt.Before(expiry) {
				oldest, expiry = k, v.ExpiresAt
			}
		}
		if oldest == "" {
			break
		}
		memory.bytes -= len(memory.rows[oldest].Data)
		delete(memory.rows, oldest)
		delete(memory.claims, oldest)
	}
	memory.rows[key] = s
	memory.bytes += len(s.Data)
	return nil
}

func Claim(ctx context.Context, namespace, id, owner string) (bool, error) {
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.ClaimNativeState(ctx, namespace, id, owner)
	}
	memory.Lock()
	defer memory.Unlock()
	key := stateKey(namespace, id, owner)
	if _, ok := memory.rows[key]; !ok || memory.claims[key] {
		return false, nil
	}
	memory.claims[key] = true
	return true, nil
}

func Complete(ctx context.Context, namespace, id, owner string) error {
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.CompleteNativeState(ctx, namespace, id, owner)
	}
	return nil
}

func Load(ctx context.Context, namespace, id, owner string) (*storage.NativeState, error) {
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.LoadNativeState(ctx, namespace, id, owner)
	}
	memory.Lock()
	defer memory.Unlock()
	s, ok := memory.rows[stateKey(namespace, id, owner)]
	if !ok || !time.Now().Before(s.ExpiresAt) {
		return nil, storage.ErrNativeStateNotFound
	}
	return &s, nil
}

func List(ctx context.Context, namespace, owner string) ([]storage.NativeState, error) {
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.ListNativeStates(ctx, namespace, owner)
	}
	memory.Lock()
	defer memory.Unlock()
	var out []storage.NativeState
	for _, s := range memory.rows {
		if s.Namespace == namespace && s.Owner == owner && time.Now().Before(s.ExpiresAt) {
			out = append(out, s)
		}
	}
	return out, nil
}

func Delete(ctx context.Context, namespace, id, owner string) error {
	if repo := storage.GetGlobalRepository(); repo != nil {
		return repo.DeleteNativeState(ctx, namespace, id, owner)
	}
	memory.Lock()
	defer memory.Unlock()
	key := stateKey(namespace, id, owner)
	memory.bytes -= len(memory.rows[key].Data)
	delete(memory.rows, key)
	delete(memory.claims, key)
	return nil
}
