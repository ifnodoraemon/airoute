package router

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

// buildMultipartBody renders a multipart/form-data body with a model field, a
// regular field and a file part, returning the body and its Content-Type.
func buildMultipartBody(t *testing.T, modelValue string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	if err := writer.WriteField("model", modelValue); err != nil {
		t.Fatalf("write model field: %v", err)
	}
	if err := writer.WriteField("language", "zh"); err != nil {
		t.Fatalf("write language field: %v", err)
	}
	fileWriter, err := writer.CreateFormFile("file", "clip.wav")
	if err != nil {
		t.Fatalf("create file part: %v", err)
	}
	if _, err := fileWriter.Write([]byte("\x00\x01audio-bytes\x02")); err != nil {
		t.Fatalf("write file part: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	return buf.Bytes(), writer.FormDataContentType()
}

// #9: the model alias must be rewritten inside multipart/form-data STT bodies,
// with a valid boundary and Content-Type matching the rebuilt body.
func TestRewriteMultipartModelRewritesAlias(t *testing.T) {
	body, contentType := buildMultipartBody(t, "speech")

	rewritten, newContentType, err := RewriteMultipartModel(body, contentType, "whisper-1")
	if err != nil {
		t.Fatalf("RewriteMultipartModel: %v", err)
	}
	if bytes.Equal(rewritten, body) {
		t.Fatal("multipart body was not rewritten")
	}

	mediaType, params, err := mime.ParseMediaType(newContentType)
	if err != nil {
		t.Fatalf("rebuilt Content-Type %q is invalid: %v", newContentType, err)
	}
	if mediaType != "multipart/form-data" || params["boundary"] == "" {
		t.Fatalf("rebuilt Content-Type = %q", newContentType)
	}

	// The header boundary must be the one actually present in the rebuilt body.
	reader := multipart.NewReader(bytes.NewReader(rewritten), params["boundary"])
	values := map[string]string{}
	var fileBytes []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("rebuilt body is not valid multipart: %v", err)
		}
		data, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("read rebuilt part: %v", err)
		}
		if part.FileName() != "" {
			fileBytes = data
		} else {
			values[part.FormName()] = string(data)
		}
	}
	if values["model"] != "whisper-1" {
		t.Fatalf("model field = %q, want whisper-1", values["model"])
	}
	if values["language"] != "zh" {
		t.Fatalf("language field = %q, want zh (other fields must be preserved)", values["language"])
	}
	if string(fileBytes) != "\x00\x01audio-bytes\x02" {
		t.Fatalf("file part = %q, want original bytes", string(fileBytes))
	}
}

// #9: DispatchHTTP applies the channel alias to multipart STT requests.
func TestDispatchHTTPRewritesMultipartModelAlias(t *testing.T) {
	type captured struct {
		model    string
		language string
		file     string
		ct       string
	}
	got := make(chan captured, 2)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		_, params, err := mime.ParseMediaType(ct)
		if err != nil || params["boundary"] == "" {
			http.Error(w, "bad multipart content type: "+ct, http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		values := map[string]string{}
		var file string
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, "bad multipart body: "+err.Error(), http.StatusBadRequest)
				return
			}
			data, _ := io.ReadAll(part)
			if part.FileName() != "" {
				file = string(data)
			} else {
				values[part.FormName()] = string(data)
			}
		}
		got <- captured{model: values["model"], language: values["language"], file: file, ct: ct}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello"}`))
	}))
	t.Cleanup(srv.Close)

	for _, alias := range []string{"speech", "asr"} {
		t.Run(alias, func(t *testing.T) {
			d := NewDispatcher([]model.ChannelConfig{{
				ID: 1, Name: "stt-" + alias, Type: model.ProviderOpenAI, BaseURL: srv.URL,
				Models:         []string{alias},
				Protocols:      []string{"audio_transcription"},
				ModelMapping:   map[string]string{alias: "whisper-1"},
				Status:         "active",
				Priority:       1,
				Weight:         1,
				TimeoutSeconds: 10,
			}})

			body, contentType := buildMultipartBody(t, alias)
			resp, err := d.DispatchHTTP(context.Background(), &UpstreamRequest{
				Path: "/v1/audio/transcriptions", Method: http.MethodPost,
				Body: body, ContentType: contentType,
				Model: alias, Protocol: "audio_transcription",
			})
			if err != nil {
				t.Fatalf("DispatchHTTP: %v", err)
			}
			if resp.Stream != nil {
				_, _ = io.Copy(io.Discard, resp.Stream)
				_ = resp.Stream.Close()
			}

			select {
			case upstream := <-got:
				if upstream.model != "whisper-1" {
					t.Fatalf("upstream model field = %q, want whisper-1", upstream.model)
				}
				if upstream.language != "zh" {
					t.Fatalf("upstream language field = %q, want zh", upstream.language)
				}
				if upstream.file != "\x00\x01audio-bytes\x02" {
					t.Fatalf("upstream file = %q, want original bytes", upstream.file)
				}
				if !strings.HasPrefix(upstream.ct, "multipart/form-data; boundary=") {
					t.Fatalf("upstream Content-Type = %q", upstream.ct)
				}
			default:
				t.Fatal("upstream never received the multipart request")
			}
		})
	}
}

