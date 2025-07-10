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

package streaming

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewEventStream(t *testing.T) {
	stream := NewEventStream("test-stream")

	if stream.ID != "test-stream" {
		t.Errorf("Expected stream ID 'test-stream', got '%s'", stream.ID)
	}

	if stream.closed {
		t.Error("Expected stream to be open initially")
	}

	if len(stream.callbacks) != 0 {
		t.Error("Expected no callbacks initially")
	}

	if len(stream.events) != 0 {
		t.Error("Expected no events initially")
	}
}

func TestEventStream_AddCallback(t *testing.T) {
	stream := NewEventStream("test-stream")
	callbackCalled := false

	callback := func(event Event) {
		callbackCalled = true
	}

	stream.AddCallback(callback)

	if len(stream.callbacks) != 1 {
		t.Error("Expected one callback to be added")
	}

	// Test that callback is called when event is emitted
	stream.EmitProgress(StageQueryAnalysis, "Test message", 50, nil)

	time.Sleep(10 * time.Millisecond) // Give goroutine time to execute

	if !callbackCalled {
		t.Error("Expected callback to be called")
	}
}

func TestEventStream_EmitEvent(t *testing.T) {
	stream := NewEventStream("test-stream")
	var receivedEvent Event
	var wg sync.WaitGroup

	wg.Add(1)
	callback := func(event Event) {
		receivedEvent = event
		wg.Done()
	}

	stream.AddCallback(callback)

	testData := map[string]interface{}{
		"key": "value",
	}

	stream.EmitEvent(EventTypeProgress, StageVectorSearch, "Test progress", 75, testData)

	wg.Wait()

	// Verify event properties
	if receivedEvent.Type != EventTypeProgress {
		t.Errorf("Expected event type %s, got %s", EventTypeProgress, receivedEvent.Type)
	}

	if receivedEvent.Stage != StageVectorSearch {
		t.Errorf("Expected stage %s, got %s", StageVectorSearch, receivedEvent.Stage)
	}

	if receivedEvent.Message != "Test progress" {
		t.Errorf("Expected message 'Test progress', got '%s'", receivedEvent.Message)
	}

	if receivedEvent.Progress != 75 {
		t.Errorf("Expected progress 75, got %d", receivedEvent.Progress)
	}

	if receivedEvent.Data["key"] != "value" {
		t.Error("Expected data to be preserved")
	}

	if receivedEvent.ID == "" {
		t.Error("Expected event to have an ID")
	}

	if receivedEvent.Timestamp.IsZero() {
		t.Error("Expected event to have a timestamp")
	}
}

func TestEventStream_EmitProgress(t *testing.T) {
	stream := NewEventStream("test-stream")
	var receivedEvent Event
	var wg sync.WaitGroup

	wg.Add(1)
	callback := func(event Event) {
		receivedEvent = event
		wg.Done()
	}

	stream.AddCallback(callback)

	stream.EmitProgress(StageEmbeddings, "Generating embeddings", 25, nil)

	wg.Wait()

	if receivedEvent.Type != EventTypeProgress {
		t.Errorf("Expected event type %s, got %s", EventTypeProgress, receivedEvent.Type)
	}

	if receivedEvent.Stage != StageEmbeddings {
		t.Errorf("Expected stage %s, got %s", StageEmbeddings, receivedEvent.Stage)
	}
}

func TestEventStream_EmitError(t *testing.T) {
	stream := NewEventStream("test-stream")
	var receivedEvent Event
	var wg sync.WaitGroup

	wg.Add(1)
	callback := func(event Event) {
		receivedEvent = event
		wg.Done()
	}

	stream.AddCallback(callback)

	testError := errors.New("test error")
	stream.EmitError(StageSynthesis, "Synthesis failed", testError, nil)

	wg.Wait()

	if receivedEvent.Type != EventTypeError {
		t.Errorf("Expected event type %s, got %s", EventTypeError, receivedEvent.Type)
	}

	if receivedEvent.Stage != StageSynthesis {
		t.Errorf("Expected stage %s, got %s", StageSynthesis, receivedEvent.Stage)
	}

	if receivedEvent.Error != "Synthesis failed" {
		t.Errorf("Expected error 'Synthesis failed', got '%s'", receivedEvent.Error)
	}

	if receivedEvent.Data["error_details"] != "test error" {
		t.Error("Expected error details to be included in data")
	}
}

func TestEventStream_EmitComplete(t *testing.T) {
	stream := NewEventStream("test-stream")
	var receivedEvent Event
	var wg sync.WaitGroup

	wg.Add(1)
	callback := func(event Event) {
		receivedEvent = event
		wg.Done()
	}

	stream.AddCallback(callback)

	stream.EmitComplete("Processing complete", nil)

	wg.Wait()

	if receivedEvent.Type != EventTypeComplete {
		t.Errorf("Expected event type %s, got %s", EventTypeComplete, receivedEvent.Type)
	}

	if receivedEvent.Stage != StageComplete {
		t.Errorf("Expected stage %s, got %s", StageComplete, receivedEvent.Stage)
	}

	if receivedEvent.Progress != 100 {
		t.Errorf("Expected progress 100, got %d", receivedEvent.Progress)
	}
}

