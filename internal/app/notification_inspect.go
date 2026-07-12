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
	return autonomousnotification.List(cfg.WorkDir)
}

func ShowNotification(cfg Config, deliveryID string) (NotificationEvidence, error) {
	intent, payload, journal, found, err := autonomousnotification.Inspect(cfg.WorkDir, deliveryID)
	if err != nil {
		return NotificationEvidence{}, err
	}
	if !found {
		return NotificationEvidence{}, os.ErrNotExist
	}
	return NotificationEvidence{Intent: intent, Payload: payload, Journal: journal}, nil
}
