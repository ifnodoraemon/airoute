package telemetry

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The fixture was captured from the exporter before this refactor. Keep scalar
// and histogram order, while normalizing only the unordered per-model series.
func TestPrometheusOutputContract(t *testing.T) {
	metrics := prometheusContractMetrics()
	actual := normalizeModelMetricOrder(metrics.ToPrometheusFormat())
	expected, err := os.ReadFile("testdata/prometheus_contract.txt")
	require.NoError(t, err)
	require.Equal(t, string(expected), actual)
}

func prometheusContractMetrics() *Metrics {
	metrics := &Metrics{
		latencyBuckets: make(map[float64]uint64),
		ttftBuckets:    make(map[float64]uint64),
		modelRequests:  make(map[string]map[string]uint64),
	}
	metrics.RecordRequestWithModel("model\"\\\nA", true, 175*time.Millisecond, 11, 19)
	metrics.RecordRequestWithModel("model-B", false, 1350*time.Millisecond, 23, 29)
	metrics.RecordClientCanceled("model-B", 700001*time.Millisecond, 31, 37)
	metrics.RecordTTFT(35 * time.Millisecond)
	metrics.RecordTTFT(135001 * time.Millisecond)
	metrics.SetEstimateBias("model\"\\\nA", -100001, 7)
	metrics.SetEstimateBias("model-B", 200003, 3)
	fields := reflect.ValueOf(metrics).Elem()
	for i := 0; i < fields.NumField(); i++ {
		field := fields.Field(i)
		if !field.CanAddr() || !field.Addr().CanInterface() {
			continue
		}
		switch counter := field.Addr().Interface().(type) {
		case *atomic.Uint64:
			counter.Store(^uint64(0) - uint64(i)) // Must not round through float64.
		case *atomic.Int64:
			counter.Store(-1000 + int64(i))
		}
	}
	return metrics
}

func normalizeModelMetricOrder(text string) string {
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); {
		marker := strings.Index(lines[i], "{model=")
		if marker < 0 {
			i++
			continue
		}
		end := i + 1
		for end < len(lines) && strings.HasPrefix(lines[end], lines[i][:marker]+"{model=") {
			end++
		}
		sort.Strings(lines[i:end])
		i = end
	}
	return strings.Join(lines, "\n")
}
