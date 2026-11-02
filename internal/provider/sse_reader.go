package provider

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// sseDataReader assembles data fields by event, rather than assuming each
// network line is a complete JSON object. It also accepts JSON pretty-printed
// by compatible relays and a final frame without its trailing newline.
type sseDataReader struct {
	reader  *bufio.Reader
	pending []byte
	eof     bool
}

func newSSEDataReader(reader io.Reader) *sseDataReader {
	return &sseDataReader{reader: bufio.NewReader(reader)}
}

func (r *sseDataReader) next() ([]byte, error) {
	var data []byte
	for {
		if r.eof && len(r.pending) == 0 {
			if len(data) > 0 {
				return data, nil
			}
			return nil, io.EOF
		}
		line := r.pending
		r.pending = nil
		if line == nil {
			var err error
			line, err = r.reader.ReadBytes('\n')
			if err != nil {
				if err != io.EOF {
					return nil, err
				}
				r.eof = true
			}
		}
		line = bytes.TrimRight(line, "\r\n")
		if bytes.HasPrefix(line, []byte("data:")) {
			part := bytes.TrimPrefix(bytes.TrimPrefix(line, []byte("data:")), []byte(" "))
			if len(data) > 0 && json.Valid(data) {
				r.pending = append(append([]byte(nil), line...), '\n')
				return data, nil
			}
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, part...)
		} else if len(bytes.TrimSpace(line)) == 0 {
			if len(data) > 0 {
				return data, nil
			}
		} else if len(data) > 0 && !json.Valid(data) && !bytes.HasPrefix(line, []byte(":")) && !bytes.HasPrefix(line, []byte("event:")) && !bytes.HasPrefix(line, []byte("id:")) && !bytes.HasPrefix(line, []byte("retry:")) {
			data = append(data, '\n')
			data = append(data, line...)
		}
		if len(data) > 16<<20 {
			return nil, fmt.Errorf("upstream SSE event exceeds 16 MiB")
		}
	}
}
