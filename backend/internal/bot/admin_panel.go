package bot

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
	"github.com/subguard/backend/internal/workerutil"
)

// ── FSM state constants ────────────────────────────────
const (
	stateNone            = ""
	stateAwaitUserID     = "await_user_id"
	stateAwaitOfferTitle = "await_offer_title"
	stateAwaitOfferDesc  = "await_offer_desc"
	stateAwaitOfferBadge = "await_offer_badge"
	stateAwaitOfferURL   = "await_offer_url"
	stateAwaitOfferIcon  = "await_offer_icon"
	stateAwaitTrafficTag = "await_traffic_tag"
)

// fsmKeyPrefix is the Redis key prefix for admin FSM state.
const fsmKeyPrefix = "admin_state:"

// fsmDataPrefix stores partial data during multi-step flows (JSON-ish).
const fsmDataPrefix = "admin_data:"

// fsmTTL caps how long an FSM state lives. After 1 hour of inactivity the
// state auto-expires — prevents zombie states from confused admins.
const fsmTTL = 1 * time.Hour

// adminPanel handles all in-bot admin commands and callbacks.
//
// appCtx and wg are propagated to every spawned background goroutine
// (broadcast worker, async export, etc.) so SIGTERM correctly cancels
// them and main.go's workerWG drain blocks until they exit. Without
// these the goroutines would inherit context.Background() and continue
// writing into a closing DB/Redis pool.
type adminPanel struct {
	cfg       *config.Config
	db        *gorm.DB
	rdb       *redis.Client
	repo      *repository.AdminRepo
	broadcast *broadcastHandler
	appCtx    context.Context
	wg        *sync.WaitGroup
}

func newAdminPanel(cfg *config.Config, db *gorm.DB, rdb *redis.Client, appCtx context.Context, wg *sync.WaitGroup) *adminPanel {
	if appCtx == nil {
		appCtx = context.Background()
	}
	p := &adminPanel{
		cfg:    cfg,
		db:     db,
		rdb:    rdb,
		repo:   repository.NewAdminRepo(db),
		appCtx: appCtx,
		wg:     wg,
	}
	p.broadcast = newBroadcastHandler(p)
	return p
}

// ── FSM helpers ────────────────────────────────────────
//
// All FSM mutators below pipeline their Redis ops so a state+data
// pair lands as a single round-trip and concurrent admin clicks
// (e.g. user double-taps a button before the first command's reply
// renders) can't interleave a "set state for flow A" with a "set
// data for flow B" across the network. The Pipeline DOES NOT give
// us atomicity in the MULTI/EXEC sense — another client could read
// between the two writes — but the only reader is THIS admin's own
// follow-up message, which is serialized at the Telegram bot
// dispatch layer (one update at a time per chat). The pipelining
// just collapses the network cost. Audit Tier-4 #4.

func (p *adminPanel) setState(ctx context.Context, tgID int64, state string) {
	idStr := strconv.FormatInt(tgID, 10)
	stateKey := fsmKeyPrefix + idStr
	dataKey := fsmDataPrefix + idStr
	if state == stateNone {
		p.rdb.Del(ctx, stateKey)
		return
	}
	pipe := p.rdb.Pipeline()
	pipe.Set(ctx, stateKey, state, fsmTTL)
	// Mirror the TTL onto the data key so a multi-step flow that only
	// writes data between state transitions doesn't let the state expire
	// underneath it.
	pipe.Expire(ctx, dataKey, fsmTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[admin-fsm] setState pipeline tg=%d: %v", tgID, err)
	}
}

func (p *adminPanel) getState(ctx context.Context, tgID int64) string {
	key := fsmKeyPrefix + strconv.FormatInt(tgID, 10)
	val, err := p.rdb.Get(ctx, key).Result()
	if err != nil {
		return stateNone
	}
	return val
}

// setData stores partial-flow data AND refreshes the state-key TTL. Long
// multi-step admin flows (broadcast composition, offer creation) used to
// expire mid-flow if the user paused near the 1h boundary because each
// setData call only touched the data key — see audit A1.
func (p *adminPanel) setData(ctx context.Context, tgID int64, data string) {
	idStr := strconv.FormatInt(tgID, 10)
	pipe := p.rdb.Pipeline()
	pipe.Set(ctx, fsmDataPrefix+idStr, data, fsmTTL)
	pipe.Expire(ctx, fsmKeyPrefix+idStr, fsmTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[admin-fsm] setData pipeline tg=%d: %v", tgID, err)
	}
}

func (p *adminPanel) getData(ctx context.Context, tgID int64) string {
	key := fsmDataPrefix + strconv.FormatInt(tgID, 10)
	val, _ := p.rdb.Get(ctx, key).Result()
	return val
}

// clearState wipes both the FSM state and any accumulated data.
// Use this for explicit resets (/cancel, /admin, admin_back).
// For intermediate transitions (e.g. URL step → waiting for lang callback)
// use setState(stateNone) which preserves data.
func (p *adminPanel) clearState(ctx context.Context, tgID int64) {
	idStr := strconv.FormatInt(tgID, 10)
	pipe := p.rdb.Pipeline()
	pipe.Del(ctx, fsmKeyPrefix+idStr)
	pipe.Del(ctx, fsmDataPrefix+idStr)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Printf("[admin-fsm] clearState pipeline tg=%d: %v", tgID, err)
	}
}

// ── /admin — Main Menu ─────────────────────────────────

func (p *adminPanel) handleAdminCommand(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	tgID := update.Message.From.ID
	if !p.cfg.IsAdmin(tgID) {
		return
	}
	p.clearState(ctx, tgID)
	p.sendMainMenu(ctx, b, update.Message.Chat.ID, 0)
}

// sendMainMenu sends or edits the admin main menu.
// If editMsgID > 0 it edits the existing message; otherwise sends a new one.
func (p *adminPanel) sendMainMenu(ctx context.Context, b *tgbot.Bot, chatID int64, editMsgID int) {
	text := "🛠 *Панель администратора*\nВыберите раздел:"
	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "📊 Статистика", CallbackData: "admin_stats"},
				{Text: "👤 Пользователи", CallbackData: "admin_users"},
			},
			{
				{Text: "💰 Спонсоры", CallbackData: "admin_sponsors"},
				{Text: "🔗 Трафик", CallbackData: "admin_traffic"},
			},
			{
				{Text: "📢 Рассылка", CallbackData: "admin_broadcast"},
				{Text: "⚙ Настройки", CallbackData: "admin_settings"},
			},
		},
	}

	if editMsgID > 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   editMsgID,
			Text:        text,
			ParseMode:   "Markdown",
			ReplyMarkup: &kb,
		})
	} else {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        text,
			ParseMode:   "Markdown",
			ReplyMarkup: &kb,
		})
	}
}

// ── /cancel — Reset FSM ────────────────────────────────

func (p *adminPanel) handleCancel(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}
	tgID := update.Message.From.ID
	if !p.cfg.IsAdmin(tgID) {
		return
	}
	p.clearState(ctx, tgID)
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "❌ Действие отменено. Введите /admin для меню.",
	})
}