// #9: bodies without a model field are forwarded unchanged (no boundary churn).
func TestRewriteMultipartModelNoModelFieldIsNoOp(t *testing.T) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	_ = writer.WriteField("language", "zh")
	fileWriter, _ := writer.CreateFormFile("file", "clip.wav")
	_, _ = fileWriter.Write([]byte("audio"))
	_ = writer.Close()
	body := buf.Bytes()
	contentType := writer.FormDataContentType()

	rewritten, newContentType, err := RewriteMultipartModel(body, contentType, "whisper-1")
	if err != nil {
		t.Fatalf("RewriteMultipartModel: %v", err)
	}
	if !bytes.Equal(rewritten, body) || newContentType != contentType {
		t.Fatal("a body without a model field must be returned unchanged")
	}
}

// #9: when the channel mapping is an identity no rebuild happens, so the bytes
// and boundary an unaliased STT request arrived with are preserved exactly.
func TestRewriteMultipartModelIdentityIsNoOp(t *testing.T) {
	body, contentType := buildMultipartBody(t, "whisper-1")

	rewritten, newContentType, err := RewriteMultipartModel(body, contentType, "whisper-1")
	if err != nil {
		t.Fatalf("RewriteMultipartModel: %v", err)
	}
	if !bytes.Equal(rewritten, body) || newContentType != contentType {
		t.Fatal("an identity model mapping must not rebuild the multipart body")
	}
}

// #9: error cases must be reported so DispatchHTTP can forward the original.
func TestRewriteMultipartModelErrors(t *testing.T) {
	body, contentType := buildMultipartBody(t, "speech")

	t.Run("wrong media type", func(t *testing.T) {
		if _, _, err := RewriteMultipartModel(body, "application/json", "m"); err == nil {
			t.Fatal("expected an error for a non-multipart content type")
		}
	})
	t.Run("missing boundary", func(t *testing.T) {
		if _, _, err := RewriteMultipartModel(body, "multipart/form-data", "m"); err == nil {
			t.Fatal("expected an error for a missing boundary")
		}
	})
	t.Run("malformed body", func(t *testing.T) {
		if _, _, err := RewriteMultipartModel([]byte("not a multipart body"), contentType, "m"); err == nil {
			t.Fatal("expected an error for a malformed multipart body")
		}
	})
	t.Run("empty new model", func(t *testing.T) {
		rewritten, ct, err := RewriteMultipartModel(body, contentType, "")
		if err != nil {
			t.Fatalf("empty model must be a no-op, got %v", err)
		}
		if !bytes.Equal(rewritten, body) || ct != contentType {
			t.Fatal("empty model must return the original body")
		}
	})
}
