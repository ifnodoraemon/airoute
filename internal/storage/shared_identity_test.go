package storage

import (
	"fmt"
	"sync"
	"testing"
)

func TestReplicasGetOrCreateOneSecret(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Construct both repositories before concurrency; NewRepository also sets a global.
	nodes := []*Repository{NewRepository(db), NewRepository(db)}
	values := make([]string, 12)
	errs := make([]error, 12)
	var wg sync.WaitGroup
	for i := range values {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			values[i], errs[i] = nodes[i%2].GetOrCreateSetting("cluster-secret-test", fmt.Sprintf("candidate-%d", i))
		}(i)
	}
	wg.Wait()
	for i := range values {
		if errs[i] != nil || values[i] == "" || values[i] != values[0] {
			t.Fatalf("replica %d: %q %v", i, values[i], errs[i])
		}
	}
}
