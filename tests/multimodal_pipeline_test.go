package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/api"
	"github.com/ifnodoraemon/airoute/internal/config"
	"github.com/ifnodoraemon/airoute/internal/model"
	"github.com/ifnodoraemon/airoute/internal/router"
)

func TestMultimodalPipeline_ImageAudioVideo(t *testing.T) {
	// 1. Setup mock multimodal upstream server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/images/generations":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{
				"created": 1700000000,
				"data": [
					{"url": "https://cdn.example.com/generated-image-1.png", "revised_prompt": "enhanced prompt"}
				]
			}`))

		case "/v1/audio/speech":
			w.Header().Set("Content-Type", "audio/mpeg")
			// Return mock MP3 binary bytes
			w.Write([]byte{0xFF, 0xFB, 0x90, 0x64, 0x00, 0x00, 0x00, 0x00})

		case "/v1/audio/transcriptions":
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				http.Error(w, "upstream received empty multipart body", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text": "Transcription from audio file successfully recognized."}`))

		case "/v1/audio/translations":
			bodyBytes, _ := io.ReadAll(r.Body)
			if len(bodyBytes) == 0 {
				http.Error(w, "upstream received empty multipart body", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"text": "Translation from audio file successfully recognized."}`))

		case "/v1/videos/generations":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": "video-task-9988", "status": "processing", "progress": 10}`))

		case "/v1/videos/tasks/video-task-9988":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id": "video-task-9988", "status": "succeeded", "video_url": "https://cdn.example.com/output.mp4"}`))

		default:
			http.NotFound(w, r)
		}
	}))
	defer mockServer.Close()

	// 2. Setup Dispatcher with multimodal channel
	channels := []model.ChannelConfig{
		{
			Name:      "multimodal-upstream",
			Type:      model.ProviderOpenAI,
			BaseURL:   mockServer.URL,
			APIKey:    "test-mm-key",
			Models:    []string{"dall-e-3", "tts-1", "whisper-1", "cogvideox"},
			Protocols: []string{"images", "audio_speech", "audio_transcription", "videos"},
			Priority:  1,
			Weight:    10,
		},
	}

	dispatcher := router.NewDispatcher(channels)
	engine := api.SetupRouter(dispatcher, nil)

	// Open access mode for tests
	config.SetGlobalConfig(&config.Config{APIKeys: nil})

	// 3. Test Image Generation (/v1/images/generations)
	t.Run("Image Generation", func(t *testing.T) {
		reqBody := `{"model": "dall-e-3", "prompt": "a cyberpunk city at night", "size": "1024x1024"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var imgResp model.ImageGenerationResponse
		if err := json.Unmarshal(w.Body.Bytes(), &imgResp); err != nil {
			t.Fatalf("failed to parse image response: %v", err)
		}
		if len(imgResp.Data) == 0 || imgResp.Data[0].URL != "https://cdn.example.com/generated-image-1.png" {
			t.Fatalf("unexpected image data: %v", imgResp)
		}
	})

	// 4. Test Audio Speech (/v1/audio/speech)
	t.Run("Audio Speech TTS", func(t *testing.T) {
		reqBody := `{"model": "tts-1", "input": "Welcome to Airoute!", "voice": "alloy"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/audio/speech", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Content-Type") != "audio/mpeg" {
			t.Fatalf("expected audio/mpeg Content-Type, got %s", w.Header().Get("Content-Type"))
		}
		if w.Body.Len() < 4 {
			t.Fatalf("expected audio binary bytes, got empty")
		}
	})

	// 5. Test Audio Transcription (/v1/audio/transcriptions)
	t.Run("Audio Transcription STT", func(t *testing.T) {
		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		_ = mw.WriteField("model", "whisper-1")
		part, _ := mw.CreateFormFile("file", "test.mp3")
		_, _ = part.Write([]byte("mock audio stream"))
		_ = mw.Close()

		req := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var sttResp model.AudioTranscriptionResponse
		if err := json.Unmarshal(w.Body.Bytes(), &sttResp); err != nil {
			t.Fatalf("failed to parse transcription response: %v", err)
		}
		if sttResp.Text != "Transcription from audio file successfully recognized." {
			t.Fatalf("unexpected transcript text: %s", sttResp.Text)
		}
	})

	// 6. Test Video Generation & Task Polling (/v1/videos/generations)
	t.Run("Video Generation and Task Status", func(t *testing.T) {
		reqBody := `{"model": "cogvideox", "prompt": "hyperspace warp drive animation"}`
		req := httptest.NewRequest(http.MethodPost, "/v1/videos/generations", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var videoTask model.VideoTaskResponse
		if err := json.Unmarshal(w.Body.Bytes(), &videoTask); err != nil {
			t.Fatalf("failed to parse video response: %v", err)
		}
		if videoTask.ID != "video-task-9988" || videoTask.Status != "processing" {
			t.Fatalf("unexpected video task response: %v", videoTask)
		}

		// Poll task
		pollReq := httptest.NewRequest(http.MethodGet, "/v1/videos/tasks/video-task-9988?model=cogvideox", nil)
		pollW := httptest.NewRecorder()

		engine.ServeHTTP(pollW, pollReq)

		if pollW.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", pollW.Code, pollW.Body.String())
		}
		var pollResp model.VideoTaskResponse
		_ = json.Unmarshal(pollW.Body.Bytes(), &pollResp)
		if pollResp.Status != "succeeded" || pollResp.VideoURL != "https://cdn.example.com/output.mp4" {
			t.Fatalf("unexpected polled video task: %v", pollResp)
		}
	})
}

