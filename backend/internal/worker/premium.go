package worker

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/observability"
)

// premiumCheckInterval is how often the worker scans for lapsed Premium
// grants. Hourly is plenty — a month-plan expiring is not time-critical
// to the minute, and it keeps the query load trivial.
const premiumCheckInterval = 1 * time.Hour

// PremiumWorker downgrades users whose time-limited Premium has lapsed.
// Lifetime grants (premium_expires_at IS NULL) are never touched.
type PremiumWorker struct {
	db  *gorm.DB
	bot *tgbot.Bot // may be nil in test mode — messages are then skipped
}

func NewPremiumWorker(db *gorm.DB, bot *tgbot.Bot) *PremiumWorker {
	return &PremiumWorker{db: db, bot: bot}
}

// Start runs the expiration check loop: once immediately on boot, then
// every premiumCheckInterval. Returns when ctx is cancelled so the
// graceful-shutdown drain can complete.
func (w *PremiumWorker) Start(ctx context.Context) {
	log.Println("[premium-worker] starting")

	w.check(ctx)

	ticker := time.NewTicker(premiumCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Println("[premium-worker] stopped")
			return
		case <-ticker.C:
			w.check(ctx)
		}
	}
}

// premiumBatchSize streams expired users in chunks so a worker tick
// that runs after a long outage (or simply at a large scale) can't OOM
// loading the whole expired-grant set into one slice. Audit Tier-3 #4.
const premiumBatchSize = 500

// check finds every user with an active, time-limited Premium grant
// that has elapsed, downgrades them (is_donator=false,
// premium_expires_at=NULL) and DMs a localized notice. Each user is
// handled independently so one failed send/update doesn't block the
// rest of the batch.
//
// Streamed via FindInBatches — the previous unbounded `Find(&expired)`
// would materialise every lapsed grant into memory at once. Fine when
// the worker ticks hourly on a healthy DB; hazardous if it falls behind
// (deploy outage, DB maintenance window) and 50k+ Premium grants pile
// up. Now peak memory is bounded by the batch size regardless of how
// far behind we are.
func (w *PremiumWorker) check(ctx context.Context) {
	now := time.Now().UTC()

	dest := &[]model.User{}
	err := w.db.WithContext(ctx).
		Where("is_donator = ? AND premium_expires_at IS NOT NULL AND premium_expires_at < ?", true, now).
		FindInBatches(dest, premiumBatchSize, func(tx *gorm.DB, batchNum int) error {
			batch, ok := tx.Statement.Dest.(*[]model.User)
			if !ok {
				return errors.New("premium-worker: unexpected batch dest type")
			}
			for i := range *batch {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				u := &(*batch)[i]

				res := w.db.WithContext(ctx).Model(&model.User{}).
					Where("id = ? AND is_donator = ?", u.ID, true).
					Updates(map[string]interface{}{
						"is_donator":         false,
						"premium_expires_at": nil,
					})
				if res.Error != nil {
					log.Printf("[premium-worker] downgrade user=%d error: %v", u.ID, res.Error)
					observability.CaptureException(res.Error)
					continue
				}
				if res.RowsAffected == 0 {
					// Already downgraded by a racing path (e.g. admin) — skip.
					continue
				}
				log.Printf("[premium-worker] downgraded user=%d (premium expired %s)",
					u.ID, u.PremiumExpiresAt)

				w.notifyExpired(ctx, u)
			}
			return nil
		}).Error
	if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		log.Printf("[premium-worker] query/iteration error: %v", err)
		observability.CaptureException(err)
	}
}

// notifyExpired DMs the user that Premium lapsed. Best-effort — a failed
// send is logged, never retried (the next worker tick won't re-pick the
// user since they're already downgraded).
func (w *PremiumWorker) notifyExpired(ctx context.Context, u *model.User) {
	if w.bot == nil || u.TelegramID == 0 {
		return
	}
	var text string
	if strings.HasPrefix(u.Locale, "ru") {
		text = "⌛ <b>Срок Premium истёк</b>\n\n" +
			"Бесплатные лимиты снова активны. Продлите Premium в приложении, чтобы вернуть все возможности."
	} else {
		text = "⌛ <b>Your Premium has expired</b>\n\n" +
			"Free-tier limits are back in effect. Renew Premium in the app to unlock everything again."
	}
	if _, err := w.bot.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    u.TelegramID,
		Text:      text,
		ParseMode: "HTML",
	}); err != nil {
		log.Printf("[premium-worker] expiry notice send error user=%d: %v", u.ID, err)
	}
}
