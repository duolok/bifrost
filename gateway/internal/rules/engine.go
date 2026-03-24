package rules

import (
	"duolok/bifrost/gateway/internal/notify"
	"log/slog"
)

type Engine struct {
	notifier *notify.Publisher
}

type HealthEvent struct {
	DeployID       string
	Project        string
	Healthy        bool
	ResponseTimeMs int
	CpuPercent     float64
	MemoryPercent  float64
	UnhealthyCount int
}

type Action struct {
	Type     string
	Severity string
	Message  string
	To       string
}

func NewEngine(notifier *notify.Publisher) *Engine {
	return &Engine{
		notifier: notifier,
	}
}

func (e *Engine) Evaluate(event HealthEvent, script string) []Action {
	L := newLuaState(event)
	defer L.Close()

	if err := L.DoString(script); err != nil {
		slog.Warn("lua script error", "project", event.Project, "error", err)
		return nil
	}

	return collectActions(L)
}
