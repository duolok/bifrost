package events

import (
	"context"
	pb "duolok/bifrost/gateway/internal/events/pb"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Emitter struct {
	client pb.EventIngressClient
	conn   *grpc.ClientConn
}

func NewEmitter(addr string) (*Emitter, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()))

	if err != nil {
		return nil, err
	}

	return &Emitter{
		client: pb.NewEventIngressClient(conn),
		conn:   conn,
	}, nil
}

func (e *Emitter) Close() error {
	return e.conn.Close()
}

func (e *Emitter) Emit(eventType, deployID, projectName, message string) {
	if e == nil {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := e.client.SendEvent(ctx, &pb.PlatformEvent{
			EventType:   eventType,
			DeployId:    deployID,
			ProjectName: projectName,
			Actor:       "gateway",
			Message:     message,
			Timestamp:   time.Now().Unix(),
		})

		if err != nil {
			slog.Warn("failed to emit event", "event_type", eventType, "deploy_id", deployID, "error", err)
		}
	}()
}
