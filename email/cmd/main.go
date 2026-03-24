package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	amqp "github.com/rabbitmq/amqp091-go"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg := loadConfig()
	ch := connectRabbitMQ(cfg.RabbitMQURL)
	defer ch.Close()

	msgs := subscribe(ch)

	slog.Info("email worker started", "smtp", cfg.SMTP.Host+":"+cfg.SMTP.Port)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go consume(msgs, cfg.SMTP)

	<-quit
	slog.Info("email worker shutting down")
}

type Config struct {
	RabbitMQURL string
	SMTP        SMTPConfig
}

func loadConfig() Config {
	return Config{
		RabbitMQURL: envOr("BF_RABBITMQ_URL", "amqp://bifrost:localdev@localhost:5672/"),
		SMTP: SMTPConfig{
			Host:     envOr("BF_SMTP_HOST", "localhost"),
			Port:     envOr("BF_SMTP_PORT", "1025"),
			From:     envOr("BF_SMTP_FROM", "bifrost@bifrost.dev"),
			Password: os.Getenv("BF_SMTP_PASSWORD"),
		},
	}
}

func connectRabbitMQ(url string) *amqp.Channel {
	conn, err := amqp.Dial(url)
	if err != nil {
		slog.Error("rabbitmq connection failed", "error", err)
		os.Exit(1)
	}

	ch, err := conn.Channel()
	if err != nil {
		slog.Error("rabbitmq channel failed", "error", err)
		os.Exit(1)
	}

	return ch
}

func subscribe(ch *amqp.Channel) <-chan amqp.Delivery {
	ch.ExchangeDeclare("bifrost.notifications", "topic", true, false, false, false, nil)

	q, err := ch.QueueDeclare("bifrost.email", true, false, false, false, nil)
	if err != nil {
		slog.Error("queue declare failed", "error", err)
		os.Exit(1)
	}

	ch.QueueBind(q.Name, "email", "bifrost.notifications", false, nil)

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	if err != nil {
		slog.Error("consume failed", "error", err)
		os.Exit(1)
	}

	return msgs
}

func consume(msgs <-chan amqp.Delivery, smtp SMTPConfig) {
	for msg := range msgs {
		var email EmailMessage
		if err := json.Unmarshal(msg.Body, &email); err != nil {
			slog.Warn("invalid message, discarding", "error", err)
			msg.Nack(false, false)
			continue
		}

		slog.Info("processing", "to", email.To, "severity", email.Severity, "project", email.Project)

		if err := sendEmail(smtp, email); err != nil {
			slog.Error("send failed, requeuing", "to", email.To, "error", err)
			msg.Nack(false, true)
			continue
		}

		msg.Ack(false)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
