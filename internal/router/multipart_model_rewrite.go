package router

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"strings"
)

// RewriteMultipartModel replaces the value of the "model" form field in a
// multipart/form-data body with newModel.
//
// It returns the rebuilt body together with the matching Content-Type (the
// boundary is regenerated, so the header must be updated with it). All other
// form fields and file parts are preserved byte-for-byte. When there is no
// "model" field the original body/content type are returned unchanged; a body
// that cannot be parsed is reported as an error so the caller can forward the
// original bytes instead of corrupting the request.
func RewriteMultipartModel(body []byte, contentType, newModel string) ([]byte, string, error) {
	if len(body) == 0 || newModel == "" {
		return body, contentType, nil
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return body, contentType, fmt.Errorf("parse multipart content type: %w", err)
	}
	if !strings.EqualFold(mediaType, "multipart/form-data") {
		return body, contentType, fmt.Errorf("content type %q is not multipart/form-data", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return body, contentType, fmt.Errorf("multipart/form-data content type has no boundary")
	}

	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var rebuilt bytes.Buffer
	writer := multipart.NewWriter(&rebuilt)
	sawModelField := false
	changed := false
	sawPart := false
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return body, contentType, fmt.Errorf("read multipart body: %w", err)
		}
		// Clone the part headers (Content-Disposition, Content-Type, ...) so the
		// rebuilt part stays semantically identical for the upstream.
		sawPart = true
		headers := make(textproto.MIMEHeader, len(part.Header))
		for key, values := range part.Header {
			headers[key] = append([]string(nil), values...)
		}
		dst, err := writer.CreatePart(headers)
		if err != nil {
			_ = part.Close()
			return body, contentType, fmt.Errorf("rebuild multipart part: %w", err)
		}
		if part.FormName() == "model" {
			// Read the old value (bounded: a model name is tiny) and write the
			// mapped one instead. When the mapping is an identity the original
			// body is returned untouched, so unaliased traffic keeps its exact
			// bytes and boundary.
			current, err := io.ReadAll(io.LimitReader(part, 1<<20))
			if err != nil {
				_ = part.Close()
				return body, contentType, fmt.Errorf("read multipart model field: %w", err)
			}
			value := newModel
			if string(current) != newModel {
				changed = true
			} else {
				value = string(current)
			}
			if _, err := dst.Write([]byte(value)); err != nil {
				_ = part.Close()
				return body, contentType, fmt.Errorf("write multipart model field: %w", err)
			}
			sawModelField = true
			_ = part.Close()
			continue
		}
		if _, err := io.Copy(dst, part); err != nil {
			_ = part.Close()
			return body, contentType, fmt.Errorf("copy multipart part: %w", err)
		}
		_ = part.Close()
	}
	if err := writer.Close(); err != nil {
		return body, contentType, fmt.Errorf("finalize multipart body: %w", err)
	}
	if !sawPart {
		// A body that ends before its first boundary (or a preamble with a
		// truncated part) is not a valid multipart payload; forwarding it
		// blindly would hide a broken client request.
		if bytes.Contains(body, []byte("--"+boundary+"--")) {
			return body, contentType, nil
		}
		return body, contentType, fmt.Errorf("multipart body contains no parts")
	}
	if !sawModelField || !changed {
		return body, contentType, nil
	}
	return rebuilt.Bytes(), writer.FormDataContentType(), nil
}
