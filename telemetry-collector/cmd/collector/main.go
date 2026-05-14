package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/ayush-github123/podLen/internal/config"
	"github.com/ayush-github123/podLen/pkg/discovery"
	"github.com/ayush-github123/podLen/pkg/events"
	"github.com/ayush-github123/podLen/pkg/metrics"
	"github.com/ayush-github123/podLen/pkg/models"
)

func main() {
	// Load configuration
	cfg := config.NewConfig()

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	// Initialize pod discovery
	fmt.Println("Initializing pod discovery...")
	discoverer := discovery.NewDiscoverer(cfg.KubeletURL)

	// Perform initial discovery
	if err := performInitialDiscovery(ctx, discoverer); err != nil {
		fmt.Printf("Warning: Initial discovery failed: %v\n", err)
	}

	// Initialize metrics collector
	fmt.Println("Initializing metrics collector...")
	collector, err := metrics.NewCollector(ctx, discoverer.Cache)
	if err != nil {
		fmt.Printf("Fatal: Failed to initialize metrics collector: %v\n", err)
		os.Exit(1)
	}

	// Initialize event broker
	fmt.Printf("Initializing event broker (mode: %s)...\n", cfg.EventMode)
	broker := events.NewBroker(cfg.EventQueueSize, cfg.EventMode)

	// Initialize emitter
	emitter := events.NewEmitter(broker)

	// Setup goroutine coordination
	var wg sync.WaitGroup
	metricsChannel := make(chan *models.Metrics, 100)

	// Start discovery goroutine
	fmt.Println("Starting pod discovery loop...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := discoverer.Run(ctx, cfg.DiscoveryInterval); err != nil && err != context.Canceled {
			fmt.Printf("Discovery loop error: %v\n", err)
		}
	}()

	// Start metrics collection goroutine
	fmt.Println("Starting metrics collection loop...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := collector.Run(ctx, cfg.MetricsInterval, metricsChannel); err != nil && err != context.Canceled {
			fmt.Printf("Collection loop error: %v\n", err)
		}
	}()

	// Start emitter goroutine
	fmt.Println("Starting event emitter...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := emitter.EmitMetrics(ctx, metricsChannel); err != nil && err != context.Canceled {
			fmt.Printf("Emitter loop error: %v\n", err)
		}
	}()

	// Start broker goroutine
	fmt.Println("Starting event broker...")
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := broker.Run(ctx); err != nil && err != context.Canceled {
			fmt.Printf("Broker loop error: %v\n", err)
		}
	}()

	// Example: Subscribe to events (in production, this would be handled by external consumers)
	fmt.Println("Opening example event subscription...")
	subscriberID, eventChan, err := broker.Subscribe(ctx)
	if err != nil {
		fmt.Printf("Warning: Failed to subscribe to events: %v\n", err)
	} else {
		defer broker.Unsubscribe(subscriberID)

		// Example event processor goroutine
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case event := <-eventChan:
					if event != nil {
						fmt.Printf("[%s] %s/%s/%s: %s event\n",
							event.Type,
							event.Selector["namespace"],
							event.Selector["pod"],
							event.Selector["container"],
							event.Type)
					}
				}
			}
		}()
	}

	fmt.Println("Telemetry collector started successfully")
	fmt.Println("Waiting for termination signal...")

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		fmt.Printf("\nReceived signal: %v\n", sig)
	case <-ctx.Done():
		fmt.Println("Context cancelled")
	}

	// Graceful shutdown
	fmt.Println("\nInitiating graceful shutdown...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.GracefulShutdownTimeout)
	defer shutdownCancel()

	// Signal all goroutines to stop
	cancel()

	// Close the metrics channel
	close(metricsChannel)

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("All goroutines shut down gracefully")
	case <-shutdownCtx.Done():
		fmt.Println("Shutdown timeout exceeded, forcing exit")
	}

	// Close broker
	if err := broker.Close(); err != nil {
		fmt.Printf("Error closing broker: %v\n", err)
	}

	fmt.Println("Telemetry collector stopped")
}

func performInitialDiscovery(ctx context.Context, discoverer *discovery.Discoverer) error {
	pods, err := discoverer.DiscoverPods(ctx)
	if err != nil {
		return err
	}

	fmt.Printf("Discovered %d pods\n", len(pods))
	for _, pod := range pods {
		fmt.Printf("  - %s/%s (%d containers)\n", pod.Namespace, pod.Name, len(pod.Containers))
	}

	discoverer.UpdateCache(pods)
	return nil
}