// ── Callback Router ────────────────────────────────────

func (p *adminPanel) handleCallback(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	cb := update.CallbackQuery
	if cb == nil {
		return
	}
	if !p.cfg.IsAdmin(cb.From.ID) {
		b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
			CallbackQueryID: cb.ID, Text: "⛔ Admin only",
		})
		return
	}

	// Always ack the callback to remove the loading spinner. A small
	// number of callbacks need a toast text instead of a silent ack —
	// e.g. "⏳ Формирую файл..." so the admin sees feedback while the
	// CSV is generated. Add new entries here when needed.
	ackText := ""
	switch cb.Data {
	case "admin_export_csv":
		ackText = "⏳ Формирую файл..."
	case "admin_toggle_paywall":
		ackText = "✅ Статус пейвола изменён!"
	case "admin_toggle_maintenance", "admin_toggle_notifications":
		ackText = "✅ Статус изменён!"
	}
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{
		CallbackQueryID: cb.ID,
		Text:            ackText,
	})

	chatID := cb.From.ID // DM, so chatID == user ID
	msgID := 0
	// cb.Message is a value-typed MaybeInaccessibleMessage; the inner
	// .Message pointer is nil when the callback came from an
	// inline_message_id (forwarded button / inline-mode result) — the
	// existing nil-check catches that. On miss we fall back to the
	// admin's own DM chat which is always safe.
	if cb.Message.Message != nil {
		chatID = cb.Message.Message.Chat.ID
		msgID = cb.Message.Message.ID
	}

	data := cb.Data
	switch {
	case data == "admin_back":
		p.clearState(ctx, cb.From.ID)
		p.sendMainMenu(ctx, b, chatID, msgID)

	case data == "admin_stats" || strings.HasPrefix(data, "admin_stats:"):
		p.handleStatsSection(ctx, b, chatID, msgID, data)

	case data == "admin_users":
		p.handleUsersPrompt(ctx, b, cb.From.ID, chatID, msgID)

	case data == "admin_sponsors":
		p.handleSponsorsMenu(ctx, b, chatID, msgID)

	case data == "admin_traffic":
		p.handleTrafficMenu(ctx, b, chatID, msgID)

	case data == "admin_broadcast":
		p.broadcast.handleBroadcastStart(ctx, b, chatID, msgID)

	case data == "admin_settings":
		p.handleSettingsMenu(ctx, b, chatID, msgID)

	case data == "admin_limits":
		p.handleLimitsMenu(ctx, b, chatID, msgID)

	case data == "admin_toggle_paywall":
		p.handlePaywallToggle(ctx, b, chatID, msgID)

	case data == "admin_toggle_maintenance":
		p.handleSwitchToggle(ctx, b, chatID, msgID, "maintenance")

	case data == "admin_toggle_notifications":
		p.handleSwitchToggle(ctx, b, chatID, msgID, "notifications")

	case data == "admin_noop":
		// label-only button, already acked above

	case data == "admin_subs_inc":
		p.handleLimitChange(ctx, b, chatID, msgID, "subs", 1)

	case data == "admin_subs_dec":
		p.handleLimitChange(ctx, b, chatID, msgID, "subs", -1)

	case data == "admin_rooms_inc":
		p.handleLimitChange(ctx, b, chatID, msgID, "rooms", 1)

	case data == "admin_rooms_dec":
		p.handleLimitChange(ctx, b, chatID, msgID, "rooms", -1)

	case data == "admin_prices":
		p.handlePricesMenu(ctx, b, chatID, msgID)

	case strings.HasPrefix(data, "pr_"):
		p.handlePriceCallback(ctx, b, chatID, msgID, data)

	case strings.HasPrefix(data, "admin_bc_lang_"):
		p.broadcast.handleBroadcastLang(ctx, b, cb.From.ID, data, chatID, msgID)

	case data == "admin_bc_confirm":
		p.broadcast.handleBroadcastConfirm(ctx, b, cb.From.ID, chatID, msgID)

	case data == "admin_export_csv":
		p.handleExportCSV(ctx, b, chatID, msgID)

	case data == "admin_recs_toggle":
		p.handleRecsToggle(ctx, b, chatID, msgID)

	case data == "admin_offer_add":
		p.handleOfferAddStart(ctx, b, cb.From.ID, chatID, msgID)

	case data == "admin_traffic_new":
		p.handleTrafficNewPrompt(ctx, b, cb.From.ID, chatID, msgID)

	case strings.HasPrefix(data, "admin_tv:"):
		p.handleTrafficView(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_td:"):
		p.handleTrafficDelete(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_view:"):
		p.handleOfferView(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_toggle:"):
		p.handleOfferToggle(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_del:"):
		p.handleOfferDelete(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_premium_grant:"):
		p.handlePremiumChange(ctx, b, data, true, chatID, msgID)

	case strings.HasPrefix(data, "admin_premium_revoke:"):
		p.handlePremiumChange(ctx, b, data, false, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_lang:"):
		p.handleOfferLangPick(ctx, b, cb.From.ID, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_ban:"):
		p.handleBanToggle(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_udel:"):
		p.handleUserDelete(ctx, b, data, chatID, msgID)

	default:
		log.Printf("[admin] unhandled callback: %s", data)
	}
}

// ── Text Router (FSM) ──────────────────────────────────

func (p *adminPanel) handleText(ctx context.Context, b *tgbot.Bot, update *models.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}
	tgID := update.Message.From.ID
	if !p.cfg.IsAdmin(tgID) {
		return
	}

	state := p.getState(ctx, tgID)
	if state == stateNone {
		return // not in FSM — let other handlers deal with it
	}

	chatID := update.Message.Chat.ID
	text := strings.TrimSpace(update.Message.Text)

	switch state {
	case stateAwaitUserID:
		p.handleUserLookup(ctx, b, tgID, chatID, text)

	case stateAwaitOfferTitle:
		p.setData(ctx, tgID, text) // store title
		p.setState(ctx, tgID, stateAwaitOfferDesc)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "📝 Введите *описание* оффера (или `-` чтобы пропустить):",
			ParseMode: "Markdown",
		})

	case stateAwaitOfferDesc:
		prev := p.getData(ctx, tgID)
		desc := text
		if desc == "-" {
			desc = ""
		}
		p.setData(ctx, tgID, prev+"\n"+desc)
		p.setState(ctx, tgID, stateAwaitOfferBadge)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "🏷 Введите *текст бейджа* (напр. \"Скидка 30%\" или `-` чтобы пропустить):",
			ParseMode: "Markdown",
		})

	case stateAwaitOfferBadge:
		prev := p.getData(ctx, tgID)
		badge := text
		if badge == "-" {
			badge = ""
		}
		p.setData(ctx, tgID, prev+"\n"+badge)
		p.setState(ctx, tgID, stateAwaitOfferURL)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "🔗 Введите *URL* оффера:",
			ParseMode: "Markdown",
		})

	case stateAwaitOfferURL:
		prev := p.getData(ctx, tgID)
		p.setData(ctx, tgID, prev+"\n"+text)
		p.setState(ctx, tgID, stateAwaitOfferIcon)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "🎨 Введите *иконку* оффера:\n• Домен бренда (напр. `nike.com`) — рекомендуется, тянется с Brandfetch CDN\n• Системное имя из каталога (напр. `netflix`)\n• URL на изображение (`https://...`)\n• `-` чтобы пропустить",
			ParseMode: "Markdown",
		})

	case stateAwaitOfferIcon:
		prev := p.getData(ctx, tgID)
		icon := text
		if icon == "-" {
			icon = ""
		}
		p.setData(ctx, tgID, prev+"\n"+icon)
		p.setState(ctx, tgID, stateNone) // lang is picked via inline button
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "🌍 Выберите *аудиторию*:",
			ParseMode: "Markdown",
			ReplyMarkup: &models.InlineKeyboardMarkup{
				InlineKeyboard: [][]models.InlineKeyboardButton{{
					{Text: "🇷🇺 RU", CallbackData: "admin_offer_lang:ru"},
					{Text: "🇬🇧 EN", CallbackData: "admin_offer_lang:en"},
					{Text: "🌍 Все", CallbackData: "admin_offer_lang:all"},
				}},
			},
		})

	case stateAwaitTrafficTag:
		p.handleTrafficCreate(ctx, b, tgID, chatID, text)

	case stateAwaitBroadcastMsg:
		p.broadcast.handleBroadcastContent(ctx, b, update)
	}
}

