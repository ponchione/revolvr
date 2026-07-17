package app

import (
	"os"

	"revolvr/internal/autonomousnotification"
)

type NotificationEvidence struct {
	Intent  autonomousnotification.Intent
	Payload autonomousnotification.Payload
	Journal autonomousnotification.Journal
}

func ListNotifications(cfg Config) ([]autonomousnotification.Summary, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return nil, err
	}
	return autonomousnotification.List(paths.WorkDir)
}

func ShowNotification(cfg Config, deliveryID string) (NotificationEvidence, error) {
	paths, err := resolveStatePaths(cfg.WorkDir)
	if err != nil {
		return NotificationEvidence{}, err
	}
	intent, payload, journal, found, err := autonomousnotification.Inspect(paths.WorkDir, deliveryID)
	if err != nil {
		return NotificationEvidence{}, err
	}
	if !found {
		return NotificationEvidence{}, os.ErrNotExist
	}
	return NotificationEvidence{Intent: intent, Payload: payload, Journal: journal}, nil
}
