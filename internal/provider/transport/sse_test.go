package transport

import (
	"strings"
	"testing"
)

func TestSSEReaderParsesFragmentedEvents(t *testing.T) {
	input := strings.Join([]string{
		": keep-alive\r\n",
		"event: first\r\n",
		"data: {\"a\":\r\n",
		"data: 1}\r\n",
		"\r\n",
		"data: second\n",
		"\n",
	}, "")
	reader := NewSSEReader(strings.NewReader(input))

	if !reader.Next() {
		t.Fatalf("first Next() = false, err=%v", reader.Err())
	}
	name, data := reader.Event()
	if name != "first" || data != "{\"a\":\n1}" {
		t.Fatalf("first event = %q, %q", name, data)
	}

	if !reader.Next() {
		t.Fatalf("second Next() = false, err=%v", reader.Err())
	}
	name, data = reader.Event()
	if name != "" || data != "second" {
		t.Fatalf("second event = %q, %q", name, data)
	}
	if reader.Next() {
		t.Fatal("unexpected third event")
	}
	if err := reader.Err(); err != nil {
		t.Fatalf("Err() = %v", err)
	}
}

func TestSSEReaderReturnsTrailingDataWithoutBlankLine(t *testing.T) {
	reader := NewSSEReader(strings.NewReader("event: done\ndata: [DONE]"))
	if !reader.Next() {
		t.Fatalf("Next() = false, err=%v", reader.Err())
	}
	name, data := reader.Event()
	if name != "done" || data != "[DONE]" {
		t.Fatalf("event = %q, %q", name, data)
	}
}