// ── Stats Module ───────────────────────────────────────

// ── Stats dashboard ─────────────────────────────────────
//
// The dashboard is four section screens (Audience / Money / Activity /
// Services), each navigable via inline buttons and scoped by a
// today / 7d / 30d period toggle. All state lives in the callback data
// — `admin_stats:<section>:<period>` — so no FSM storage is needed.

type statsPeriod struct {
	code  string
	label string
	since func(now time.Time) time.Time
}

var statsPeriods = []statsPeriod{
	{"1d", "Сегодня", func(now time.Time) time.Time {
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}},
	{"7d", "7 дней", func(now time.Time) time.Time { return now.AddDate(0, 0, -7) }},
	{"30d", "30 дней", func(now time.Time) time.Time { return now.AddDate(0, 0, -30) }},
}

const (
	defaultStatsSection = "aud"
	defaultStatsPeriod  = "7d"
)

func statsPeriodByCode(code string) statsPeriod {
	for _, pr := range statsPeriods {
		if pr.code == code {
			return pr
		}
	}
	return statsPeriods[1] // 7d
}

// parseStatsCallback splits `admin_stats[:section[:period]]` into its
// parts, defaulting to the audience section and the 7-day window.
func parseStatsCallback(data string) (section, period string) {
	section, period = defaultStatsSection, defaultStatsPeriod
	parts := strings.Split(data, ":")
	if len(parts) > 1 && parts[1] != "" {
		section = parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		period = parts[2]
	}
	return section, period
}

func (p *adminPanel) handleStatsSection(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int, data string) {
	section, periodCode := parseStatsCallback(data)
	period := statsPeriodByCode(periodCode)
	since := period.since(time.Now().UTC())

	var text string
	if section == "svc" {
		popular, err := p.repo.GetPopularServices(10, since)
		if err != nil {
			log.Printf("[admin] popular services error: %v", err)
			return
		}
		text = renderStatsServices(popular, period)
	} else {
		stats, err := p.repo.GetStats()
		if err != nil {
			log.Printf("[admin] stats error: %v", err)
			return
		}
		ps, err := p.repo.GetPeriodStats(since)
		if err != nil {
			log.Printf("[admin] period stats error: %v", err)
			return
		}
		switch section {
		case "mon":
			text = renderStatsMoney(stats, ps, period)
		case "act":
			text = renderStatsActivity(stats, ps, period)
		default:
			section = "aud"
			text = renderStatsAudience(stats, ps, period)
		}
	}

	kb := statsKeyboard(section, period.code)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func renderStatsAudience(s *repository.StatsResult, ps *repository.PeriodStats, period statsPeriod) string {
	churn, ru, en, other := 0, 0, 0, 0
	if s.TotalUsers > 0 {
		churn = int(s.ChurnedUsers * 100 / s.TotalUsers)
		ru = int(s.LocaleRU * 100 / s.TotalUsers)
		en = int(s.LocaleEN * 100 / s.TotalUsers)
		other = 100 - ru - en
	}
	var sb strings.Builder
	sb.WriteString("👥 *Аудитория* · " + period.label + "\n\n")
	sb.WriteString(fmt.Sprintf("• Всего заходило: *%d*\n", s.TotalUsers))
	sb.WriteString(fmt.Sprintf("• 🟢 Живых: *%d*\n", s.ActiveUsers))
	sb.WriteString(fmt.Sprintf("• 🔴 Отписок: *%d* (%d%%)\n", s.ChurnedUsers, churn))
	sb.WriteString(fmt.Sprintf("• Новых за период: *+%d*\n", ps.NewUsers))
	for _, src := range ps.Sources {
		icon, name := "🔗", "`"+src.Source+"`"
		if src.Source == "organic" {
			icon, name = "🌿", "Органика"
		}
		sb.WriteString(fmt.Sprintf("   ↳ %s %s: %d\n", icon, name, src.Count))
	}
	sb.WriteString(fmt.Sprintf("• DAU: %d | MAU: %d\n\n", s.DAU, s.MAU))
	sb.WriteString("🌍 *Демография*\n")
	sb.WriteString(fmt.Sprintf("• 🇷🇺 RU: %d (%d%%)\n", s.LocaleRU, ru))
	sb.WriteString(fmt.Sprintf("• 🇬🇧 EN: %d (%d%%)\n", s.LocaleEN, en))
	if s.LocaleOther > 0 {
		sb.WriteString(fmt.Sprintf("• 🌐 Other: %d (%d%%)\n", s.LocaleOther, other))
	}
	return sb.String()
}

func renderStatsMoney(s *repository.StatsResult, ps *repository.PeriodStats, period statsPeriod) string {
	var sb strings.Builder
	sb.WriteString("💎 *Монетизация* · " + period.label + "\n\n")
	sb.WriteString(fmt.Sprintf("• Всего Premium: *%d*\n", s.Donators))
	sb.WriteString(fmt.Sprintf("   ↳ 🔁 Месячных: %d\n", s.DonatorsMonthly))
	sb.WriteString(fmt.Sprintf("   ↳ ♾ Пожизненных: %d\n", s.DonatorsLifetime))
	sb.WriteString(fmt.Sprintf("• Покупок за период: *+%d*\n\n", ps.NewPurchases))
	sb.WriteString("💰 *Выручка*\n")
	sb.WriteString(fmt.Sprintf("• Всего: ⭐ %d · 💵 %s\n", s.RevenueStars, formatCents(s.RevenueCryptoCents)))
	sb.WriteString(fmt.Sprintf("• За период: ⭐ %d · 💵 %s\n", ps.RevenueStars, formatCents(ps.RevenueCryptoCents)))
	return sb.String()
}

func renderStatsActivity(s *repository.StatsResult, ps *repository.PeriodStats, period statsPeriod) string {
	var sb strings.Builder
	sb.WriteString("📋 *Активность* · " + period.label + "\n\n")
	sb.WriteString(fmt.Sprintf("• Всего подписок: *%d*\n", s.TotalSubscriptions))
	sb.WriteString(fmt.Sprintf("• Добавлено за период: *+%d*\n", ps.NewSubs))
	sb.WriteString(fmt.Sprintf("• Активных комнат: *%d*\n", s.TotalRooms))
	return sb.String()
}

func renderStatsServices(popular []repository.PopularServiceStat, period statsPeriod) string {
	var sb strings.Builder
	sb.WriteString("📈 *Популярные сервисы* · " + period.label + "\n\n")
	if len(popular) == 0 {
		sb.WriteString("_Нет данных за выбранный период._")
		return sb.String()
	}
	for i, s := range popular {
		medal := fmt.Sprintf("%d.", i+1)
		switch i {
		case 0:
			medal = "🥇"
		case 1:
			medal = "🥈"
		case 2:
			medal = "🥉"
		}
		sb.WriteString(fmt.Sprintf("%s `%s` — %d\n", medal, escapeMarkdownLite(s.Name), s.Count))
	}
	return sb.String()
}

// formatCents renders USD cents as a dollar amount, e.g. 4500 → "$45.00".
func formatCents(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

// statsKeyboard builds the period toggle + section navigation for a stats
// screen. The active period and section are prefixed with a dot.
func statsKeyboard(section, periodCode string) models.InlineKeyboardMarkup {
	periodRow := make([]models.InlineKeyboardButton, 0, len(statsPeriods))
	for _, pr := range statsPeriods {
		label := pr.label
		if pr.code == periodCode {
			label = "• " + label
		}
		periodRow = append(periodRow, models.InlineKeyboardButton{
			Text:         label,
			CallbackData: "admin_stats:" + section + ":" + pr.code,
		})
	}

	navBtn := func(code, label string) models.InlineKeyboardButton {
		if code == section {
			label = "• " + label
		}
		return models.InlineKeyboardButton{
			Text:         label,
			CallbackData: "admin_stats:" + code + ":" + periodCode,
		}
	}

	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			periodRow,
			{navBtn("aud", "👥 Аудитория"), navBtn("mon", "💎 Монетизация")},
			{navBtn("act", "📋 Активность"), navBtn("svc", "📈 Сервисы")},
			{{Text: "📥 Экспорт в CSV", CallbackData: "admin_export_csv"}},
			{{Text: "🔙 Назад", CallbackData: "admin_back"}},
		},
	}
}

