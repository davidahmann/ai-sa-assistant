// Copyright 2024 AI SA Assistant Project
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package streaming provides progress event management for real-time response streaming
package streaming

import (
	"encoding/json"
	"sync"
	"time"
)

// EventType represents different types of progress events
type EventType string

const (
	// EventTypeProgress represents a progress update event
	EventTypeProgress EventType = "progress"
	// EventTypeError represents an error event
	EventTypeError EventType = "error"
	// EventTypeComplete represents a completion event
	EventTypeComplete EventType = "complete"
	// EventTypeMetrics represents metrics/performance data
	EventTypeMetrics EventType = "metrics"
)

// StageType represents different stages in the RAG pipeline
type StageType string

const (
	// StageQueryAnalysis represents query analysis and classification
	StageQueryAnalysis StageType = "query_analysis"
	// StageMetadataFilter represents metadata filtering
	StageMetadataFilter StageType = "metadata_filter"
	// StageEmbeddings represents embedding generation
	StageEmbeddings StageType = "embeddings"
	// StageVectorSearch represents vector database search
	StageVectorSearch StageType = "vector_search"
	// StageFreshnessDetection represents freshness keyword detection
	StageFreshnessDetection StageType = "freshness_detection"
	// StageWebSearch represents web search execution
	StageWebSearch StageType = "web_search"
	// StageSynthesis represents LLM synthesis
	StageSynthesis StageType = "synthesis"
	// StageDiagramRendering represents diagram rendering
	StageDiagramRendering StageType = "diagram_rendering"
	// StageComplete represents pipeline completion
	StageComplete StageType = "complete"
)

// Event represents a streaming progress event
type Event struct {
	ID        string                 `json:"id"`
	Type      EventType              `json:"type"`
	Stage     StageType              `json:"stage"`
	Message   string                 `json:"message"`
	Progress  int                    `json:"progress"` // 0-100
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
}

// ProgressCallback is a function type for progress callbacks
type ProgressCallback func(event Event)

// EventStream manages a stream of progress events
type EventStream struct {
	ID        string
	callbacks []ProgressCallback
	events    []Event
	mutex     sync.RWMutex
	closed    bool
}

// NewEventStream creates a new event stream
func NewEventStream(streamID string) *EventStream {
	return &EventStream{
		ID:        streamID,
		callbacks: make([]ProgressCallback, 0),
		events:    make([]Event, 0),
		closed:    false,
	}
}

// AddCallback adds a progress callback to the stream
func (es *EventStream) AddCallback(callback ProgressCallback) {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	if !es.closed {
		es.callbacks = append(es.callbacks, callback)
	}
}

// EmitEvent emits a progress event to all callbacks
func (es *EventStream) EmitEvent(eventType EventType, stage StageType, message string, progress int, data map[string]interface{}) {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	if es.closed {
		return
	}

	event := Event{
		ID:        generateEventID(),
		Type:      eventType,
		Stage:     stage,
		Message:   message,
		Progress:  progress,
		Timestamp: time.Now(),
		Data:      data,
	}

	es.events = append(es.events, event)

	// Notify all callbacks
	for _, callback := range es.callbacks {
		go callback(event)
	}
}

// EmitProgress is a convenience method for emitting progress events
func (es *EventStream) EmitProgress(stage StageType, message string, progress int, data map[string]interface{}) {
	es.EmitEvent(EventTypeProgress, stage, message, progress, data)
}

// EmitError emits an error event
func (es *EventStream) EmitError(stage StageType, message string, err error, data map[string]interface{}) {
	if data == nil {
		data = make(map[string]interface{})
	}
	if err != nil {
		data["error_details"] = err.Error()
	}

	event := Event{
		ID:        generateEventID(),
		Type:      EventTypeError,
		Stage:     stage,
		Message:   message,
		Progress:  0,
		Timestamp: time.Now(),
		Data:      data,
		Error:     message,
	}

	es.mutex.Lock()
	es.events = append(es.events, event)
	callbacks := make([]ProgressCallback, len(es.callbacks))
	copy(callbacks, es.callbacks)
	es.mutex.Unlock()

	// Notify all callbacks
	for _, callback := range callbacks {
		go callback(event)
	}
}

// EmitComplete emits a completion event
func (es *EventStream) EmitComplete(message string, data map[string]interface{}) {
	es.EmitEvent(EventTypeComplete, StageComplete, message, 100, data)
}

// EmitMetrics emits performance metrics
func (es *EventStream) EmitMetrics(stage StageType, metrics map[string]interface{}) {
	es.EmitEvent(EventTypeMetrics, stage, "Performance metrics", 0, metrics)
}

// Close closes the event stream
func (es *EventStream) Close() {
	es.mutex.Lock()
	defer es.mutex.Unlock()

	es.closed = true
	es.callbacks = nil
}

// GetEvents returns all events in the stream
func (es *EventStream) GetEvents() []Event {
	es.mutex.RLock()
	defer es.mutex.RUnlock()

	events := make([]Event, len(es.events))
	copy(events, es.events)
	return events
}

// ToSSEMessage converts an event to Server-Sent Events format
func (e Event) ToSSEMessage() string {
	data, _ := json.Marshal(e)
	return "data: " + string(data) + "\n\n"
}

// StreamManager manages multiple event streams
type StreamManager struct {
	streams map[string]*EventStream
	mutex   sync.RWMutex
}

// NewStreamManager creates a new stream manager
func NewStreamManager() *StreamManager {
	return &StreamManager{
		streams: make(map[string]*EventStream),
	}
}

// CreateStream creates a new event stream
func (sm *StreamManager) CreateStream(streamID string) *EventStream {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	stream := NewEventStream(streamID)
	sm.streams[streamID] = stream
	return stream
}

// GetStream retrieves an existing event stream
func (sm *StreamManager) GetStream(streamID string) (*EventStream, bool) {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stream, exists := sm.streams[streamID]
	return stream, exists
}

// CloseStream closes and removes an event stream
func (sm *StreamManager) CloseStream(streamID string) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if stream, exists := sm.streams[streamID]; exists {
		stream.Close()
		delete(sm.streams, streamID)
	}
}

// CleanupOldStreams removes streams older than the specified duration
func (sm *StreamManager) CleanupOldStreams(maxAge time.Duration) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for streamID, stream := range sm.streams {
		if len(stream.events) > 0 {
			if stream.events[0].Timestamp.Before(cutoff) {
				stream.Close()
				delete(sm.streams, streamID)
			}
		}
	}
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return "event_" + time.Now().Format("20060102150405") + "_" + randomString(6)
}

// randomString generates a random string of specified length
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}
