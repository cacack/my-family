package repository

import "encoding/json"

// SetEventUpcasterForTest registers upcast for eventType and returns a func
// that restores the registry as it was.
func SetEventUpcasterForTest(eventType string, upcast func(json.RawMessage) (json.RawMessage, error)) (restore func()) {
	prev, had := eventUpcasters[eventType]
	eventUpcasters[eventType] = upcast
	return func() {
		if had {
			eventUpcasters[eventType] = prev
		} else {
			delete(eventUpcasters, eventType)
		}
	}
}