// ── CSV Export ──────────────────────────────────────────

// exportBatchSize controls how many user rows are read + flushed to the
// CSV writer per FindInBatches iteration. 1000 keeps peak memory bounded
// (a row ≈ 400 bytes → batch ≈ 400 KB) while amortising per-batch
// overhead. Bump if export latency matters more than memory headroom.
const exportBatchSize = 1000

// exportCSVLockKey is the per-admin Redis single-flight gate for CSV
// export. The export is a 10-minute job that streams the whole users
// table; two clicks (admin double-tapped the button, or Telegram retried
// the callback) used to spawn two parallel iterations both feeding the
// SAME Telegram document upload — wasted DB read + risk of two
// big-file uploads racing on the receiving end. Audit Tier-4 #1.
//
// The key is per-admin (not global) because two different admins
// legitimately running their own exports in parallel should both
// succeed. TTL covers the export's 10-minute ctx deadline.
const exportCSVLockTTL = 11 * time.Minute

func exportCSVLockKey(adminChatID int64) string {
	return fmt.Sprintf("admin_export_csv_lock:%d", adminChatID)
}

// handleExportCSV dispatches the heavy export to a background goroutine
// and returns immediately. The Telegram callback was already acked with
// a "⏳ Формирую файл..." toast by the router, so the admin sees feedback
// without us blocking the callback handler past Telegram's 30s ack
// deadline (audit C4).
//
// The goroutine is registered on workerWG so graceful shutdown waits for
// in-flight exports to finish (or hit the drain timeout) before closing
// the DB pool.
//
// Single-flight per admin: Redis SetNX gate ensures a double-click
// surfaces a "уже идёт" message instead of spawning a parallel export
// (audit Tier-4 #1). Lock TTL covers the runExportCSV 10-min ctx;
// defer-Del releases earlier on normal completion.
func (p *adminPanel) handleExportCSV(ctx context.Context, b *tgbot.Bot, chatID int64, _ int) {
	lockKey := exportCSVLockKey(chatID)
	lockAcquired := false
	if p.rdb != nil {
		acquired, err := p.rdb.SetNX(ctx, lockKey, "1", exportCSVLockTTL).Result()
		if err != nil {
			log.Printf("[admin-export-csv] redis SetNX error: %v — proceeding without lock", err)
		} else if !acquired {
			b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID: chatID,
				Text:   "⚠️ Экспорт уже идёт — дождитесь файла.",
			})
			return
		} else {
			lockAcquired = true
		}
	}

	if p.wg != nil {
		p.wg.Add(1)
	}
	go func() {
		if p.wg != nil {
			defer p.wg.Done()
		}
		if lockAcquired {
			defer func() {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := p.rdb.Del(releaseCtx, lockKey).Err(); err != nil {
					log.Printf("[admin-export-csv] lock release error: %v (TTL will reclaim)", err)
				}
			}()
		}
		workerutil.Supervise("admin-export-csv", func() {
			p.runExportCSV(b, chatID)
		})
	}()
}

