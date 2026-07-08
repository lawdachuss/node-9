package server

import (
	"fmt"
	"os"
	"sync"
	"time"
)

type logEntry struct {
	Time    time.Time `json:"time"`
	Message string    `json:"message"`
}

type LogBuffer struct {
	mu    sync.RWMutex
	lines []logEntry
	cap   int
}

var globalLogBuffer = NewLogBuffer(5000)

func GetLogBuffer() *LogBuffer {
	return globalLogBuffer
}

func NewLogBuffer(capacity int) *LogBuffer {
	return &LogBuffer{
		lines: make([]logEntry, 0, capacity),
		cap:   capacity,
	}
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for len(p) > 0 {
		idx := -1
		for i, c := range p {
			if c == '\n' {
				idx = i
				break
			}
		}

		var line string
		if idx >= 0 {
			line = string(p[:idx])
			p = p[idx+1:]
		} else {
			line = string(p)
			p = nil
		}

		if line != "" {
			if len(b.lines) >= b.cap {
				b.lines = b.lines[1:]
			}
			b.lines = append(b.lines, logEntry{Time: time.Now(), Message: line})
		}
	}

	return len(p), nil
}

func (b *LogBuffer) Lines(n int) []logEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if n <= 0 || n > len(b.lines) {
		n = len(b.lines)
	}
	result := make([]logEntry, n)
	copy(result, b.lines[len(b.lines)-n:])
	return result
}

func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lines = b.lines[:0]
}

func (b *LogBuffer) WriteString(s string) {
	b.Write([]byte(s))
}

// LoadWorkflowLogs reads a workflow setup log file (written by the GitHub Actions
// workflow before the keep-alive loop) and injects its lines into the log buffer
// so they appear in /api/logs alongside runtime logs.
func LoadWorkflowLogs(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	buf := GetLogBuffer()
	buf.WriteString("━━━ GitHub Actions workflow setup logs ━━━\n")
	buf.Write(data)
	buf.WriteString("━━━ End of workflow setup logs ━━━\n")
}

func (e logEntry) String() string {
	return fmt.Sprintf("%s %s", e.Time.Format("15:04:05"), e.Message)
}
