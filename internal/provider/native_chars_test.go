package provider

import (
	"strings"
	"testing"

	"github.com/ifnodoraemon/airoute/internal/model"
)

func TestNativeMissingUsageCountsGeneratedTextWithoutSignaturesOrBinary(t *testing.T) {
	for _, test := range []struct{ protocol, body string }{
		{model.NativeGemini, `{"candidates":[{"content":{"parts":[{"text":"generated text","thoughtSignature":"opaque"},{"inlineData":{"data":"binary"}}]}}]}`},
		{model.NativeMessages, `{"id":"m","content":[{"type":"text","text":"generated text"},{"type":"image","source":{"data":"binary"}}]}`},
		{model.NativeResponses, `{"id":"r","output":[{"type":"message","content":[{"type":"output_text","text":"generated text"},{"type":"output_image","image_url":"binary"}]}]}`},
	} {
		t.Run(test.protocol, func(t *testing.T) {
			body := strings.ReplaceAll(test.body, "binary", strings.Repeat("A", 4096))
			response, err := nativeReply(test.protocol, []byte(body))
			if err != nil {
				t.Fatal(err)
			}
			if response.NativeOutputChars != len("generated text") {
				t.Fatalf("missing-usage estimate included binary/signatures or lost text: %d", response.NativeOutputChars)
			}
		})
	}
}
