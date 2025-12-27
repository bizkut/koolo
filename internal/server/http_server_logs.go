package server

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// AddLog adds a log entry to the buffer
func (s *HttpServer) AddLog(entry LogEntry) {
	if s.LogBufferManager != nil {
		s.LogBufferManager.Append(entry)
	}
}

// BroadcastLogs streams logs to websocket subscribers
func (s *HttpServer) BroadcastLogs() {
	ch := s.LogBufferManager.Subscribe()
	defer s.LogBufferManager.Unsubscribe(ch)

	for entry := range ch {
		msg := map[string]interface{}{
			"type": "log",
			"data": entry,
		}

		wrappedJson, err := json.Marshal(msg)
		if err != nil {
			s.logger.Error("Failed to marshal log entry", "error", err)
			continue
		}

		s.wsServer.broadcast <- wrappedJson
	}
}

func (s *HttpServer) getLogs(w http.ResponseWriter, r *http.Request) {
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "koolo"
	}

	// Support getting last N logs
	lastN := 100
	if n := r.URL.Query().Get("last"); n != "" {
		if parsed, err := strconv.Atoi(n); err == nil && parsed > 0 {
			lastN = parsed
		}
	}

	buf := s.LogBufferManager.GetBuffer(source)
	logs := buf.GetLast(lastN)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *HttpServer) getLogSources(w http.ResponseWriter, r *http.Request) {
	sources := s.LogBufferManager.GetSources()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sources)
}
