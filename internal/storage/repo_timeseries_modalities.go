package storage

import (
	"fmt"
	"strings"
)

func inferModalityCategory(modelName string) string {
	lower := strings.ToLower(modelName)
	if strings.Contains(lower, "rerank") || strings.Contains(lower, "bge-reranker") {
		return "rerank"
	}
	if strings.Contains(lower, "embed") || strings.Contains(lower, "bge-") || strings.Contains(lower, "text-embedding") {
		return "embeddings"
	}
	if strings.Contains(lower, "image") || strings.Contains(lower, "dall-e") || strings.Contains(lower, "flux") || strings.Contains(lower, "midjourney") || strings.Contains(lower, "stable-diffusion") || strings.Contains(lower, "seedream") || strings.Contains(lower, "sdxl") {
		return "images"
	}
	if strings.Contains(lower, "tts") || strings.Contains(lower, "speech") || strings.Contains(lower, "whisper") || strings.Contains(lower, "audio") || strings.Contains(lower, "voice") || strings.Contains(lower, "cosyvoice") || strings.Contains(lower, "sensevoice") || strings.Contains(lower, "transcription") || strings.Contains(lower, "asr") {
		return "audio"
	}
	if strings.Contains(lower, "video") || strings.Contains(lower, "sora") || strings.Contains(lower, "kling") || strings.Contains(lower, "wan") || strings.Contains(lower, "cogvideox") || strings.Contains(lower, "seedance") || strings.Contains(lower, "luma") || strings.Contains(lower, "runway") || strings.Contains(lower, "pika") || strings.Contains(lower, "vidu") {
		return "video"
	}
	return "chat"
}

func (r *Repository) readTimeSeriesModalities(window timeSeriesWindow) (map[string]int64, error) {
	modalityCounts := map[string]int64{
		"chat": 0, "images": 0, "audio": 0, "video": 0, "embeddings": 0, "rerank": 0,
	}
	modalityRows, err := r.db.Query(`SELECT COALESCE(model, ''), COUNT(*) FROM usage_logs WHERE created_at >= ?`+window.keyCond+` GROUP BY COALESCE(model, '')`, window.windowArgs...)
	if err != nil {
		return nil, fmt.Errorf("query timeseries modality error: %w", err)
	}
	for modalityRows.Next() {
		var modelName string
		var count int64
		if err := modalityRows.Scan(&modelName, &count); err != nil {
			modalityRows.Close()
			return nil, fmt.Errorf("scan timeseries modality error: %w", err)
		}
		modalityCounts[inferModalityCategory(modelName)] += count
	}
	if err := modalityRows.Err(); err != nil {
		modalityRows.Close()
		return nil, fmt.Errorf("iterate timeseries modality error: %w", err)
	}
	modalityRows.Close()

	return modalityCounts, nil
}
