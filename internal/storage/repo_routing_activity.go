package storage

import (
	"strings"
	"time"
)

type RoutingChannelActivity struct {
	Name       string `json:"name"`
	Requests   int64  `json:"requests"`
	Errors     int64  `json:"errors"`
	Failovers  int64  `json:"failovers"`
	DurationMs int64  `json:"duration_ms"`
}

type RoutingEvent struct {
	ID         int64     `json:"id"`
	TraceID    string    `json:"trace_id"`
	Channel    string    `json:"channel"`
	StatusCode int       `json:"status_code"`
	Error      string    `json:"error"`
	CreatedAt  time.Time `json:"created_at"`
}

type RoutingActivity struct {
	Since     time.Time                          `json:"since"`
	Requests  int64                              `json:"requests"`
	Errors    int64                              `json:"errors"`
	Failovers int64                              `json:"failovers"`
	Channels  map[string]*RoutingChannelActivity `json:"channels"`
	Recent    []RoutingEvent                     `json:"recent"`
}

// GetRoutingActivity aggregates terminal audit records, including recorded
// channel trails. It never treats an unobserved backup as a successful failover.
func (r *Repository) GetRoutingActivity(modelName string, since time.Time) (*RoutingActivity, error) {
	result := &RoutingActivity{Since: since, Channels: map[string]*RoutingChannelActivity{}, Recent: []RoutingEvent{}}
	where, args := r.buildUsageLogWhere(LogFilter{Model: modelName, StartTime: since.UTC().Format(time.RFC3339)})
	rows, err := r.db.Query(`SELECT COALESCE(channel,''), COUNT(*), SUM(CASE WHEN `+usageLogErrorSQL+` THEN 1 ELSE 0 END), SUM(CASE WHEN channel LIKE '%→%' THEN 1 ELSE 0 END), COALESCE(SUM(duration_ms),0) FROM usage_logs WHERE 1=1`+where+` GROUP BY channel`, args...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var trail string
		var count, failures, failovers, duration int64
		if err := rows.Scan(&trail, &count, &failures, &failovers, &duration); err != nil {
			rows.Close()
			return nil, err
		}
		parts := strings.Split(trail, "→")
		name := parts[len(parts)-1]
		entry := result.Channels[name]
		if entry == nil {
			entry = &RoutingChannelActivity{Name: name}
			result.Channels[name] = entry
		}
		entry.Requests += count
		entry.Errors += failures
		entry.Failovers += failovers
		entry.DurationMs += duration
		result.Requests += count
		result.Errors += failures
		result.Failovers += failovers
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	result.Recent, err = r.recentRoutingEvents(where, args)
	return result, err
}

func (r *Repository) recentRoutingEvents(where string, args []interface{}) ([]RoutingEvent, error) {
	rows, err := r.db.Query(`SELECT id, COALESCE(trace_id,''), COALESCE(channel,''), status_code, COALESCE(error_message,''), created_at FROM usage_logs WHERE (`+usageLogErrorSQL+` OR channel LIKE '%→%')`+where+` ORDER BY id DESC LIMIT 8`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []RoutingEvent{}
	for rows.Next() {
		var event RoutingEvent
		if err := rows.Scan(&event.ID, &event.TraceID, &event.Channel, &event.StatusCode, &event.Error, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}
