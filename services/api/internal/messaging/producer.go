package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

// EventType represents the type of event
type EventType string

const (
	EventTypeFIRCreated        EventType = "fir.created"
	EventTypeFIRUpdated        EventType = "fir.updated"
	EventTypeFIRStatusChanged  EventType = "fir.status_changed"
	EventTypeCaseCreated       EventType = "case.created"
	EventTypeCaseUpdated       EventType = "case.updated"
	EventTypeCaseStatusChanged EventType = "case.status_changed"
	EventTypeEvidenceAdded     EventType = "evidence.added"
	EventTypeEvidenceTransferred EventType = "evidence.transferred"
	EventTypeAlertCreated      EventType = "alert.created"
	EventTypeAlertAcknowledged EventType = "alert.acknowledged"
	EventTypeWarrantIssued     EventType = "warrant.issued"
	EventTypeWarrantExecuted   EventType = "warrant.executed"
	EventTypeBailGranted       EventType = "bail.granted"
	EventTypeBailDenied        EventType = "bail.denied"
	EventTypeChallanIssued     EventType = "traffic.challan_issued"
	EventTypeChallanPaid       EventType = "traffic.challan_paid"
	EventTypeAuditLog          EventType = "audit.log"
	EventTypeNotification      EventType = "notification"
	EventTypeAnalytics         EventType = "analytics"
)