// runExportCSV streams the user table to a CSV via FindInBatches so peak
// memory stays bounded regardless of total user count. The CSV bytes
// accumulate in a single bytes.Buffer (we have to materialise the full
// document anyway — Telegram's SendDocument needs an io.Reader of the
// complete file), but each batch writes 1000 rows to the csv.Writer and
// flushes, so we never hold the decoded User structs and the CSV rows
// simultaneously.
//
// Filter is intentionally wide: everyone except soft-deleted users
// (incl. banned + churned). The admin uses this dump for ad-network
// retargeting where bot-blocked accounts are exactly the segment to
// re-engage off-platform.
func (p *adminPanel) runExportCSV(b *tgbot.Bot, chatID int64) {
	ctx, cancel := context.WithTimeout(p.appCtx, 10*time.Minute)
	defer cancel()

	var buf bytes.Buffer
	// UTF-8 BOM so Excel on Windows auto-detects the encoding. Linux
	// readers ignore the leading 0xEF 0xBB 0xBF.
	buf.Write([]byte{0xEF, 0xBB, 0xBF})

	w := csv.NewWriter(&buf)
	header := []string{
		"Telegram ID", "Username", "First Name", "Last Name",
		"Locale", "Premium", "Active", "Traffic Source", "Registration Date",
	}
	if err := w.Write(header); err != nil {
		log.Printf("[admin] export csv header error: %v", err)
		p.notifyExportError(ctx, b, chatID)
		return
	}

	var total int
	err := p.db.WithContext(ctx).
		Model(&model.User{}).
		Where("deleted_at IS NULL").
		Order("created_at DESC").
		FindInBatches(&[]model.User{}, exportBatchSize, func(tx *gorm.DB, _ int) error {
			users, ok := tx.Statement.Dest.(*[]model.User)
			if !ok {
				return fmt.Errorf("export: unexpected batch dest type")
			}
			for i := range *users {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}
				u := &(*users)[i]
				premium := "no"
				if u.IsDonator {
					premium = "yes"
				}
				active := "no"
				if u.IsActive {
					active = "yes"
				}
				if err := w.Write([]string{
					strconv.FormatInt(u.TelegramID, 10),
					u.Username,
					u.FirstName,
					u.LastName,
					u.Locale,
					premium,
					active,
					u.TrafficSourceID,
					u.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
				}); err != nil {
					return err
				}
				total++
			}
			// Flush per batch so the writer's internal buffer doesn't grow
			// unboundedly across iterations.
			w.Flush()
			return w.Error()
		}).Error

	if err != nil {
		log.Printf("[admin] export csv iteration error: %v", err)
		p.notifyExportError(ctx, b, chatID)
		return
	}
	w.Flush()
	if err := w.Error(); err != nil {
		log.Printf("[admin] export csv final flush error: %v", err)
		p.notifyExportError(ctx, b, chatID)
		return
	}

	filename := fmt.Sprintf("subguard_users_%s.csv", time.Now().Format("20060102"))
	if _, sendErr := b.SendDocument(ctx, &tgbot.SendDocumentParams{
		ChatID: chatID,
		Document: &models.InputFileUpload{
			Filename: filename,
			Data:     bytes.NewReader(buf.Bytes()),
		},
		Caption:   fmt.Sprintf("✅ Экспорт завершён. В файле записей: *%d*", total),
		ParseMode: "Markdown",
	}); sendErr != nil {
		log.Printf("[admin] export csv send error: %v", sendErr)
		p.notifyExportError(ctx, b, chatID)
	}
}

func (p *adminPanel) notifyExportError(ctx context.Context, b *tgbot.Bot, chatID int64) {
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   "❌ Ошибка при выгрузке. Попробуйте позже.",
	})
}

// ── Users Module ───────────────────────────────────────

func (p *adminPanel) handleUsersPrompt(ctx context.Context, b *tgbot.Bot, tgID, chatID int64, msgID int) {
	p.setState(ctx, tgID, stateAwaitUserID)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "👤 Введите *Telegram ID* или *@username* пользователя:",
		ParseMode: "Markdown",
	})
}

func (p *adminPanel) handleUserLookup(ctx context.Context, b *tgbot.Bot, adminTgID, chatID int64, text string) {
	p.clearState(ctx, adminTgID)

	var user *model.User
	var err error

	if strings.HasPrefix(text, "@") {
		username := strings.TrimPrefix(text, "@")
		user, err = p.repo.FindUserByUsername(username)
	} else {
		lookupID, parseErr := strconv.ParseInt(text, 10, 64)
		if parseErr != nil {
			b.SendMessage(ctx, &tgbot.SendMessageParams{
				ChatID: chatID,
				Text:   "❌ Некорректный Telegram ID. Введите /admin для меню.",
			})
			return
		}
		user, err = p.repo.FindUserByTelegramID(lookupID)
	}

	if err != nil || user == nil {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Пользователь не найден.",
		})
		return
	}

	p.sendUserCard(ctx, b, chatID, 0, user)
}

// sendUserCard renders the user detail card. If msgID > 0 it edits, otherwise sends.
func (p *adminPanel) sendUserCard(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int, user *model.User) {
	premiumStatus := "нет"
	if user.IsDonator {
		premiumStatus = "✅ да"
	}
	banStatus := "нет"
	if user.IsBanned {
		banStatus = "🛑 да"
	}

	card := fmt.Sprintf(`👤 *Карточка пользователя*

📛 Имя: %s %s
👤 Username: @%s
🆔 Telegram ID: %d
🆔 Internal ID: %d
💎 Premium: %s
🚫 Бан: %s
🕐 Регистрация: %s`,
		escapeMarkdownLite(user.FirstName),
		escapeMarkdownLite(user.LastName),
		escapeMarkdownLite(user.Username),
		user.TelegramID,
		user.ID,
		premiumStatus,
		banStatus,
		user.CreatedAt.Format("02.01.2006"))

	idStr := strconv.FormatUint(uint64(user.ID), 10)

	banBtnText := "🛑 Заблокировать"
	if user.IsBanned {
		banBtnText = "✅ Разблокировать"
	}

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "💎 Выдать Premium", CallbackData: "admin_premium_grant:" + idStr},
				{Text: "🚫 Забрать Premium", CallbackData: "admin_premium_revoke:" + idStr},
			},
			{
				{Text: banBtnText, CallbackData: "admin_ban:" + idStr},
				{Text: "🗑 Удалить", CallbackData: "admin_udel:" + idStr},
			},
			{{Text: "🔙 Назад", CallbackData: "admin_back"}},
		},
	}

	if msgID > 0 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   msgID,
			Text:        card,
			ParseMode:   "Markdown",
			ReplyMarkup: &kb,
		})
	} else {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:      chatID,
			Text:        card,
			ParseMode:   "Markdown",
			ReplyMarkup: &kb,
		})
	}
}

func (p *adminPanel) handlePremiumChange(ctx context.Context, b *tgbot.Bot, data string, grant bool, chatID int64, msgID int) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	uid, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return
	}

	if err := p.repo.SetDonatorStatus(uint(uid), grant); err != nil {
		log.Printf("[admin] premium change error: %v", err)
		return
	}

	// Refresh the user card
	var user model.User
	if err := p.db.First(&user, uid).Error; err == nil {
		p.sendUserCard(ctx, b, chatID, msgID, &user)
	}
}

func (p *adminPanel) handleBanToggle(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	uid, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return
	}

	var user model.User
	if err := p.db.First(&user, uid).Error; err != nil {
		return
	}

	newBanned := !user.IsBanned
	if err := p.repo.SetBannedStatus(uint(uid), newBanned); err != nil {
		log.Printf("[admin] ban toggle error: %v", err)
		return
	}

	// Notify the user about ban
	if newBanned {
		banMsg := "🛑 Your account has been suspended for violating the terms of service."
		if user.Locale == "ru" {
			banMsg = "🛑 Ваш аккаунт был заблокирован за нарушение правил сервиса."
		}
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: user.TelegramID,
			Text:   banMsg,
		})
	}

	// Refresh the card
	user.IsBanned = newBanned
	p.sendUserCard(ctx, b, chatID, msgID, &user)
}

func (p *adminPanel) handleUserDelete(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	uid, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return
	}

	if err := p.repo.SoftDeleteUser(uint(uid)); err != nil {
		log.Printf("[admin] user delete error: %v", err)
		return
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        fmt.Sprintf("🗑 Пользователь #%d удалён.", uid),
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "🔙 Назад", CallbackData: "admin_back"}}}},
	})
}

