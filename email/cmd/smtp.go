package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     string
	From     string
	Password string
}

type EmailMessage struct {
	To       string `json:"to"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Severity string `json:"severity"`
	DeployID string `json:"deploy_id"`
	Project  string `json:"project"`
	SentAt   int64  `json:"sent_at"`
}

type templateData struct {
	Project  string
	Severity string
	Message  string
	DeployID string
	Time     string
	Color    string
}

var emailTemplate = template.Must(template.New("email").Parse(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
</head>
<body style="margin:0;padding:0;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;background:#f4f4f7">
  <table width="100%" cellpadding="0" cellspacing="0" style="padding:40px 0">
    <tr>
      <td align="center">
        <table width="560" cellpadding="0" cellspacing="0" style="background:#fff;border-radius:8px;overflow:hidden;box-shadow:0 2px 8px rgba(0,0,0,0.08)">
          <tr>
            <td style="background:{{.Color}};padding:24px 32px">
              <span style="color:#fff;font-size:14px;font-weight:600;text-transform:uppercase;letter-spacing:1px">{{.Severity}}</span>
            </td>
          </tr>
          <tr>
            <td style="padding:32px">
              <h2 style="margin:0 0 8px;color:#1a1a2e;font-size:20px">{{.Project}}</h2>
              <p style="margin:0 0 24px;color:#4a4a68;font-size:15px;line-height:1.6">{{.Message}}</p>
              <table cellpadding="0" cellspacing="0" style="background:#f8f8fc;border-radius:6px;width:100%">
                <tr>
                  <td style="padding:16px">
                    <p style="margin:0 0 4px;color:#8888a0;font-size:12px;text-transform:uppercase">Deployment</p>
                    <p style="margin:0;color:#1a1a2e;font-size:13px;font-family:monospace">{{.DeployID}}</p>
                  </td>
                </tr>
                <tr>
                  <td style="padding:0 16px 16px">
                    <p style="margin:0 0 4px;color:#8888a0;font-size:12px;text-transform:uppercase">Time</p>
                    <p style="margin:0;color:#1a1a2e;font-size:13px">{{.Time}}</p>
                  </td>
                </tr>
              </table>
            </td>
          </tr>
          <tr>
            <td style="padding:16px 32px;background:#f8f8fc;border-top:1px solid #ececf0">
              <p style="margin:0;color:#8888a0;font-size:12px">Bifrost Platform — Automated Alert</p>
            </td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`))

func severityColor(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "#dc3545"
	case "warning":
		return "#f59e0b"
	default:
		return "#3b82f6"
	}
}

func renderEmail(msg EmailMessage) (string, error) {
	sentAt := time.Unix(msg.SentAt, 0).UTC().Format("2006-01-02 15:04:05 UTC")

	data := templateData{
		Project:  msg.Project,
		Severity: strings.ToUpper(msg.Severity),
		Message:  msg.Body,
		DeployID: msg.DeployID,
		Time:     sentAt,
		Color:    severityColor(msg.Severity),
	}

	var buf bytes.Buffer
	if err := emailTemplate.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func sendEmail(cfg SMTPConfig, msg EmailMessage) error {
	html, err := renderEmail(msg)
	if err != nil {
		return fmt.Errorf("render template: %w", err)
	}

	headers := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"utf-8\"\r\n\r\n",
		cfg.From, msg.To, msg.Subject,
	)

	var auth smtp.Auth
	if cfg.Password != "" {
		auth = smtp.PlainAuth("", cfg.From, cfg.Password, cfg.Host)
	}

	addr := cfg.Host + ":" + cfg.Port
	if err := smtp.SendMail(addr, auth, cfg.From, []string{msg.To}, []byte(headers+html)); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	slog.Info("email sent", "to", msg.To, "subject", msg.Subject)
	return nil
}
