package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ayush-github123/podLen/pkg/cgroups"
	"github.com/ayush-github123/podLen/pkg/discovery"
	"github.com/ayush-github123/podLen/pkg/models"
)

type Collector struct {
	cgroupReader *cgroups.MetricsReader
	podCache     *discovery.PodCache
	mu           sync.RWMutex
	lastMetrics  map[string]*models.Metrics
}

func NewCollector(ctx context.Context, podCache *discovery.PodCache) (*Collector, error) {
	reader, err := cgroups.NewMetricsReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create cgroup reader: %w", err)
	}

	return &Collector{
		cgroupReader: reader,
		podCache:     podCache,
		lastMetrics:  make(map[string]*models.Metrics),
	}, nil
}

func (c *Collector) CollectMetrics(ctx context.Context) ([]*models.Metrics, error) {
	pods := c.podCache.GetAllPods()
	var allMetrics []*models.Metrics

	c.mu.Lock()
	defer c.mu.Unlock()

	for _, pod := range pods {
		for _, container := range pod.Containers {
			metrics, err := c.collectContainerMetrics(ctx, pod, container)
			if err != nil {
				fmt.Printf("Error collecting metrics for container %s: %v\n", container.ID, err)
				continue
			}

			metrics.PodName = pod.Name
			metrics.PodNamespace = pod.Namespace
			metrics.ContainerName = container.Name
			metrics.ContainerID = container.ID

			c.lastMetrics[container.ID] = metrics
			allMetrics = append(allMetrics, metrics)
		}
	}

	return allMetrics, nil
}

func (c *Collector) collectContainerMetrics(ctx context.Context, pod *models.Pod, container models.Container) (*models.Metrics, error) {
	metrics, err := c.cgroupReader.ReadMetrics(ctx, container.CgroupID)
	if err != nil {
		return nil, fmt.Errorf("failed to read cgroup metrics: %w", err)
	}

	metrics.Timestamp = time.Now()
	metrics.ContainerID = container.ID
	return metrics, nil
}

func (c *Collector) GetLastMetrics(containerID string) *models.Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if metrics, exists := c.lastMetrics[containerID]; exists {
		metricsCopy := *metrics
		return &metricsCopy
	}
	return nil
}

func (c *Collector) Run(ctx context.Context, interval time.Duration, metricsChan chan<- *models.Metrics) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			metrics, err := c.CollectMetrics(ctx)
			if err != nil {
				fmt.Printf("Error collecting metrics: %v\n", err)
				continue
			}

			for _, m := range metrics {
				select {
				case metricsChan <- m:
				case <-ctx.Done():
					return ctx.Err()
				default:
					fmt.Println("Metrics channel full, skipping")
				}
			}
		}
	}
}
