package controlplane

import (
	"errors"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// The channel timeout is the only timeout an operator configures, so it must be
// stored when valid and rejected when it could not take effect.
func TestNormalizeChannelTimeout(t *testing.T) {
	cases := []struct {
		in      int
		want    int
		wantErr bool
	}{
		{0, model.DefaultChannelTimeoutSeconds, false},
		{300, 300, false},
		// An hour, or several hours, must simply work: the proxy budget is a day.
		{3600, 3600, false},
		{7200, 7200, false},
		{model.MaxChannelTimeoutSeconds - 1, model.MaxChannelTimeoutSeconds - 1, false},
		{model.MaxChannelTimeoutSeconds, model.MaxChannelTimeoutSeconds, false},
		// Zero is the "not provided" sentinel on partial updates; a negative value
		// is an outright invalid budget.
		{-5, 0, true},
		{model.MaxChannelTimeoutSeconds + 1, 0, true},
	}
	for _, tc := range cases {
		got, err := normalizeChannelTimeout(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("timeout %d was accepted, want a validation error", tc.in)
			}
			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("timeout %d error is not a ValidationError: %T", tc.in, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("timeout %d rejected: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("timeout %d normalized to %d, want %d", tc.in, got, tc.want)
		}
	}
}