// ── Settings Module ────────────────────────────────────

func (p *adminPanel) handleSettingsMenu(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	settings, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] settings load error: %v", err)
		return
	}

	paywallStatus := "ВЫКЛЮЧЕН 🔴"
	if settings.PaywallEnabled {
		paywallStatus = "ВКЛЮЧЕН 🟢"
	}
	// Emergency switches read inverted: "ON" is the alarm state, so the
	// red dot marks ON and the green dot marks the healthy OFF state.
	maintenanceStatus := "ВЫКЛЮЧЕНЫ 🟢"
	if settings.MaintenanceMode {
		maintenanceStatus = "ВКЛЮЧЕНЫ 🔴"
	}
	notificationsStatus := "ВЫКЛЮЧЕНА 🟢"
	if settings.PauseNotifications {
		notificationsStatus = "ВКЛЮЧЕНА 🔴"
	}

	// Free-tier limits moved to their own "📊 Настройка лимитов" submenu
	// to keep this top-level menu short — see handleLimitsMenu.
	text := fmt.Sprintf("⚙ *Настройки системы*\n\n"+
		"💳 Пейвол: *%s*\n\n"+
		"🛠 Техработы: *%s*\n"+
		"🔕 Пауза уведомлений: *%s*",
		paywallStatus, maintenanceStatus, notificationsStatus)

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "Переключить Пейвол 🔄", CallbackData: "admin_toggle_paywall"}},
			{{Text: "📊 Настройка лимитов", CallbackData: "admin_limits"}},
			{{Text: "💰 Настройка цен", CallbackData: "admin_prices"}},
			{{Text: "🛠 Переключить Техработы", CallbackData: "admin_toggle_maintenance"}},
			{{Text: "🔕 Переключить Уведомления", CallbackData: "admin_toggle_notifications"}},
			{{Text: "🔙 Назад", CallbackData: "admin_back"}},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handlePaywallToggle(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	settings, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] paywall toggle: load error: %v", err)
		return
	}

	settings.PaywallEnabled = !settings.PaywallEnabled
	if err := p.repo.UpdateSettings(settings); err != nil {
		log.Printf("[admin] paywall toggle: save error: %v", err)
		return
	}

	// Re-render the settings menu with updated status
	p.handleSettingsMenu(ctx, b, chatID, msgID)
}

// handleSwitchToggle flips one of the emergency kill-switches
// (maintenance mode / notification pause) and re-renders the settings
// menu. Same shape as handlePaywallToggle — load, invert, persist,
// redraw. The callback was already acked with a "Статус изменён!"
// toast by the router.
func (p *adminPanel) handleSwitchToggle(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int, which string) {
	settings, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] %s toggle: load error: %v", which, err)
		return
	}

	switch which {
	case "maintenance":
		settings.MaintenanceMode = !settings.MaintenanceMode
		log.Printf("[admin] maintenance_mode -> %v (by tg=%d)", settings.MaintenanceMode, chatID)
	case "notifications":
		settings.PauseNotifications = !settings.PauseNotifications
		log.Printf("[admin] pause_notifications -> %v (by tg=%d)", settings.PauseNotifications, chatID)
	default:
		return
	}

	if err := p.repo.UpdateSettings(settings); err != nil {
		log.Printf("[admin] %s toggle: save error: %v", which, err)
		return
	}

	p.handleSettingsMenu(ctx, b, chatID, msgID)
}

// handleLimitsMenu renders the "📊 Настройка лимитов" submenu — the
// free-tier subscription / room limits with ± buttons. Split out of the
// main settings menu to keep that top level short.
func (p *adminPanel) handleLimitsMenu(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	settings, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] limits menu: load error: %v", err)
		return
	}

	text := fmt.Sprintf("📊 *Настройка лимитов*\n\n"+
		"Установите ограничения для бесплатных пользователей:\n"+
		"📋 Подписки: *%d*\n"+
		"🚪 Комнаты: *%d*",
		settings.FreeSubsLimit, settings.FreeRoomLimit)

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "➖", CallbackData: "admin_subs_dec"},
				{Text: fmt.Sprintf("📋 Подписки: %d", settings.FreeSubsLimit), CallbackData: "admin_noop"},
				{Text: "➕", CallbackData: "admin_subs_inc"},
			},
			{
				{Text: "➖", CallbackData: "admin_rooms_dec"},
				{Text: fmt.Sprintf("🚪 Комнаты: %d", settings.FreeRoomLimit), CallbackData: "admin_noop"},
				{Text: "➕", CallbackData: "admin_rooms_inc"},
			},
			{{Text: "🔙 Назад", CallbackData: "admin_settings"}},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

// handleLimitChange adjusts free_subs_limit or free_room_limit by delta
// (typically +1 or -1) and re-renders the limits submenu it was invoked
// from.
func (p *adminPanel) handleLimitChange(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int, resource string, delta int) {
	settings, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] limit change: load error: %v", err)
		return
	}

	switch resource {
	case "subs":
		settings.FreeSubsLimit += delta
		if settings.FreeSubsLimit < 0 {
			settings.FreeSubsLimit = 0
		}
	case "rooms":
		settings.FreeRoomLimit += delta
		if settings.FreeRoomLimit < 0 {
			settings.FreeRoomLimit = 0
		}
	}

	if err := p.repo.UpdateSettings(settings); err != nil {
		log.Printf("[admin] limit change: save error: %v", err)
		return
	}

	p.handleLimitsMenu(ctx, b, chatID, msgID)
}

// ── Premium Pricing Submenu ────────────────────────────

// Plan-split pricing ± steps and floors. Stars move in 10s (floor 10);
// crypto USD moves in 1s (floor 1). A ➖ tap can never drive a price
// below its floor.
const (
	priceStarsStep   = 10
	priceCryptoStep  = 1
	priceStarsFloor  = 10
	priceCryptoFloor = 1
)

