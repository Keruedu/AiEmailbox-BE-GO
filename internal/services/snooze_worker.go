package services

import (
	"aiemailbox-be/internal/models"
	"aiemailbox-be/internal/repository"
	"context"
	"log"
	"time"
)

// StartSnoozeWorker starts a background goroutine that periodically checks for snoozed emails
// that are due and restores them to Inbox. The worker stops when ctx is done.
func StartSnoozeWorker(ctx context.Context, interval time.Duration, repo *repository.EmailRepository) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				log.Println("snooze worker: shutting down")
				return
			case <-ticker.C:
				now := time.Now()
				due, err := repo.ListSnoozedDue(ctx, now)
				if err != nil {
					log.Println("snooze worker: error listing due emails:", err)
					continue
				}
				for _, e := range due {
					// restore to inbox and clear snoozedUntil via UpdateStatus
					if err := repo.UpdateStatus(ctx, e.ID, string(models.StatusInbox)); err != nil {
						log.Println("snooze worker: failed to restore email:", e.ID, err)
					}
				}
			}
		}
	}()
}
