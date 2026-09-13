package messaging

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// KafkaConfig holds Kafka connection configuration
type KafkaConfig struct {
	Brokers        []string
	ConsumerGroup  string
	Username       string
	Password       string
	TLSEnabled     bool
	SASLMechanism  string // "PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"
	ClientID       string
	SessionTimeout time.Duration
	RebalanceTimeout time.Duration
}

// KafkaClient manages Kafka connections and operations
type KafkaClient struct {
	config    *KafkaConfig
	dialer    *kafka.Dialer
	producers map[string]*kafka.Writer
	consumers map[string]*kafka.Reader
	mu        sync.RWMutex
}

// Topic definitions for NPDMS events
const (
	TopicFIRCreated        = "npdms.fir.created"
	TopicFIRUpdated        = "npdms.fir.updated"
	TopicFIRStatusChanged  = "npdms.fir.status_changed"
	TopicCaseCreated       = "npdms.case.created"
	TopicCaseUpdated       = "npdms.case.updated"
	TopicCaseStatusChanged = "npdms.case.status_changed"
	TopicEvidenceAdded     = "npdms.evidence.added"
	TopicEvidenceTransferred = "npdms.evidence.transferred"
	TopicAlertCreated      = "npdms.alert.created"
	TopicAlertAcknowledged = "npdms.alert.acknowledged"
	TopicWarrantIssued     = "npdms.warrant.issued"
	TopicWarrantExecuted   = "npdms.warrant.executed"
	TopicBailGranted       = "npdms.bail.granted"
	TopicBailDenied        = "npdms.bail.denied"
	TopicChallanIssued     = "npdms.traffic.challan_issued"
	TopicChallanPaid       = "npdms.traffic.challan_paid"
	TopicAuditLog          = "npdms.audit.log"
	TopicNotifications     = "npdms.notifications"
	TopicAnalytics         = "npdms.analytics"
)

// AllTopics returns all NPDMS topics
func AllTopics() []string {
	return []string{
		TopicFIRCreated,
		TopicFIRUpdated,
		TopicFIRStatusChanged,
		TopicCaseCreated,
		TopicCaseUpdated,
		TopicCaseStatusChanged,
		TopicEvidenceAdded,
		TopicEvidenceTransferred,
		TopicAlertCreated,
		TopicAlertAcknowledged,
		TopicWarrantIssued,
		TopicWarrantExecuted,
		TopicBailGranted,
		TopicBailDenied,
		TopicChallanIssued,
		TopicChallanPaid,
		TopicAuditLog,
		TopicNotifications,
		TopicAnalytics,
	}
}

// NewKafkaClient creates a new Kafka client
func NewKafkaClient(config *KafkaConfig) (*KafkaClient, error) {
	dialer, err := createDialer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dialer: %w", err)
	}

	client := &KafkaClient{
		config:    config,
		dialer:    dialer,
		producers: make(map[string]*kafka.Writer),
		consumers: make(map[string]*kafka.Reader),
	}

	return client, nil
}

func createDialer(config *KafkaConfig) (*kafka.Dialer, error) {
	dialer := &kafka.Dialer{
		Timeout:   10 * time.Second,
		DualStack: true,
		ClientID:  config.ClientID,
	}

	// Configure TLS if enabled
	if config.TLSEnabled {
		dialer.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// Configure SASL authentication
	if config.Username != "" && config.Password != "" {
		switch config.SASLMechanism {
		case "PLAIN":
			dialer.SASLMechanism = plain.Mechanism{
				Username: config.Username,
				Password: config.Password,
			}
		case "SCRAM-SHA-256":
			mechanism, err := scram.Mechanism(scram.SHA256, config.Username, config.Password)
			if err != nil {
				return nil, fmt.Errorf("failed to create SCRAM-SHA-256 mechanism: %w", err)
			}
			dialer.SASLMechanism = mechanism
		case "SCRAM-SHA-512":
			mechanism, err := scram.Mechanism(scram.SHA512, config.Username, config.Password)
			if err != nil {
				return nil, fmt.Errorf("failed to create SCRAM-SHA-512 mechanism: %w", err)
			}
			dialer.SASLMechanism = mechanism
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %s", config.SASLMechanism)
		}
	}

	return dialer, nil
}

// CreateTopics creates all required topics
func (c *KafkaClient) CreateTopics(ctx context.Context) error {
	conn, err := kafka.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("failed to get controller: %w", err)
	}

	controllerConn, err := kafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("failed to connect to controller: %w", err)
	}
	defer controllerConn.Close()

	topicConfigs := make([]kafka.TopicConfig, 0, len(AllTopics()))
	for _, topic := range AllTopics() {
		topicConfigs = append(topicConfigs, kafka.TopicConfig{
			Topic:             topic,
			NumPartitions:     3,
			ReplicationFactor: 1, // Increase in production
		})
	}

	err = controllerConn.CreateTopics(topicConfigs...)
	if err != nil {
		log.Printf("Warning: Some topics may already exist: %v", err)
	}

	return nil
}

// GetProducer returns a producer for the given topic
func (c *KafkaClient) GetProducer(topic string) *kafka.Writer {
	c.mu.RLock()
	if producer, ok := c.producers[topic]; ok {
		c.mu.RUnlock()
		return producer
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if producer, ok := c.producers[topic]; ok {
		return producer
	}

	producer := &kafka.Writer{
		Addr:         kafka.TCP(c.config.Brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    100,
		BatchTimeout: 10 * time.Millisecond,
		Async:        false,
		Transport: &kafka.Transport{
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return c.dialer.DialContext(ctx, network, address)
			},
		},
	}

	c.producers[topic] = producer
	return producer
}

// GetConsumer returns a consumer for the given topic
func (c *KafkaClient) GetConsumer(topic string) *kafka.Reader {
	c.mu.RLock()
	if consumer, ok := c.consumers[topic]; ok {
		c.mu.RUnlock()
		return consumer
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if consumer, ok := c.consumers[topic]; ok {
		return consumer
	}

	consumer := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        c.config.Brokers,
		Topic:          topic,
		GroupID:        c.config.ConsumerGroup,
		Dialer:         c.dialer,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		MaxWait:        500 * time.Millisecond,
		StartOffset:    kafka.LastOffset,
		CommitInterval: time.Second,
	})

	c.consumers[topic] = consumer
	return consumer
}

// Close closes all producers and consumers
func (c *KafkaClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errors []error

	for topic, producer := range c.producers {
		if err := producer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close producer for %s: %w", topic, err))
		}
	}

	for topic, consumer := range c.consumers {
		if err := consumer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close consumer for %s: %w", topic, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors closing Kafka client: %v", errors)
	}

	return nil
}

// HealthCheck checks if Kafka is reachable
func (c *KafkaClient) HealthCheck(ctx context.Context) error {
	conn, err := c.dialer.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	_, err = conn.Brokers()
	if err != nil {
		return fmt.Errorf("failed to get brokers: %w", err)
	}

	return nil
}

// ListTopics returns all topics in the cluster
func (c *KafkaClient) ListTopics(ctx context.Context) ([]string, error) {
	conn, err := c.dialer.DialContext(ctx, "tcp", c.config.Brokers[0])
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Kafka: %w", err)
	}
	defer conn.Close()

	partitions, err := conn.ReadPartitions()
	if err != nil {
		return nil, fmt.Errorf("failed to read partitions: %w", err)
	}

	topicSet := make(map[string]bool)
	for _, p := range partitions {
		topicSet[p.Topic] = true
	}

	topics := make([]string, 0, len(topicSet))
	for topic := range topicSet {
		topics = append(topics, topic)
	}

	return topics, nil
}
