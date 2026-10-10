package storage

// SeedDefaultModelPrices seeds industry-standard pricing benchmark presets if table is empty.
func (r *Repository) SeedDefaultModelPrices() error {
	var count int
	_ = r.db.QueryRow(`SELECT COUNT(*) FROM model_prices`).Scan(&count)
	if count > 0 {
		return nil
	}

	defaults := []ModelPriceRecord{
		// 1. Series benchmarks & wildcards (auto-match entire series)
		{Model: "DeepSeek 系列", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek*", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "OpenAI GPT 系列", PromptPrice: 15.0, CompletionPrice: 60.0, CacheReadPrice: 7.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-*", PromptPrice: 15.0, CompletionPrice: 60.0, CacheReadPrice: 7.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "o1*", PromptPrice: 105.0, CompletionPrice: 420.0, CacheReadPrice: 52.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "o3*", PromptPrice: 7.7, CompletionPrice: 30.8, CacheReadPrice: 3.85, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Anthropic Claude 系列", PromptPrice: 20.0, CompletionPrice: 100.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-*", PromptPrice: 20.0, CompletionPrice: 100.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Google Gemini 系列", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-*", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "通义千问 Qwen 系列", PromptPrice: 4.0, CompletionPrice: 12.0, CacheReadPrice: 0.8, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "qwen-*", PromptPrice: 4.0, CompletionPrice: 12.0, CacheReadPrice: 0.8, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "FLUX 图像生成系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "flux-*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "视频生成系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideo*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "Whisper 语音识别系列", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-*", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 2. 2026 Frontier & Canonical Models
		// OpenAI 2026 (GPT-6 series & live transcribe)
		{Model: "gpt-6", PromptPrice: 25.0, CompletionPrice: 100.0, CacheReadPrice: 12.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6-luna", PromptPrice: 12.0, CompletionPrice: 48.0, CacheReadPrice: 6.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-6.1-sol", PromptPrice: 28.0, CompletionPrice: 112.0, CacheReadPrice: 14.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-5.5-instant", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.75, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-live-transcribe", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.03, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gpt-4o", PromptPrice: 18.0, CompletionPrice: 72.0, CacheReadPrice: 9.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// Anthropic Claude 2026 (Claude 5.5 series & Fable)
		{Model: "claude-opus-5.5", PromptPrice: 30.0, CompletionPrice: 150.0, CacheReadPrice: 3.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-sonnet-5.5", PromptPrice: 15.0, CompletionPrice: 75.0, CacheReadPrice: 1.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-haiku-5.5", PromptPrice: 3.0, CompletionPrice: 15.0, CacheReadPrice: 0.3, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-fable-5.1", PromptPrice: 50.0, CompletionPrice: 250.0, CacheReadPrice: 5.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "claude-3-7-sonnet", PromptPrice: 21.0, CompletionPrice: 105.0, CacheReadPrice: 2.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// Google Gemini 2026 (Gemini 4 Argon & 3.8 Flash)
		{Model: "gemini-4-argon", PromptPrice: 18.0, CompletionPrice: 72.0, CacheReadPrice: 4.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.8-flash", PromptPrice: 1.0, CompletionPrice: 4.0, CacheReadPrice: 0.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-3.5-flash-lite", PromptPrice: 0.4, CompletionPrice: 1.6, CacheReadPrice: 0.1, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-nano-banana-2.1", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-2.0-flash", PromptPrice: 0.7, CompletionPrice: 2.8, CacheReadPrice: 0.175, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "gemini-1.5-pro", PromptPrice: 9.0, CompletionPrice: 36.0, CacheReadPrice: 2.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// DeepSeek 2026 (V4-Pro & V4.1-Flash)
		{Model: "deepseek-v4-pro", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 1.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-v4.1-flash", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.35, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-r1", PromptPrice: 4.0, CompletionPrice: 16.0, CacheReadPrice: 1.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "deepseek-chat", PromptPrice: 2.0, CompletionPrice: 8.0, CacheReadPrice: 0.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: true, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 国内主流 2026 (Qwen 3.8 / GLM-5.3 / Doubao-Seed-2.1 / Kimi-K3)
		{Model: "qwen-3.8", PromptPrice: 5.0, CompletionPrice: 20.0, CacheReadPrice: 1.25, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "qwen-coder", PromptPrice: 3.5, CompletionPrice: 14.0, CacheReadPrice: 0.85, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "glm-5.3", PromptPrice: 6.0, CompletionPrice: 24.0, CacheReadPrice: 1.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.1-pro", PromptPrice: 1.5, CompletionPrice: 6.0, CacheReadPrice: 0.35, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.1-turbo", PromptPrice: 0.8, CompletionPrice: 3.2, CacheReadPrice: 0.2, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "doubao-seed-2.0", PromptPrice: 1.2, CompletionPrice: 4.8, CacheReadPrice: 0.3, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kimi-k3", PromptPrice: 10.0, CompletionPrice: 40.0, CacheReadPrice: 2.5, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kimi-latest", PromptPrice: 8.0, CompletionPrice: 32.0, CacheReadPrice: 2.0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},

		// 多模态与检索 2026 (FLUX.1-Pro / Kling 4.0 / CogVideoX / EmbeddingGemma-2)
		{Model: "flux-1.1-pro", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.15, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "flux-1-schnell", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.05, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "kling-4.0", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.80, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideox-5b", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.80, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "cogvideox", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.50, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "embeddinggemma-2", PromptPrice: 0.08, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-large-v3-turbo", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.02, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "whisper-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.05, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "dall-e-3", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.28, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
		{Model: "tts-1", PromptPrice: 0, CompletionPrice: 0, CacheReadPrice: 0, FixedPrice: 0.10, Currency: "CNY", OffPeakEnabled: false, OffPeakStart: "00:00", OffPeakEnd: "08:30", OffPeakDiscount: 0.5},
	}

	for _, d := range defaults {
		_ = r.SaveModelPrice(&d)
	}
	return nil
}
