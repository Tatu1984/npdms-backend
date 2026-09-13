package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

// EventHandler is a function that handles an event
type EventHandler func(ctx context.Context, event Event) error

// EventConsumer consumes events from Kafka
type EventConsumer struct {
	client       *KafkaClient
	handlers     map[string][]EventHandler
	mu           sync.RWMutex
	running      bool
	stopChan     chan struct{}
	workerCount  int
	errorHandler func(topic string, err error)
}

// ConsumerConfig holds consumer configuration
type ConsumerConfig struct {
	WorkerCount  int
	ErrorHandler func(topic string, err error)
}

// NewEventConsumer creates a new event consumer
func NewEventConsumer(client *KafkaClient, config *ConsumerConfig) *EventConsumer {
	workerCount := 3
	if config != nil && config.WorkerCount > 0 {
		workerCount = config.WorkerCount
	}

	errorHandler := func(topic string, err error) {
		log.Printf("Error consuming from %s: %v", topic, err)
	}
	if config != nil && config.ErrorHandler != nil {
		errorHandler = config.ErrorHandler
	}

	return &EventConsumer{
		client:       client,
		handlers:     make(map[string][]EventHandler),
		stopChan:     make(chan struct{}),
		workerCount:  workerCount,
		errorHandler: errorHandler,
	}
}

// Subscribe registers a handler for a specific topic
func (c *EventConsumer) Subscribe(topic string, handler EventHandler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.handlers[topic] = append(c.handlers[topic], handler)
}

// SubscribeToEventType registers a handler for a specific event type
func (c *EventConsumer) SubscribeToEventType(eventType EventType, handler EventHandler) {
	topic := getTopicForEventType(eventType)
	if topic == "" {
		log.Printf("Warning: no topic mapping for event type: %s", eventType)
		return
	}
	c.Subscribe(topic, handler)
}

// Start starts consuming messages from all subscribed topics
func (c *EventConsumer) Start(ctx context.Context) error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("consumer already running")
	}
	c.running = true
	topics := make([]string, 0, len(c.handlers))
	for topic := range c.handlers {
		topics = append(topics, topic)
	}
	c.mu.Unlock()

	if len(topics) == 0 {
		return fmt.Errorf("no topics subscribed")
	}

	var wg sync.WaitGroup

	for _, topic := range topics {
		for i := 0; i < c.workerCount; i++ {
			wg.Add(1)
			go func(t string, workerID int) {
				defer wg.Done()
				c.consumeTopic(ctx, t, workerID)
			}(topic, i)
		}
	}

	// Wait for stop signal
	go func() {
		<-c.stopChan
		wg.Wait()
		c.mu.Lock()
		c.running = false
		c.mu.Unlock()
	}()

	return nil
}

// Stop stops the consumer
func (c *EventConsumer) Stop() {
	c.mu.RLock()
	if !c.running {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	close(c.stopChan)
}

func (c *EventConsumer) consumeTopic(ctx context.Context, topic string, workerID int) {
	reader := c.client.GetConsumer(topic)

	log.Printf("Starting consumer worker %d for topic %s", workerID, topic)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Consumer worker %d for %s stopping: context cancelled", workerID, topic)
			return
		case <-c.stopChan:
			log.Printf("Consumer worker %d for %s stopping: stop signal received", workerID, topic)
			return
		default:
			msg, err := reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				c.errorHandler(topic, err)
				time.Sleep(time.Second)
				continue
			}

			// Parse the event
			var event Event
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				c.errorHandler(topic, fmt.Errorf("failed to unmarshal event: %w", err))
				// Commit the message even if it fails to parse to avoid blocking
				reader.CommitMessages(ctx, msg)
				continue
			}

			// Get handlers for this topic
			c.mu.RLock()
			handlers := c.handlers[topic]
			c.mu.RUnlock()

			// Execute all handlers
			var handlerErr error
			for _, handler := range handlers {
				if err := handler(ctx, event); err != nil {
					handlerErr = err
					c.errorHandler(topic, fmt.Errorf("handler error: %w", err))
				}
			}

			// Commit the message if all handlers succeeded
			if handlerErr == nil {
				if err := reader.CommitMessages(ctx, msg); err != nil {
					c.errorHandler(topic, fmt.Errorf("failed to commit message: %w", err))
				}
			}
		}
	}
}

// EventRouter routes events to appropriate handlers based on event type
type EventRouter struct {
	handlers map[EventType][]EventHandler
	mu       sync.RWMutex
}

// NewEventRouter creates a new event router
func NewEventRouter() *EventRouter {
	return &EventRouter{
		handlers: make(map[EventType][]EventHandler),
	}
}

// Register registers a handler for a specific event type
func (r *EventRouter) Register(eventType EventType, handler EventHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[eventType] = append(r.handlers[eventType], handler)
}

// Route routes an event to its handlers
func (r *EventRouter) Route(ctx context.Context, event Event) error {
	r.mu.RLock()
	handlers, ok := r.handlers[event.Type]
	r.mu.RUnlock()

	if !ok {
		return nil // No handlers for this event type
	}

	var errors []error
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("handler errors: %v", errors)
	}

	return nil
}