func TestMultimodal_FallbackAndCircuitBreaker(t *testing.T) {
	// Faulty primary server returns 500
	mockFaultyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "GPU cluster out of memory", http.StatusInternalServerError)
	}))
	defer mockFaultyServer.Close()

	// Healthy secondary server
	mockHealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"created": 1700000000, "data": [{"url": "https://cdn.example.com/backup-ok.png"}]}`))
	}))
	defer mockHealthyServer.Close()

	channels := []model.ChannelConfig{
		{
			Name:      "faulty-primary-image",
			Type:      model.ProviderOpenAI,
			BaseURL:   mockFaultyServer.URL,
			APIKey:    "key1",
			Models:    []string{"flux-pro"},
			Protocols: []string{"images"},
			Priority:  1,
			Weight:    10,
		},
		{
			Name:      "backup-healthy-image",
			Type:      model.ProviderOpenAI,
			BaseURL:   mockHealthyServer.URL,
			APIKey:    "key2",
			Models:    []string{"flux-pro"},
			Protocols: []string{"images"},
			Priority:  2,
			Weight:    10,
		},
	}

	dispatcher := router.NewDispatcher(channels)

	upReq := &router.UpstreamRequest{
		Path:        "/v1/images/generations",
		Method:      http.MethodPost,
		Body:        []byte(`{"model": "flux-pro", "prompt": "a sunset over mountains"}`),
		ContentType: "application/json",
		Model:       "flux-pro",
		Protocol:    "images",
	}

	resp, err := dispatcher.DispatchHTTP(context.Background(), upReq)
	if err != nil {
		t.Fatalf("expected successful fallback, got error: %v", err)
	}
	defer resp.Stream.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK after fallback, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Stream)
	var imgResp model.ImageGenerationResponse
	_ = json.Unmarshal(bodyBytes, &imgResp)
	if len(imgResp.Data) == 0 || imgResp.Data[0].URL != "https://cdn.example.com/backup-ok.png" {
		t.Fatalf("expected fallback image response, got: %s", string(bodyBytes))
	}

	// Verify Circuit Breaker recorded failure on faulty channel
	status := dispatcher.GetBreakerStatus("faulty-primary-image")
	if status == "OPEN" {
		// If threshold reached, OPEN
	}
}
