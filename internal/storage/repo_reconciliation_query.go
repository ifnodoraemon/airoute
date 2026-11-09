package storage

func (r *Repository) supplierReconciliationFilter(f ChannelReconciliationFilter) (string, []interface{}) {
	var whereClause string
	var args []interface{}

	var startClause, endClause string
	if r.db.Dialect() == "postgres" {
		startClause = ` AND created_at >= ?`
		endClause = ` AND created_at <= ?`
	} else {
		startClause = ` AND datetime(created_at) >= datetime(?)`
		endClause = ` AND datetime(created_at) <= datetime(?)`
	}

	if f.StartTime != "" {
		whereClause += startClause
		args = append(args, f.StartTime)
	}
	if f.EndTime != "" {
		whereClause += endClause
		args = append(args, f.EndTime)
	}
	if f.Channel != "" {
		whereClause += ` AND channel = ?`
		args = append(args, f.Channel)
	}
	if f.Model != "" {
		whereClause += ` AND model = ?`
		args = append(args, f.Model)
	}

	return whereClause, args
}

func supplierChannelQuery(whereClause string) string {
	channelQuery := `SELECT
		COALESCE(channel, '') as ch_name,
		COUNT(*) as total_reqs,
		SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) as succ_reqs,
		SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as fail_reqs,
		COALESCE(SUM(total_tokens), 0) as tot_tok,
		COALESCE(SUM(prompt_tokens), 0) as prm_tok,
		COALESCE(SUM(CASE WHEN prompt_tokens >= cached_tokens THEN prompt_tokens - cached_tokens ELSE prompt_tokens END), 0) as uncached_prm_tok,
		COALESCE(SUM(completion_tokens), 0) as cmp_tok,
		COALESCE(SUM(cached_tokens), 0) as cch_tok,
		COALESCE(SUM(cost), 0.0) as tot_cost,
		COALESCE(AVG(duration_ms), 0) as avg_dur,
		COALESCE(AVG(ttft_ms), 0) as avg_ttft,
		COALESCE(SUM(completion_tokens) * 1000.0 / NULLIF(SUM(CASE WHEN completion_tokens > 0 THEN (CASE WHEN duration_ms > ttft_ms AND ttft_ms > 0 THEN duration_ms - ttft_ms ELSE duration_ms END) ELSE 0 END), 0), 0) as avg_tps,
		MIN(created_at) as min_created,
		MAX(created_at) as max_created
	FROM usage_logs
	WHERE channel IS NOT NULL AND TRIM(channel) != '' ` + whereClause + `
	GROUP BY channel
	ORDER BY tot_cost DESC, total_reqs DESC`

	return channelQuery
}

func supplierModelQuery(whereClause string) string {
	modelQuery := `SELECT
		COALESCE(channel, '') as ch_name,
		COALESCE(model, 'unknown') as mdl_name,
		COUNT(*) as total_reqs,
		SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) as succ_reqs,
		SUM(CASE WHEN status_code >= 400 THEN 1 ELSE 0 END) as fail_reqs,
		COALESCE(SUM(total_tokens), 0) as tot_tok,
		COALESCE(SUM(prompt_tokens), 0) as prm_tok,
		COALESCE(SUM(CASE WHEN prompt_tokens >= cached_tokens THEN prompt_tokens - cached_tokens ELSE prompt_tokens END), 0) as uncached_prm_tok,
		COALESCE(SUM(completion_tokens), 0) as cmp_tok,
		COALESCE(SUM(cached_tokens), 0) as cch_tok,
		COALESCE(SUM(cost), 0.0) as tot_cost,
		COALESCE(AVG(duration_ms), 0) as avg_dur,
		COALESCE(AVG(ttft_ms), 0) as avg_ttft,
		COALESCE(SUM(completion_tokens) * 1000.0 / NULLIF(SUM(CASE WHEN completion_tokens > 0 THEN (CASE WHEN duration_ms > ttft_ms AND ttft_ms > 0 THEN duration_ms - ttft_ms ELSE duration_ms END) ELSE 0 END), 0), 0) as avg_tps
	FROM usage_logs
	WHERE channel IS NOT NULL AND TRIM(channel) != '' ` + whereClause + `
	GROUP BY channel, model
	ORDER BY channel ASC, tot_cost DESC, total_reqs DESC`

	return modelQuery
}