// handlePricesMenu renders the "💰 Настройка цен" submenu — the six
// plan-split Premium prices (Stars RU/EN × Month/Lifetime, Crypto USD ×
// Month/Lifetime) with a ± row per price.
func (p *adminPanel) handlePricesMenu(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	s, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] prices menu: load error: %v", err)
		return
	}

	text := fmt.Sprintf("💰 *Настройка цен*\n\n"+
		"⭐ Telegram Stars (RU):\n"+
		"1 Месяц: *%d* | Навсегда: *%d*\n\n"+
		"⭐ Telegram Stars (EN):\n"+
		"1 Месяц: *%d* | Навсегда: *%d*\n\n"+
		"💎 CryptoPay (USD) - RU:\n"+
		"1 Месяц: *$%d* | Навсегда: *$%d*\n\n"+
		"💎 CryptoPay (USD) - EN:\n"+
		"1 Месяц: *$%d* | Навсегда: *$%d*",
		s.PriceStarsMonthRU, s.PriceStarsLifetimeRU,
		s.PriceStarsMonthEN, s.PriceStarsLifetimeEN,
		s.PriceCryptoMonthUSDRU, s.PriceCryptoLifetimeUSDRU,
		s.PriceCryptoMonthUSDEN, s.PriceCryptoLifetimeUSDEN)

	// priceRow builds a "[ −step ] [ label ] [ +step ]" keyboard row.
	priceRow := func(label, key, minus, plus string) []models.InlineKeyboardButton {
		return []models.InlineKeyboardButton{
			{Text: minus, CallbackData: key + "_dec"},
			{Text: label, CallbackData: "admin_noop"},
			{Text: plus, CallbackData: key + "_inc"},
		}
	}

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			priceRow("⭐ Месяц RU", "pr_st_m_ru", "➖10", "➕10"),
			priceRow("⭐ Навсегда RU", "pr_st_l_ru", "➖10", "➕10"),
			priceRow("⭐ Месяц EN", "pr_st_m_en", "➖10", "➕10"),
			priceRow("⭐ Навсегда EN", "pr_st_l_en", "➖10", "➕10"),
			priceRow("💎 Месяц RU", "pr_cr_m_ru", "➖1", "➕1"),
			priceRow("💎 Навсегда RU", "pr_cr_l_ru", "➖1", "➕1"),
			priceRow("💎 Месяц EN", "pr_cr_m_en", "➖1", "➕1"),
			priceRow("💎 Навсегда EN", "pr_cr_l_en", "➖1", "➕1"),
			{{Text: "🔙 Назад", CallbackData: "admin_settings"}},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

// handlePriceCallback applies a "pr_<field>_<dir>" pricing tweak. The
// suffix (_inc/_dec) gives the direction; the field prefix selects the
// AppSettings column. Stars step ±10 (floor 10), crypto ±1 (floor 1).
func (p *adminPanel) handlePriceCallback(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int, data string) {
	var sign int
	var field string
	switch {
	case strings.HasSuffix(data, "_inc"):
		sign, field = 1, strings.TrimSuffix(data, "_inc")
	case strings.HasSuffix(data, "_dec"):
		sign, field = -1, strings.TrimSuffix(data, "_dec")
	default:
		return
	}

	s, err := p.repo.GetSettings()
	if err != nil {
		log.Printf("[admin] price change: load error: %v", err)
		return
	}

	starsDelta := sign * priceStarsStep
	cryptoDelta := sign * priceCryptoStep
	switch field {
	case "pr_st_m_ru":
		s.PriceStarsMonthRU = clampMin(s.PriceStarsMonthRU+starsDelta, priceStarsFloor)
	case "pr_st_l_ru":
		s.PriceStarsLifetimeRU = clampMin(s.PriceStarsLifetimeRU+starsDelta, priceStarsFloor)
	case "pr_st_m_en":
		s.PriceStarsMonthEN = clampMin(s.PriceStarsMonthEN+starsDelta, priceStarsFloor)
	case "pr_st_l_en":
		s.PriceStarsLifetimeEN = clampMin(s.PriceStarsLifetimeEN+starsDelta, priceStarsFloor)
	case "pr_cr_m_ru":
		s.PriceCryptoMonthUSDRU = clampMin(s.PriceCryptoMonthUSDRU+cryptoDelta, priceCryptoFloor)
	case "pr_cr_l_ru":
		s.PriceCryptoLifetimeUSDRU = clampMin(s.PriceCryptoLifetimeUSDRU+cryptoDelta, priceCryptoFloor)
	case "pr_cr_m_en":
		s.PriceCryptoMonthUSDEN = clampMin(s.PriceCryptoMonthUSDEN+cryptoDelta, priceCryptoFloor)
	case "pr_cr_l_en":
		s.PriceCryptoLifetimeUSDEN = clampMin(s.PriceCryptoLifetimeUSDEN+cryptoDelta, priceCryptoFloor)
	default:
		return
	}

	if err := p.repo.UpdateSettings(s); err != nil {
		log.Printf("[admin] price change: save error: %v", err)
		return
	}
	p.handlePricesMenu(ctx, b, chatID, msgID)
}

// clampMin keeps v at or above floor.
func clampMin(v, floor int) int {
	if v < floor {
		return floor
	}
	return v
}

// ── Sponsors Module ────────────────────────────────────

func (p *adminPanel) handleSponsorsMenu(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	settings, _ := p.repo.GetSettings()
	offers, _ := p.repo.ListOffers()

	recsStatus := "✅ ВКЛ"
	if !settings.RecommendationsEnabled {
		recsStatus = "❌ ВЫКЛ"
	}

	text := fmt.Sprintf("💰 *Спонсорские офферы*\nБлок рекомендаций: %s\n\n", recsStatus)

	if len(offers) == 0 {
		text += "_Офферов пока нет_"
	}

	var buttons [][]models.InlineKeyboardButton
	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🔄 Вкл/Выкл весь блок", CallbackData: "admin_recs_toggle"},
	})

	for _, o := range offers {
		status := "✅"
		if !o.IsActive {
			status = "❌"
		}
		langBadge := "[" + strings.ToUpper(o.TargetLanguage) + "]"
		label := fmt.Sprintf("%s %s %s", status, langBadge, o.Title)
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("admin_offer_view:%d", o.ID)},
		})
	}

	buttons = append(buttons,
		[]models.InlineKeyboardButton{{Text: "➕ Добавить оффер", CallbackData: "admin_offer_add"}},
		[]models.InlineKeyboardButton{{Text: "🔙 Назад", CallbackData: "admin_back"}},
	)

	kb := models.InlineKeyboardMarkup{InlineKeyboard: buttons}
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handleRecsToggle(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	settings, _ := p.repo.GetSettings()
	settings.RecommendationsEnabled = !settings.RecommendationsEnabled
	p.repo.UpdateSettings(settings)
	// Re-render the sponsors menu to reflect the change
	p.handleSponsorsMenu(ctx, b, chatID, msgID)
}

func (p *adminPanel) handleOfferView(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_offer_view:42"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)
	offer, err := p.repo.GetOffer(uint(id))
	if err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID,
			Text: "❌ Оффер не найден.",
		})
		return
	}

	status := "✅ Активен"
	if !offer.IsActive {
		status = "❌ Выключен"
	}

	var ctr float64
	if offer.Views > 0 {
		ctr = float64(offer.Clicks) / float64(offer.Views) * 100
	}

	text := fmt.Sprintf(`🏷 *Оффер:* %s
🌍 *Аудитория:* %s

📊 *Статистика:*
👁 Показы: %d | 🖱 Клики: %d | 📈 CTR: %.1f%%

🚦 *Статус:* %s`,
		escapeMarkdownLite(offer.Title),
		strings.ToUpper(offer.TargetLanguage),
		offer.Views, offer.Clicks, ctr,
		status)

	toggleLabel := "❌ Выключить"
	if !offer.IsActive {
		toggleLabel = "✅ Включить"
	}

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: toggleLabel, CallbackData: fmt.Sprintf("admin_offer_toggle:%d", offer.ID)},
				{Text: "🗑 Удалить", CallbackData: fmt.Sprintf("admin_offer_del:%d", offer.ID)},
			},
			{{Text: "🔙 К списку", CallbackData: "admin_sponsors"}},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handleOfferToggle(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_offer_toggle:42"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)
	offer, err := p.repo.GetOffer(uint(id))
	if err != nil {
		return
	}
	p.repo.ToggleOffer(uint(id), !offer.IsActive)
	// Re-render the detail card with updated status
	p.handleOfferView(ctx, b, fmt.Sprintf("admin_offer_view:%d", id), chatID, msgID)
}