// AsHandler returns an EventHandler that routes events
func (r *EventRouter) AsHandler() EventHandler {
	return func(ctx context.Context, event Event) error {
		return r.Route(ctx, event)
	}
}

// DeadLetterQueue handles failed messages
type DeadLetterQueue struct {
	client   *KafkaClient
	dlqTopic string
}

// NewDeadLetterQueue creates a new DLQ handler
func NewDeadLetterQueue(client *KafkaClient, dlqTopic string) *DeadLetterQueue {
	return &DeadLetterQueue{
		client:   client,
		dlqTopic: dlqTopic,
	}
}

// Send sends a failed message to the DLQ
func (d *DeadLetterQueue) Send(ctx context.Context, originalTopic string, event Event, err error) error {
	dlqMessage := map[string]interface{}{
		"original_topic": originalTopic,
		"event":          event,
		"error":          err.Error(),
		"failed_at":      time.Now().UTC(),
	}

	dlqBytes, marshalErr := json.Marshal(dlqMessage)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal DLQ message: %w", marshalErr)
	}

	producer := d.client.GetProducer(d.dlqTopic)
	return producer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(event.ID),
		Value: dlqBytes,
		Headers: []kafka.Header{
			{Key: "original_topic", Value: []byte(originalTopic)},
			{Key: "event_id", Value: []byte(event.ID)},
			{Key: "error", Value: []byte(err.Error())},
		},
	})
}

// RetryableConsumer wraps a consumer with retry logic
type RetryableConsumer struct {
	consumer     *EventConsumer
	dlq          *DeadLetterQueue
	maxRetries   int
	retryBackoff time.Duration
}

// NewRetryableConsumer creates a consumer with retry capability
func NewRetryableConsumer(consumer *EventConsumer, dlq *DeadLetterQueue, maxRetries int, retryBackoff time.Duration) *RetryableConsumer {
	return &RetryableConsumer{
		consumer:     consumer,
		dlq:          dlq,
		maxRetries:   maxRetries,
		retryBackoff: retryBackoff,
	}
}

// SubscribeWithRetry subscribes with automatic retry
func (r *RetryableConsumer) SubscribeWithRetry(topic string, handler EventHandler) {
	wrappedHandler := func(ctx context.Context, event Event) error {
		var lastErr error
		for i := 0; i <= r.maxRetries; i++ {
			if i > 0 {
				time.Sleep(r.retryBackoff * time.Duration(i))
			}

			lastErr = handler(ctx, event)
			if lastErr == nil {
				return nil
			}

			log.Printf("Retry %d/%d for event %s failed: %v", i, r.maxRetries, event.ID, lastErr)
		}

		// Send to DLQ after all retries exhausted
		if r.dlq != nil {
			if err := r.dlq.Send(ctx, topic, event, lastErr); err != nil {
				log.Printf("Failed to send to DLQ: %v", err)
			}
		}

		return lastErr
	}

	r.consumer.Subscribe(topic, wrappedHandler)
}

// BatchConsumer processes messages in batches
type BatchConsumer struct {
	client      *KafkaClient
	topic       string
	batchSize   int
	batchTimeout time.Duration
	handler     func(ctx context.Context, events []Event) error
}

// NewBatchConsumer creates a new batch consumer
func NewBatchConsumer(client *KafkaClient, topic string, batchSize int, batchTimeout time.Duration, handler func(ctx context.Context, events []Event) error) *BatchConsumer {
	return &BatchConsumer{
		client:       client,
		topic:        topic,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
		handler:      handler,
	}
}

// Start starts batch consuming
func (b *BatchConsumer) Start(ctx context.Context) error {
	reader := b.client.GetConsumer(b.topic)

	batch := make([]Event, 0, b.batchSize)
	messages := make([]kafka.Message, 0, b.batchSize)
	ticker := time.NewTicker(b.batchTimeout)
	defer ticker.Stop()

	processBatch := func() error {
		if len(batch) == 0 {
			return nil
		}

		if err := b.handler(ctx, batch); err != nil {
			return err
		}

		if err := reader.CommitMessages(ctx, messages...); err != nil {
			return err
		}

		batch = batch[:0]
		messages = messages[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return processBatch()
		case <-ticker.C:
			if err := processBatch(); err != nil {
				log.Printf("Error processing batch: %v", err)
			}
		default:
			// Set a short timeout for fetching
			fetchCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
			msg, err := reader.FetchMessage(fetchCtx)
			cancel()

			if err != nil {
				if ctx.Err() != nil {
					return processBatch()
				}
				continue
			}

			var event Event
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				log.Printf("Failed to unmarshal event: %v", err)
				continue
			}

			batch = append(batch, event)
			messages = append(messages, msg)

			if len(batch) >= b.batchSize {
				if err := processBatch(); err != nil {
					log.Printf("Error processing batch: %v", err)
				}
			}
		}
	}
}
