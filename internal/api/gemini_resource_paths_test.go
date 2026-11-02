package api

import "testing"

func TestGoogleResourcePathCannotEscapeOwnedIdentifier(t *testing.T) {
	for _, tail := range []string{"owned/../../cachedContents/foreign", "owned/../foreign", "owned:download/../foreign", "owned/private", "owned:cancel/private"} {
		for _, collection := range []string{"files", "cachedContents", "batches", "interactions"} {
			if validGoogleResourceTail(collection, "owned", tail) {
				t.Fatalf("unsafe resource path accepted: %s/%s", collection, tail)
			}
		}
	}
	for _, test := range []struct{ collection, tail string }{{"files", "owned:download"}, {"batches", "owned:cancel"}, {"interactions", "owned/cancel"}, {"cachedContents", "owned"}} {
		if !validGoogleResourceTail(test.collection, "owned", test.tail) {
			t.Errorf("valid resource path rejected: %+v", test)
		}
	}
}
