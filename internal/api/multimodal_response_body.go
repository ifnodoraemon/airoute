package api

import (
	"bytes"
	"io"
	"strconv"

	"github.com/ifnodoraemon/airoute/internal/router"
)

// bufferAndRepairResponseBody buffers an upstream modality response so it can be
// charged and written exactly once, and repairs the client-visible model name
// (B-08) before anything is flushed.
//
// Reading before charging matters: the previous flow billed and flushed HTTP 200
// first and only then copied the stream, so a truncated upstream body was charged
// as a success while the client received a broken response. Re-installing the
// buffered bytes keeps the header copy loop and the final io.Copy observing the
// same payload.
//
// restoreResponseModelJSON only rewrites a JSON object's top-level model field;
// binary and multipart payloads (audio streams, generated images) pass through
// untouched, so this is safe to run for every modality.
func (h *MultimodalHandler) bufferAndRepairResponseBody(resp *router.UpstreamResponse, modelName string) error {
	body, err := readMultimodalBody(resp.Stream, maxMultimodalResponseBytes)
	if err != nil {
		return err
	}
	repaired := restoreResponseModelJSON(body, modelName)
	resp.Stream = io.NopCloser(bytes.NewReader(repaired))
	if resp.Headers != nil {
		// The buffered body is what the client will read, so the upstream length
		// (and any chunked framing) must not survive the rewrite.
		resp.Headers.Set("Content-Length", strconv.Itoa(len(repaired)))
	}
	return nil
}
