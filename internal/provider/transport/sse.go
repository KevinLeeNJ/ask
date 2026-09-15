package transport

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const maxSSELineBytes = 1 << 20

type SSEReader struct {
	reader    *bufio.Reader
	eventName string
	data      string
	pending   bool
	done      bool
	err       error
}

func NewSSEReader(reader io.Reader) *SSEReader {
	return &SSEReader{reader: bufio.NewReaderSize(reader, 64*1024)}
}

func (r *SSEReader) Next() bool {
	if r.done {
		return false
	}
	if r.pending {
		r.pending = false
		return true
	}

	eventName := ""
	dataLines := make([]string, 0, 1)
	for {
		line, err := r.reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				r.err = err
				return false
			}
			if line == "" && len(dataLines) == 0 {
				r.done = true
				return false
			}
		}
		if len(line) > maxSSELineBytes {
			r.err = fmt.Errorf("SSE line exceeds %d bytes", maxSSELineBytes)
			return false
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			if len(dataLines) == 0 {
				eventName = ""
				if err == io.EOF {
					r.done = true
					return false
				}
				continue
			}
			r.eventName = eventName
			r.data = strings.Join(dataLines, "\n")
			if err == io.EOF {
				r.done = true
			}
			return true
		}
		if strings.HasPrefix(line, ":") {
			if err == io.EOF {
				r.done = true
				return false
			}
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			eventName = value
		case "data":
			dataLines = append(dataLines, value)
		}
		if err == io.EOF {
			if len(dataLines) == 0 {
				r.done = true
				return false
			}
			r.eventName = eventName
			r.data = strings.Join(dataLines, "\n")
			r.done = true
			return true
		}
	}
}

func (r *SSEReader) Event() (name, data string) {
	return r.eventName, r.data
}

func (r *SSEReader) Err() error {
	return r.err
}
