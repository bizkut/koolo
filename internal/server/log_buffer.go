package server

import (
	"sync"
	"time"
)

// LogEntry represents a single log message
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"` // "koolo" or supervisor name
}

// LogBuffer is a thread-safe ring buffer for log entries with improved efficiency
type LogBuffer struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
	head    int // Points to next write position
	count   int // Current number of entries
}

// NewLogBuffer creates a new log buffer with the specified max size
func NewLogBuffer(maxSize int) *LogBuffer {
	return &LogBuffer{
		entries: make([]LogEntry, maxSize),
		maxSize: maxSize,
	}
}

// Append adds a new log entry to the buffer
func (b *LogBuffer) Append(entry LogEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.entries[b.head] = entry
	b.head = (b.head + 1) % b.maxSize
	if b.count < b.maxSize {
		b.count++
	}
}

// GetAll returns all log entries in chronological order
func (b *LogBuffer) GetAll() []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 {
		return []LogEntry{}
	}

	result := make([]LogEntry, b.count)
	if b.count < b.maxSize {
		// Buffer not yet full, entries are from 0 to count-1
		copy(result, b.entries[:b.count])
	} else {
		// Buffer is full, oldest entry is at head position
		// Copy from head to end, then from start to head
		firstPart := b.maxSize - b.head
		copy(result[:firstPart], b.entries[b.head:])
		copy(result[firstPart:], b.entries[:b.head])
	}
	return result
}

// GetLast returns the last n log entries in chronological order
func (b *LogBuffer) GetLast(n int) []LogEntry {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.count == 0 || n <= 0 {
		return []LogEntry{}
	}

	if n > b.count {
		n = b.count
	}

	result := make([]LogEntry, n)
	// Calculate start position for last n entries
	start := (b.head - n + b.maxSize) % b.maxSize

	for i := 0; i < n; i++ {
		idx := (start + i) % b.maxSize
		result[i] = b.entries[idx]
	}
	return result
}

// Clear empties the buffer
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.head = 0
	b.count = 0
}

// Len returns the current number of entries in the buffer
func (b *LogBuffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.count
}

// LogBufferManager manages multiple log buffers for different sources
type LogBufferManager struct {
	mu      sync.RWMutex
	buffers map[string]*LogBuffer
	maxSize int

	// Subscribers for real-time log streaming
	subMu       sync.RWMutex
	subscribers map[chan LogEntry]struct{}
}

// NewLogBufferManager creates a new manager with the specified buffer size
func NewLogBufferManager(maxSize int) *LogBufferManager {
	return &LogBufferManager{
		buffers:     make(map[string]*LogBuffer),
		maxSize:     maxSize,
		subscribers: make(map[chan LogEntry]struct{}),
	}
}

// GetBuffer returns the buffer for a source, creating it if needed
func (m *LogBufferManager) GetBuffer(source string) *LogBuffer {
	m.mu.Lock()
	defer m.mu.Unlock()

	if buf, ok := m.buffers[source]; ok {
		return buf
	}

	buf := NewLogBuffer(m.maxSize)
	m.buffers[source] = buf
	return buf
}

// Append adds a log entry to the appropriate buffer and notifies subscribers
func (m *LogBufferManager) Append(entry LogEntry) {
	buf := m.GetBuffer(entry.Source)
	buf.Append(entry)

	// Notify subscribers asynchronously to avoid blocking
	go m.notifySubscribers(entry)
}

// Subscribe registers a channel to receive new log entries
func (m *LogBufferManager) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 200) // Increased buffer for better throughput
	m.subMu.Lock()
	m.subscribers[ch] = struct{}{}
	m.subMu.Unlock()
	return ch
}

// Unsubscribe removes a channel from receiving log entries
func (m *LogBufferManager) Unsubscribe(ch chan LogEntry) {
	m.subMu.Lock()
	delete(m.subscribers, ch)
	m.subMu.Unlock()
	// Drain and close after a short delay to avoid blocking senders
	go func() {
		time.Sleep(100 * time.Millisecond)
		close(ch)
	}()
}

// notifySubscribers sends log entry to all subscribers
func (m *LogBufferManager) notifySubscribers(entry LogEntry) {
	m.subMu.RLock()
	defer m.subMu.RUnlock()

	for ch := range m.subscribers {
		select {
		case ch <- entry:
		default:
			// Channel full, skip to avoid blocking
		}
	}
}

// GetSources returns all source names
func (m *LogBufferManager) GetSources() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sources := make([]string, 0, len(m.buffers))
	for source := range m.buffers {
		sources = append(sources, source)
	}
	return sources
}

// GetAllLogs returns logs from a specific source
func (m *LogBufferManager) GetAllLogs(source string) []LogEntry {
	buf := m.GetBuffer(source)
	return buf.GetAll()
}