// Event represents a domain event
type Event struct {
	ID           string                 `json:"id"`
	Type         EventType              `json:"type"`
	Source       string                 `json:"source"`
	Subject      string                 `json:"subject,omitempty"`
	Time         time.Time              `json:"time"`
	DataContentType string              `json:"datacontenttype"`
	Data         interface{}            `json:"data"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// FIREventData represents data for FIR events
type FIREventData struct {
	FIRID        uuid.UUID  `json:"fir_id"`
	FIRNumber    string     `json:"fir_number"`
	StationID    uuid.UUID  `json:"station_id"`
	Status       string     `json:"status"`
	PreviousStatus string   `json:"previous_status,omitempty"`
	CrimeType    string     `json:"crime_type"`
	CreatedBy    uuid.UUID  `json:"created_by"`
	UpdatedBy    *uuid.UUID `json:"updated_by,omitempty"`
}

// CaseEventData represents data for Case events
type CaseEventData struct {
	CaseID         uuid.UUID  `json:"case_id"`
	CaseNumber     string     `json:"case_number"`
	FIRID          uuid.UUID  `json:"fir_id"`
	StationID      uuid.UUID  `json:"station_id"`
	Status         string     `json:"status"`
	PreviousStatus string     `json:"previous_status,omitempty"`
	InvestigatorID uuid.UUID  `json:"investigator_id"`
	UpdatedBy      *uuid.UUID `json:"updated_by,omitempty"`
}

// EvidenceEventData represents data for Evidence events
type EvidenceEventData struct {
	EvidenceID    uuid.UUID `json:"evidence_id"`
	CaseID        uuid.UUID `json:"case_id"`
	EvidenceType  string    `json:"evidence_type"`
	CurrentCustody uuid.UUID `json:"current_custody"`
	PreviousCustody *uuid.UUID `json:"previous_custody,omitempty"`
	TransferReason string   `json:"transfer_reason,omitempty"`
	AddedBy       uuid.UUID `json:"added_by"`
}

// AlertEventData represents data for Alert events
type AlertEventData struct {
	AlertID      uuid.UUID  `json:"alert_id"`
	AlertType    string     `json:"alert_type"`
	Priority     string     `json:"priority"`
	Title        string     `json:"title"`
	TargetType   string     `json:"target_type"`
	TargetID     *uuid.UUID `json:"target_id,omitempty"`
	AcknowledgedBy *uuid.UUID `json:"acknowledged_by,omitempty"`
}

// WarrantEventData represents data for Warrant events
type WarrantEventData struct {
	WarrantID     uuid.UUID  `json:"warrant_id"`
	WarrantNumber string     `json:"warrant_number"`
	WarrantType   string     `json:"warrant_type"`
	CaseID        uuid.UUID  `json:"case_id"`
	SubjectName   string     `json:"subject_name"`
	IssuedBy      *uuid.UUID `json:"issued_by,omitempty"`
	ExecutedBy    *uuid.UUID `json:"executed_by,omitempty"`
}

// BailEventData represents data for Bail events
type BailEventData struct {
	BailID      uuid.UUID `json:"bail_id"`
	CaseID      uuid.UUID `json:"case_id"`
	AccusedID   uuid.UUID `json:"accused_id"`
	AccusedName string    `json:"accused_name"`
	BailType    string    `json:"bail_type"`
	Amount      float64   `json:"amount,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

// ChallanEventData represents data for Traffic Challan events
type ChallanEventData struct {
	ChallanID     uuid.UUID `json:"challan_id"`
	ChallanNumber string    `json:"challan_number"`
	VehicleNumber string    `json:"vehicle_number"`
	ViolationType string    `json:"violation_type"`
	Amount        float64   `json:"amount"`
	PaidAmount    float64   `json:"paid_amount,omitempty"`
	PaymentMethod string    `json:"payment_method,omitempty"`
	IssuedBy      uuid.UUID `json:"issued_by"`
}

// AuditEventData represents data for Audit events
type AuditEventData struct {
	UserID       *uuid.UUID `json:"user_id"`
	Action       string     `json:"action"`
	ResourceType string     `json:"resource_type"`
	ResourceID   *uuid.UUID `json:"resource_id,omitempty"`
	Description  string     `json:"description"`
	IPAddress    string     `json:"ip_address,omitempty"`
	UserAgent    string     `json:"user_agent,omitempty"`
	Success      bool       `json:"success"`
}

// NotificationEventData represents data for Notification events
type NotificationEventData struct {
	UserID      uuid.UUID              `json:"user_id"`
	Title       string                 `json:"title"`
	Message     string                 `json:"message"`
	Type        string                 `json:"type"`
	Channel     string                 `json:"channel"` // push, sms, email
	Priority    string                 `json:"priority"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// EventProducer produces events to Kafka
type EventProducer struct {
	client  *KafkaClient
	source  string
}

// NewEventProducer creates a new event producer
func NewEventProducer(client *KafkaClient, source string) *EventProducer {
	return &EventProducer{
		client:  client,
		source:  source,
	}
}

// Publish publishes an event to the appropriate topic
func (p *EventProducer) Publish(ctx context.Context, eventType EventType, data interface{}) error {
	event := Event{
		ID:              uuid.New().String(),
		Type:            eventType,
		Source:          p.source,
		Time:            time.Now().UTC(),
		DataContentType: "application/json",
		Data:            data,
	}

	return p.PublishEvent(ctx, event)
}

// PublishEvent publishes a pre-constructed event
func (p *EventProducer) PublishEvent(ctx context.Context, event Event) error {
	topic := getTopicForEventType(event.Type)
	if topic == "" {
		return fmt.Errorf("no topic mapping for event type: %s", event.Type)
	}

	eventBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	producer := p.client.GetProducer(topic)

	msg := kafka.Message{
		Key:   []byte(event.ID),
		Value: eventBytes,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(event.Type)},
			{Key: "source", Value: []byte(event.Source)},
			{Key: "content_type", Value: []byte("application/json")},
		},
		Time: event.Time,
	}

	err = producer.WriteMessages(ctx, msg)
	if err != nil {
		return fmt.Errorf("failed to publish event to %s: %w", topic, err)
	}

	return nil
}

// PublishBatch publishes multiple events
func (p *EventProducer) PublishBatch(ctx context.Context, events []Event) error {
	// Group events by topic
	eventsByTopic := make(map[string][]kafka.Message)

	for _, event := range events {
		topic := getTopicForEventType(event.Type)
		if topic == "" {
			continue
		}

		eventBytes, err := json.Marshal(event)
		if err != nil {
			continue
		}

		msg := kafka.Message{
			Key:   []byte(event.ID),
			Value: eventBytes,
			Headers: []kafka.Header{
				{Key: "event_type", Value: []byte(event.Type)},
				{Key: "source", Value: []byte(event.Source)},
				{Key: "content_type", Value: []byte("application/json")},
			},
			Time: event.Time,
		}

		eventsByTopic[topic] = append(eventsByTopic[topic], msg)
	}

	// Publish to each topic
	for topic, messages := range eventsByTopic {
		producer := p.client.GetProducer(topic)
		if err := producer.WriteMessages(ctx, messages...); err != nil {
			return fmt.Errorf("failed to publish batch to %s: %w", topic, err)
		}
	}

	return nil
}

// PublishFIRCreated publishes a FIR created event
func (p *EventProducer) PublishFIRCreated(ctx context.Context, data FIREventData) error {
	return p.Publish(ctx, EventTypeFIRCreated, data)
}

// PublishFIRUpdated publishes a FIR updated event
func (p *EventProducer) PublishFIRUpdated(ctx context.Context, data FIREventData) error {
	return p.Publish(ctx, EventTypeFIRUpdated, data)
}

// PublishFIRStatusChanged publishes a FIR status changed event
func (p *EventProducer) PublishFIRStatusChanged(ctx context.Context, data FIREventData) error {
	return p.Publish(ctx, EventTypeFIRStatusChanged, data)
}

// PublishCaseCreated publishes a Case created event
func (p *EventProducer) PublishCaseCreated(ctx context.Context, data CaseEventData) error {
	return p.Publish(ctx, EventTypeCaseCreated, data)
}

// PublishCaseUpdated publishes a Case updated event
func (p *EventProducer) PublishCaseUpdated(ctx context.Context, data CaseEventData) error {
	return p.Publish(ctx, EventTypeCaseUpdated, data)
}

// PublishCaseStatusChanged publishes a Case status changed event
func (p *EventProducer) PublishCaseStatusChanged(ctx context.Context, data CaseEventData) error {
	return p.Publish(ctx, EventTypeCaseStatusChanged, data)
}

// PublishEvidenceAdded publishes an Evidence added event
func (p *EventProducer) PublishEvidenceAdded(ctx context.Context, data EvidenceEventData) error {
	return p.Publish(ctx, EventTypeEvidenceAdded, data)
}

// PublishEvidenceTransferred publishes an Evidence transferred event
func (p *EventProducer) PublishEvidenceTransferred(ctx context.Context, data EvidenceEventData) error {
	return p.Publish(ctx, EventTypeEvidenceTransferred, data)
}

// PublishAlertCreated publishes an Alert created event
func (p *EventProducer) PublishAlertCreated(ctx context.Context, data AlertEventData) error {
	return p.Publish(ctx, EventTypeAlertCreated, data)
}

// PublishWarrantIssued publishes a Warrant issued event
func (p *EventProducer) PublishWarrantIssued(ctx context.Context, data WarrantEventData) error {
	return p.Publish(ctx, EventTypeWarrantIssued, data)
}

// PublishBailGranted publishes a Bail granted event
func (p *EventProducer) PublishBailGranted(ctx context.Context, data BailEventData) error {
	return p.Publish(ctx, EventTypeBailGranted, data)
}

// PublishChallanIssued publishes a Traffic Challan issued event
func (p *EventProducer) PublishChallanIssued(ctx context.Context, data ChallanEventData) error {
	return p.Publish(ctx, EventTypeChallanIssued, data)
}

// PublishChallanPaid publishes a Traffic Challan paid event
func (p *EventProducer) PublishChallanPaid(ctx context.Context, data ChallanEventData) error {
	return p.Publish(ctx, EventTypeChallanPaid, data)
}

// PublishAuditLog publishes an Audit log event
func (p *EventProducer) PublishAuditLog(ctx context.Context, data AuditEventData) error {
	return p.Publish(ctx, EventTypeAuditLog, data)
}

// PublishNotification publishes a Notification event
func (p *EventProducer) PublishNotification(ctx context.Context, data NotificationEventData) error {
	return p.Publish(ctx, EventTypeNotification, data)
}

func getTopicForEventType(eventType EventType) string {
	topicMap := map[EventType]string{
		EventTypeFIRCreated:          TopicFIRCreated,
		EventTypeFIRUpdated:          TopicFIRUpdated,
		EventTypeFIRStatusChanged:    TopicFIRStatusChanged,
		EventTypeCaseCreated:         TopicCaseCreated,
		EventTypeCaseUpdated:         TopicCaseUpdated,
		EventTypeCaseStatusChanged:   TopicCaseStatusChanged,
		EventTypeEvidenceAdded:       TopicEvidenceAdded,
		EventTypeEvidenceTransferred: TopicEvidenceTransferred,
		EventTypeAlertCreated:        TopicAlertCreated,
		EventTypeAlertAcknowledged:   TopicAlertAcknowledged,
		EventTypeWarrantIssued:       TopicWarrantIssued,
		EventTypeWarrantExecuted:     TopicWarrantExecuted,
		EventTypeBailGranted:         TopicBailGranted,
		EventTypeBailDenied:          TopicBailDenied,
		EventTypeChallanIssued:       TopicChallanIssued,
		EventTypeChallanPaid:         TopicChallanPaid,
		EventTypeAuditLog:            TopicAuditLog,
		EventTypeNotification:        TopicNotifications,
		EventTypeAnalytics:           TopicAnalytics,
	}

	return topicMap[eventType]
}
