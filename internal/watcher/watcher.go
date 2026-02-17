package watcher

import (
	"github.com/contextpilot-dev/memorypilot/pkg/models"
)

// Watcher is the interface for all event watchers
type Watcher interface {
	Start() error
	Stop()
}

// EventSink is a channel that receives events
type EventSink chan<- models.Event
