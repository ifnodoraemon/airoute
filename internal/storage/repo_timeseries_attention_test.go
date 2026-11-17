package storage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTimeSeriesAttentionSummary(t *testing.T) {
	db, err := OpenDB(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	checkTimeSeriesAttention(t, NewRepository(db))
}

func checkTimeSeriesAttention(t *testing.T, repo *Repository) {
	t.Helper()
	now := time.Now()
	key := "attention-scope-" + NewBillingID()
	cases := []struct {
		key     string
		status  int
		message string
		at      time.Time
	}{
		{key, 200, "", now},
		{key, 429, "rate limited", now},
		{key, 503, "unavailable", now},
		{key, 200, "stream truncated", now},
		{"foreign-" + key, 429, "foreign tenant", now},
		{key, 429, "outside window", now.Add(-2 * time.Hour)},
	}
	for _, tc := range cases {
		require.NoError(t, repo.RecordUsageLogContext(context.Background(), &UsageLogRecord{
			APIKey: tc.key, StatusCode: tc.status, ErrorMessage: tc.message, CreatedAt: tc.at,
		}))
	}
	data, err := repo.GetStatsTimeSeries("1h", []string{key})
	require.NoError(t, err)
	require.NotNil(t, data.Attention)
	require.EqualValues(t, 4, data.Attention.TotalRequests)
	require.EqualValues(t, 3, data.Attention.FailedRequests)
	require.EqualValues(t, 1, data.Attention.RateLimitedRequests)
	require.Equal(t, map[int]int64{200: 1, 429: 1, 503: 1}, data.Attention.ErrorCountByCode)
	empty, err := repo.GetStatsTimeSeries("1h", []string{"empty-" + key})
	require.NoError(t, err)
	require.Zero(t, empty.Attention.FailedRequests)
	require.Empty(t, empty.Attention.ErrorCountByCode)
}
