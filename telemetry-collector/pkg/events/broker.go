package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/ayush-github123/podLen/pkg/models"
)

type EmissionMode string

const (
	ModeSnapshot EmissionMode = "snapshot"
	ModeDelta    EmissionMode = "delta"
	ModeBoth     EmissionMode = "both"
)

type Broker struct {
	mu          sync.RWMutex
	subscribers map[string]chan *models.Event
	counter     int
	mode        EmissionMode
}

func NewBroker(bufferSize int, mode EmissionMode) *Broker {
	return &Broker{
		subscribers: make(map[string]chan *models.Event),
		mode:        mode,
		counter:     0,
	}
}

func (b *Broker) Subscribe(ctx context.Context) (string, <-chan *models.Event, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *models.Event, 10)
	subscriberID := fmt.Sprintf("subscriber_%d", b.counter)
	b.counter++

	b.subscribers[subscriberID] = ch

	return subscriberID, ch, nil
}

func (b *Broker) Unsubscribe(subscriberID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, exists := b.subscribers[subscriberID]; exists {
		close(ch)
		delete(b.subscribers, subscriberID)
		return nil
	}

	return fmt.Errorf("subscriber %s not found", subscriberID)
}

func (b *Broker) PublishEvent(event *models.Event) error {
	b.mu.RLock()
	subscribers := make(map[string]chan *models.Event)
	for id, ch := range b.subscribers {
		subscribers[id] = ch
	}
	b.mu.RUnlock()

	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}

	return nil
}

func (b *Broker) EmitMetrics(metrics *models.Metrics, delta *models.MetricsDelta) error {
	var event *models.Event

	switch b.mode {
	case ModeSnapshot:
		event = &models.Event{
			Type:      models.EventTypeSnapshot,
			Timestamp: metrics.Timestamp,
			Metrics:   metrics,
			Selector: map[string]string{
				"pod":       metrics.PodName,
				"namespace": metrics.PodNamespace,
				"container": metrics.ContainerName,
			},
		}

	case ModeDelta:
		if delta == nil {
			return nil
		}
		event = &models.Event{
			Type:      models.EventTypeDelta,
			Timestamp: metrics.Timestamp,
			Metrics:   metrics,
			Delta:     delta,
			Selector: map[string]string{
				"pod":       metrics.PodName,
				"namespace": metrics.PodNamespace,
				"container": metrics.ContainerName,
			},
		}

	case ModeBoth:
		snapshotEvent := &models.Event{
			Type:      models.EventTypeSnapshot,
			Timestamp: metrics.Timestamp,
			Metrics:   metrics,
			Selector: map[string]string{
				"pod":       metrics.PodName,
				"namespace": metrics.PodNamespace,
				"container": metrics.ContainerName,
			},
		}
		b.PublishEvent(snapshotEvent)

		if delta != nil {
			deltaEvent := &models.Event{
				Type:      models.EventTypeDelta,
				Timestamp: metrics.Timestamp,
				Metrics:   metrics,
				Delta:     delta,
				Selector: map[string]string{
					"pod":       metrics.PodName,
					"namespace": metrics.PodNamespace,
					"container": metrics.ContainerName,
				},
			}
			b.PublishEvent(deltaEvent)
		}
		return nil
	}

	if event != nil {
		return b.PublishEvent(event)
	}

	return nil
}

func (b *Broker) Run(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (b *Broker) GetSubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return len(b.subscribers)
}

func (b *Broker) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}

	return nil
}
