package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	gcppubsub "cloud.google.com/go/pubsub"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type BuildRequest struct {
	DeployID    string `json:"deploy_id"`
	ProjectName string `json:"project_name"`
	RepoURL     string `json:"repo_url"`
	CommitSHA   string `json:"commit_sha"`
	ImageURI    string `json:"image_uri"`
}

type Publisher struct {
	topic  *gcppubsub.Topic
	arRepo string
}

func NewPublisher(ctx context.Context, gcpProject, topicName, arRepo string) (*Publisher, error) {
	client, err := gcppubsub.NewClient(ctx, gcpProject)
	if err != nil {
		return nil, err
	}

	return &Publisher{
		topic:  client.Topic(topicName),
		arRepo: arRepo,
	}, nil
}

func (p *Publisher) PublishBuildRequest(ctx context.Context, req BuildRequest) error {
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal build request: %w", err)
	}

	attrs := make(map[string]string)
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(attrs))

	result := p.topic.Publish(ctx, &gcppubsub.Message{Data: data, Attributes: attrs})
	if _, err := result.Get(ctx); err != nil {
		return fmt.Errorf("publish build request: %w", err)
	}

	return nil
}

var invalidChars = regexp.MustCompile(`[^a-z0-9-]`)

func (p *Publisher) ImageURI(projectName, commitSHA string) string {
	name := strings.ToLower(projectName)
	name = invalidChars.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-")
	return fmt.Sprintf("%s/%s:%s", p.arRepo, name, commitSHA)
}

func (p *Publisher) Stop() {
	p.topic.Stop()
}