func TestEventStream_Close(t *testing.T) {
	stream := NewEventStream("test-stream")

	callback := func(event Event) {}
	stream.AddCallback(callback)

	stream.Close()

	if !stream.closed {
		t.Error("Expected stream to be closed")
	}

	if stream.callbacks != nil {
		t.Error("Expected callbacks to be cleared")
	}

	// Test that adding callback after close does nothing
	stream.AddCallback(callback)
	if len(stream.callbacks) != 0 {
		t.Error("Expected no callbacks after close")
	}
}

func TestEventStream_GetEvents(t *testing.T) {
	stream := NewEventStream("test-stream")

	stream.EmitProgress(StageQueryAnalysis, "Message 1", 25, nil)
	stream.EmitProgress(StageVectorSearch, "Message 2", 50, nil)
	stream.EmitComplete("Done", nil)

	events := stream.GetEvents()

	if len(events) != 3 {
		t.Errorf("Expected 3 events, got %d", len(events))
	}

	if events[0].Message != "Message 1" {
		t.Errorf("Expected first event message 'Message 1', got '%s'", events[0].Message)
	}

	if events[1].Message != "Message 2" {
		t.Errorf("Expected second event message 'Message 2', got '%s'", events[1].Message)
	}

	if events[2].Type != EventTypeComplete {
		t.Errorf("Expected third event type %s, got %s", EventTypeComplete, events[2].Type)
	}
}

func TestEvent_ToSSEMessage(t *testing.T) {
	event := Event{
		ID:        "test-id",
		Type:      EventTypeProgress,
		Stage:     StageVectorSearch,
		Message:   "Test message",
		Progress:  50,
		Timestamp: time.Now(),
		Data:      map[string]interface{}{"key": "value"},
	}

	sseMessage := event.ToSSEMessage()

	if !strings.HasPrefix(sseMessage, "data: ") {
		t.Error("Expected SSE message to start with 'data: '")
	}

	if !strings.HasSuffix(sseMessage, "\n\n") {
		t.Error("Expected SSE message to end with double newline")
	}

	// Verify JSON can be parsed
	jsonData := strings.TrimPrefix(sseMessage, "data: ")
	jsonData = strings.TrimSuffix(jsonData, "\n\n")

	var parsedEvent Event
	if err := json.Unmarshal([]byte(jsonData), &parsedEvent); err != nil {
		t.Errorf("Failed to parse SSE message JSON: %v", err)
	}

	if parsedEvent.ID != event.ID {
		t.Error("Expected parsed event to match original")
	}
}

func TestStreamManager(t *testing.T) {
	manager := NewStreamManager()

	// Test creating a stream
	stream1 := manager.CreateStream("stream-1")
	if stream1.ID != "stream-1" {
		t.Errorf("Expected stream ID 'stream-1', got '%s'", stream1.ID)
	}

	// Test getting a stream
	retrievedStream, exists := manager.GetStream("stream-1")
	if !exists {
		t.Error("Expected stream to exist")
	}
	if retrievedStream.ID != "stream-1" {
		t.Error("Expected retrieved stream to match created stream")
	}

	// Test getting non-existent stream
	_, exists = manager.GetStream("non-existent")
	if exists {
		t.Error("Expected non-existent stream to not exist")
	}

	// Test closing a stream
	manager.CloseStream("stream-1")

	_, exists = manager.GetStream("stream-1")
	if exists {
		t.Error("Expected stream to be removed after closing")
	}
}

func TestStreamManager_CleanupOldStreams(t *testing.T) {
	manager := NewStreamManager()

	// Create a stream and add an old event
	stream := manager.CreateStream("old-stream")

	// Manually set an old timestamp
	oldEvent := Event{
		ID:        "old-event",
		Type:      EventTypeProgress,
		Stage:     StageQueryAnalysis,
		Message:   "Old event",
		Progress:  0,
		Timestamp: time.Now().Add(-2 * time.Hour),
	}

	stream.mutex.Lock()
	stream.events = append(stream.events, oldEvent)
	stream.mutex.Unlock()

	// Create a new stream for comparison
	manager.CreateStream("new-stream")

	// Cleanup streams older than 1 hour
	manager.CleanupOldStreams(1 * time.Hour)

	// Old stream should be removed
	_, exists := manager.GetStream("old-stream")
	if exists {
		t.Error("Expected old stream to be cleaned up")
	}

	// New stream should remain
	_, exists = manager.GetStream("new-stream")
	if !exists {
		t.Error("Expected new stream to remain")
	}
}

func TestConcurrentEventEmission(t *testing.T) {
	stream := NewEventStream("concurrent-test")

	var eventCount int
	var mutex sync.Mutex

	callback := func(event Event) {
		mutex.Lock()
		eventCount++
		mutex.Unlock()
	}

	stream.AddCallback(callback)

	// Emit events concurrently
	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 5

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				stream.EmitProgress(StageVectorSearch, "Concurrent event", j*20, nil)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(50 * time.Millisecond) // Give callbacks time to execute

	expectedEvents := numGoroutines * eventsPerGoroutine

	mutex.Lock()
	actualEvents := eventCount
	mutex.Unlock()

	if actualEvents != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, actualEvents)
	}

	// Verify all events were stored
	storedEvents := stream.GetEvents()
	if len(storedEvents) != expectedEvents {
		t.Errorf("Expected %d stored events, got %d", expectedEvents, len(storedEvents))
	}
}
