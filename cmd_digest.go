// The daily digest: `quire digest` sends it now (cron-able), and the server
// schedules it itself at QUIRE_DIGEST_TIME when SMTP is configured. Quiet
// days send nothing.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jclement/quire/internal/config"
	"github.com/jclement/quire/internal/mail"
	"github.com/jclement/quire/internal/service"
)

func runDigest() error {
	cfg, svc, err := setup()
	if err != nil {
		return err
	}
	if err := svc.Index.FullScan(); err != nil {
		return err
	}
	sent, err := sendDigest(cfg, svc)
	if err != nil {
		return err
	}
	if sent {
		fmt.Println("digest sent")
	} else {
		fmt.Println("nothing to report today — no digest sent")
	}
	return nil
}

// sendDigest builds and delivers today's digest; returns false when there
// was nothing worth sending.
func sendDigest(cfg config.Config, svc *service.Service) (bool, error) {
	smtpCfg, configured := mail.FromEnv()
	if !configured {
		return false, fmt.Errorf("SMTP is not configured (set QUIRE_SMTP_HOST, QUIRE_SMTP_FROM, …)")
	}
	to := os.Getenv("QUIRE_DIGEST_TO")
	if to == "" {
		return false, fmt.Errorf("QUIRE_DIGEST_TO is not set")
	}

	today, err := svc.Today()
	if err != nil {
		return false, err
	}
	msg, empty := mail.BuildDigest(today, cfg.BaseURL)
	if empty {
		return false, nil
	}
	msg.To = to

	sender, err := mail.NewSMTP(smtpCfg)
	if err != nil {
		return false, err
	}
	return true, sender.Send(msg)
}

// scheduleDigest runs the daily send loop inside serve. digestTime is local
// "HH:MM"; returns silently if the schedule is not configured.
func scheduleDigest(ctx context.Context, cfg config.Config, svc *service.Service) {
	digestTime := os.Getenv("QUIRE_DIGEST_TIME")
	if digestTime == "" || os.Getenv("QUIRE_DIGEST_TO") == "" {
		return
	}
	if _, configured := mail.FromEnv(); !configured {
		slog.Warn("QUIRE_DIGEST_TIME set but SMTP is not configured — digest disabled")
		return
	}
	at, err := time.Parse("15:04", digestTime)
	if err != nil {
		slog.Warn("invalid QUIRE_DIGEST_TIME (want HH:MM)", "value", digestTime)
		return
	}
	slog.Info("daily digest scheduled", "at", digestTime, "to", os.Getenv("QUIRE_DIGEST_TO"))

	go func() {
		for {
			now := time.Now()
			next := time.Date(now.Year(), now.Month(), now.Day(), at.Hour(), at.Minute(), 0, 0, now.Location())
			if !next.After(now) {
				next = next.AddDate(0, 0, 1)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
				if _, err := sendDigest(cfg, svc); err != nil {
					slog.Error("daily digest failed", "err", err)
				}
			}
		}
	}()
}
