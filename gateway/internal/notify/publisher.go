package notify

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	exchangeName = "bifrost.notifications"
	queueName    = "bifrost.email"
	routingKey   = "email"
)

type EmailMessage struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
	DeployID string `json:"deploy_id,omitempty"`
	Project  string `json:"project,omitempty"`
	SentAt   int64  `json:"sent_at"`
}

type Publisher struct {
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(url string) (*Publisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("rabbitmq connect: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("rabbitmq channel: %w", err)
	}

	// Declare exchange — topic type allows routing by key
	if err := ch.ExchangeDeclare(exchangeName, "topic", true, false, false, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq exchange: %w", err)
	}

	// Declare and bind the email queue
	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq queue: %w", err)
	}

	if err := ch.QueueBind(q.Name, routingKey, exchangeName, false, nil); err != nil {
		ch.Close()
		conn.Close()
		return nil, fmt.Errorf("rabbitmq bind: %w", err)
	}

	slog.Info("rabbitmq publisher connected", "exchange", exchangeName, "queue", queueName)
	return &Publisher{conn: conn, ch: ch}, nil
}

func (p *Publisher) Close() error {
	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

func (p *Publisher) PublishEmail(msg EmailMessage) {
	if p == nil {
		return
	}

	msg.SentAt = time.Now().Unix()

	body, err := json.Marshal(msg)
	if err != nil {
		slog.Warn("failed to marshal email message", "error", err)
		return
	}

	err = p.ch.Publish(exchangeName, routingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})
	if err != nil {
		slog.Warn("failed to publish email", "to", msg.To, "error", err)
		return
	}

	slog.Info("notification queued", "to", msg.To, "subject", msg.Subject, "severity", msg.Severity)
}