func (p *adminPanel) handleOfferDelete(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_offer_del:42"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)
	p.repo.DeleteOffer(uint(id))
	// Return to sponsors list
	p.handleSponsorsMenu(ctx, b, chatID, msgID)
}

func (p *adminPanel) handleOfferAddStart(ctx context.Context, b *tgbot.Bot, tgID, chatID int64, msgID int) {
	p.setState(ctx, tgID, stateAwaitOfferTitle)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "📝 Введите *название* оффера:\n\n_Для отмены введите /cancel_",
		ParseMode: "Markdown",
	})
}

func (p *adminPanel) handleOfferLangPick(ctx context.Context, b *tgbot.Bot, tgID int64, data string, chatID int64, msgID int) {
	// data = "admin_offer_lang:ru"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	lang := parts[1]

	// Parse stored data: title\ndesc\nbadge\nurl\nicon
	raw := p.getData(ctx, tgID)
	lines := strings.SplitN(raw, "\n", 5)
	if len(lines) < 5 {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      "❌ Данные оффера потеряны. Начните заново через /admin.",
		})
		p.clearState(ctx, tgID)
		return
	}

	offer := model.SponsoredOffer{
		Title:          lines[0],
		Description:    lines[1],
		BadgeText:      lines[2],
		URL:            lines[3],
		IconName:       lines[4],
		TargetLanguage: lang,
		IsActive:       true,
	}

	if err := p.repo.CreateOffer(&offer); err != nil {
		log.Printf("[admin] create offer error: %v", err)
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      "❌ Ошибка при создании оффера.",
		})
	} else {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID:    chatID,
			MessageID: msgID,
			Text:      fmt.Sprintf("✅ Оффер *%s* создан! [%s]", escapeMarkdownLite(offer.Title), strings.ToUpper(lang)),
			ParseMode: "Markdown",
			ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: "🔙 К спонсорам", CallbackData: "admin_sponsors"}},
				{{Text: "🔙 Главное меню", CallbackData: "admin_back"}},
			}},
		})
	}
	p.clearState(ctx, tgID)
}

// ── Traffic Module ─────────────────────────────────────

func (p *adminPanel) handleTrafficMenu(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	campaigns, _ := p.repo.ListCampaigns()

	text := "🔗 *Трафик и кампании*\n\n"
	if len(campaigns) == 0 {
		text += "_Кампаний пока нет_"
	} else {
		text += "Выберите кампанию для просмотра деталей:"
	}

	var buttons [][]models.InlineKeyboardButton
	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🔗 Создать ссылку", CallbackData: "admin_traffic_new"},
	})

	for i, c := range campaigns {
		if i >= 20 {
			break
		}
		label := fmt.Sprintf("%s — 🖱 %d | ✅ %d", c.Tag, c.BotStarts, c.Auths)
		// Use tag ID to keep callback under 64 bytes.
		buttons = append(buttons, []models.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("admin_tv:%d", c.ID)},
		})
	}

	buttons = append(buttons, []models.InlineKeyboardButton{
		{Text: "🔙 Назад", CallbackData: "admin_back"},
	})

	kb := models.InlineKeyboardMarkup{InlineKeyboard: buttons}
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handleTrafficView(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_tv:42"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)

	var campaign model.TrafficCampaign
	if err := p.db.First(&campaign, id).Error; err != nil {
		b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
			ChatID: chatID, MessageID: msgID,
			Text: "❌ Кампания не найдена.",
		})
		return
	}

	// Get bot username for the link
	botUsername := "SubGuardBot"
	if info, err := b.GetMe(ctx); err == nil && info != nil {
		botUsername = info.Username
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, campaign.Tag)

	text := fmt.Sprintf("🏷 *Кампания:* `%s`\n🔗 *Ссылка:*\n`%s`\n\n📊 *Статистика:*\n🚀 Стартов бота: %d\n✅ Регистраций: %d",
		campaign.Tag, link, campaign.BotStarts, campaign.Auths)

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🗑 Удалить кампанию", CallbackData: fmt.Sprintf("admin_td:%d", campaign.ID)}},
			{{Text: "🔙 К списку", CallbackData: "admin_traffic"}},
		},
	}

	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handleTrafficDelete(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_td:42"
	parts := strings.SplitN(data, ":", 2)
	if len(parts) < 2 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)
	// Surface DB failures — the previous `p.db.Delete(...)` discarded
	// the result silently, so a permission denial or constraint
	// violation showed up as "nothing happened" in the admin UI with
	// no log trail. Audit Low.
	if err := p.db.Delete(&model.TrafficCampaign{}, id).Error; err != nil {
		log.Printf("[admin.handleTrafficDelete] id=%d: %v", id, err)
	}
	// Return to campaign list
	p.handleTrafficMenu(ctx, b, chatID, msgID)
}

func (p *adminPanel) handleTrafficNewPrompt(ctx context.Context, b *tgbot.Bot, tgID, chatID int64, msgID int) {
	p.setState(ctx, tgID, stateAwaitTrafficTag)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "🏷 Введите *тег* для кампании (латиница, без пробелов):\n\n_Для отмены введите /cancel_",
		ParseMode: "Markdown",
	})
}

func (p *adminPanel) handleTrafficCreate(ctx context.Context, b *tgbot.Bot, tgID, chatID int64, tag string) {
	p.clearState(ctx, tgID)

	// Strip leading/trailing whitespace; allow underscores (Telegram supports them).
	tag = strings.TrimSpace(tag)
	if tag == "" || strings.ContainsAny(tag, " \t\n") {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Тег не может содержать пробелы.",
		})
		return
	}

	fullTag := "ad_" + tag

	// Eagerly create the campaign row so it shows up in the list immediately.
	if err := p.repo.EnsureCampaign(fullTag); err != nil {
		log.Printf("[traffic] EnsureCampaign(%s) error: %v", fullTag, err)
	}

	// Get bot username for the link
	botInfo, err := b.GetMe(ctx)
	botUsername := "SubGuardBot"
	if err == nil && botInfo != nil {
		botUsername = botInfo.Username
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, fullTag)

	// Use code blocks (backticks) for tag and link to prevent Markdown
	// from eating underscores as italic markup.
	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      fmt.Sprintf("✅ Ссылка создана!\n\n🏷 Тег: `%s`\n🔗 Ссылка:\n`%s`", fullTag, link),
		ParseMode: "Markdown",
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🔙 К трафику", CallbackData: "admin_traffic"}},
			{{Text: "🔙 Главное меню", CallbackData: "admin_back"}},
		}},
	})
}

// ── Helpers ────────────────────────────────────────────

func backButton() models.InlineKeyboardMarkup {
	return models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🔙 Назад", CallbackData: "admin_back"}},
		},
	}
}
