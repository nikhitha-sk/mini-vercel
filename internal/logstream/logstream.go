// Package logstream carries build events over Redis pub/sub.
//
// Channel logs:<deploymentId> carries log lines and status changes from the
// worker to every API instance (which relays them to browsers over SSE).
// Channel cancel:<deploymentId> lets the API stop an in-flight build, and
// channel edge:invalidate tells edge routers to drop cached host lookups.
package logstream

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nikhitha-sk/mini-vercel/internal/store"
)

const EdgeInvalidateChannel = "edge:invalidate"

func LogChannel(id string) string    { return "logs:" + id }
func CancelChannel(id string) string { return "cancel:" + id }

// Event is the message format on logs:<id>.
type Event struct {
	Type       string            `json:"type"` // "log" | "status"
	Log        *store.LogLine    `json:"log,omitempty"`
	Status     store.Status      `json:"status,omitempty"`
	Error      string            `json:"error,omitempty"`
	Deployment *store.Deployment `json:"deployment,omitempty"`
}

func Publish(ctx context.Context, rdb *redis.Client, id string, ev Event) {
	b, _ := json.Marshal(ev)
	if err := rdb.Publish(ctx, LogChannel(id), b).Err(); err != nil {
		slog.Warn("publish event", "deployment", id, "err", err)
	}
}

// Logger publishes every line live and persists them in batches.
type Logger struct {
	id   string
	rdb  *redis.Client
	st   *store.Store
	mu   sync.Mutex
	seq  int
	buf  []store.LogLine
	done chan struct{}
	wg   sync.WaitGroup
}

func NewLogger(ctx context.Context, id string, rdb *redis.Client, st *store.Store) (*Logger, error) {
	seq, err := st.NextLogSeq(ctx, id)
	if err != nil {
		return nil, err
	}
	l := &Logger{id: id, rdb: rdb, st: st, seq: seq, done: make(chan struct{})}
	l.wg.Add(1)
	go l.flushLoop()
	return l, nil
}

func (l *Logger) flushLoop() {
	defer l.wg.Done()
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			l.flush()
		case <-l.done:
			l.flush()
			return
		}
	}
}

func (l *Logger) flush() {
	l.mu.Lock()
	batch := l.buf
	l.buf = nil
	l.mu.Unlock()
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.st.InsertLogs(ctx, l.id, batch); err != nil {
		slog.Warn("persist logs", "deployment", l.id, "err", err)
	}
}

// Line records one line of output. stream is stdout, stderr or system.
func (l *Logger) Line(stream, text string) {
	text = strings.TrimRight(text, "\r\n")
	if len(text) > 4000 {
		text = text[:4000] + "…"
	}
	l.mu.Lock()
	line := store.LogLine{Seq: l.seq, TS: time.Now().UTC(), Stream: stream, Line: text}
	l.seq++
	l.buf = append(l.buf, line)
	l.mu.Unlock()
	Publish(context.Background(), l.rdb, l.id, Event{Type: "log", Log: &line})
}

func (l *Logger) System(text string) { l.Line("system", text) }

// Close flushes remaining lines. Safe to call once.
func (l *Logger) Close() {
	close(l.done)
	l.wg.Wait()
}

// LineWriter adapts the logger to io.Writer, splitting on newlines.
type LineWriter struct {
	L      *Logger
	Stream string
	mu     sync.Mutex
	buf    []byte
}

func (w *LineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := indexNL(w.buf)
		if i < 0 {
			break
		}
		w.L.Line(w.Stream, stripANSI(string(w.buf[:i])))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 8192 { // very long line without newline
		w.L.Line(w.Stream, stripANSI(string(w.buf)))
		w.buf = nil
	}
	return len(p), nil
}

func (w *LineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.L.Line(w.Stream, stripANSI(string(w.buf)))
		w.buf = nil
	}
}

func indexNL(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return -1
}

// stripANSI removes terminal escape sequences and carriage-return progress
// redraws so logs render cleanly in the browser.
func stripANSI(s string) string {
	if i := strings.LastIndexByte(s, '\r'); i >= 0 && i < len(s)-1 {
		s = s[i+1:]
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && !((s[j] >= 'A' && s[j] <= 'Z') || (s[j] >= 'a' && s[j] <= 'z')) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
