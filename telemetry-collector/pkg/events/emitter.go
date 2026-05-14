package events

import (
	"context"
	"fmt"
	"sync"

	"github.com/ayush-github123/podLen/pkg/metrics"
	"github.com/ayush-github123/podLen/pkg/models"
)

type Emitter struct {
	broker        *Broker
	deltaComputer *metrics.DeltaComputer
	mu            sync.Mutex
}

func NewEmitter(broker *Broker) *Emitter {
	return &Emitter{
		broker:        broker,
		deltaComputer: metrics.NewDeltaComputer(),
	}
}

func (e *Emitter) EmitMetrics(ctx context.Context, metricsStream <-chan *models.Metrics) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m := <-metricsStream:
			if m == nil {
				continue
			}

			delta := e.deltaComputer.ComputeDelta(m)

			if err := e.broker.EmitMetrics(m, delta); err != nil {
				fmt.Printf("Error emitting metric: %v\n", err)
			}
		}
	}
}

func (e *Emitter) EmitError(errorMsg string) error {
	event := &models.Event{
		Type:  models.EventTypeError,
		Error: errorMsg,
	}

	return e.broker.PublishEvent(event)
}
