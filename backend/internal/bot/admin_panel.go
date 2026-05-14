package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/subguard/backend/internal/config"
	"github.com/subguard/backend/internal/model"
	"github.com/subguard/backend/internal/repository"
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
type adminPanel struct {
	cfg  *config.Config
	db   *gorm.DB
	rdb  *redis.Client
	repo *repository.AdminRepo
}

func newAdminPanel(cfg *config.Config, db *gorm.DB, rdb *redis.Client) *adminPanel {
	return &adminPanel{
		cfg:  cfg,
		db:   db,
		rdb:  rdb,
		repo: repository.NewAdminRepo(db),
	}
}

// ── FSM helpers ────────────────────────────────────────

func (p *adminPanel) setState(ctx context.Context, tgID int64, state string) {
	key := fsmKeyPrefix + strconv.FormatInt(tgID, 10)
	if state == stateNone {
		p.rdb.Del(ctx, key)
		return
	}
	p.rdb.Set(ctx, key, state, fsmTTL)
}

func (p *adminPanel) getState(ctx context.Context, tgID int64) string {
	key := fsmKeyPrefix + strconv.FormatInt(tgID, 10)
	val, err := p.rdb.Get(ctx, key).Result()
	if err != nil {
		return stateNone
	}
	return val
}

func (p *adminPanel) setData(ctx context.Context, tgID int64, data string) {
	key := fsmDataPrefix + strconv.FormatInt(tgID, 10)
	p.rdb.Set(ctx, key, data, fsmTTL)
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
	p.rdb.Del(ctx, fsmKeyPrefix+idStr)
	p.rdb.Del(ctx, fsmDataPrefix+idStr)
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

	// Always ack the callback to remove the loading spinner.
	b.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: cb.ID})

	chatID := cb.From.ID // DM, so chatID == user ID
	msgID := 0
	if cb.Message.Message != nil {
		chatID = cb.Message.Message.Chat.ID
		msgID = cb.Message.Message.ID
	}

	data := cb.Data
	switch {
	case data == "admin_back":
		p.clearState(ctx, cb.From.ID)
		p.sendMainMenu(ctx, b, chatID, msgID)

	case data == "admin_stats":
		p.handleStats(ctx, b, chatID, msgID)

	case data == "admin_users":
		p.handleUsersPrompt(ctx, b, cb.From.ID, chatID, msgID)

	case data == "admin_sponsors":
		p.handleSponsorsMenu(ctx, b, chatID, msgID)

	case data == "admin_traffic":
		p.handleTrafficMenu(ctx, b, chatID, msgID)

	case data == "admin_recs_toggle":
		p.handleRecsToggle(ctx, b, chatID, msgID)

	case data == "admin_offer_add":
		p.handleOfferAddStart(ctx, b, cb.From.ID, chatID, msgID)

	case data == "admin_traffic_new":
		p.handleTrafficNewPrompt(ctx, b, cb.From.ID, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_toggle:"):
		p.handleOfferToggle(ctx, b, data, chatID, msgID)

	case strings.HasPrefix(data, "admin_premium_grant:"):
		p.handlePremiumChange(ctx, b, data, true, chatID, msgID)

	case strings.HasPrefix(data, "admin_premium_revoke:"):
		p.handlePremiumChange(ctx, b, data, false, chatID, msgID)

	case strings.HasPrefix(data, "admin_offer_lang:"):
		p.handleOfferLangPick(ctx, b, cb.From.ID, data, chatID, msgID)
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
			ChatID: chatID,
			Text:   "📝 Введите *описание* оффера (или `-` чтобы пропустить):",
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
			ChatID: chatID,
			Text:   "🏷 Введите *текст бейджа* (напр. \"Скидка 30%\" или `-` чтобы пропустить):",
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
			ChatID: chatID,
			Text:   "🔗 Введите *URL* оффера:",
			ParseMode: "Markdown",
		})

	case stateAwaitOfferURL:
		prev := p.getData(ctx, tgID)
		p.setData(ctx, tgID, prev+"\n"+text)
		p.setState(ctx, tgID, stateAwaitOfferIcon)
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID:    chatID,
			Text:      "🎨 Введите *иконку* оффера:\n• Системное имя (напр. `netflix`)\n• URL на изображение (`https://...`)\n• `-` чтобы пропустить",
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
			ChatID: chatID,
			Text:   "🌍 Выберите *аудиторию*:",
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
	}
}

// ── Stats Module ───────────────────────────────────────

func (p *adminPanel) handleStats(ctx context.Context, b *tgbot.Bot, chatID int64, msgID int) {
	stats, err := p.repo.GetStats()
	if err != nil {
		log.Printf("[admin] stats error: %v", err)
		return
	}

	text := fmt.Sprintf(`📊 *Статистика SubGuard*

👥 Пользователей: %d
📱 DAU: %d | MAU: %d
💎 Премиум: %d
📋 Подписок: %d
🏠 Комнат: %d`,
		stats.TotalUsers, stats.DAU, stats.MAU,
		stats.Donators, stats.TotalSubscriptions, stats.TotalRooms)

	kb := backButton()
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        text,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

// ── Users Module ───────────────────────────────────────

func (p *adminPanel) handleUsersPrompt(ctx context.Context, b *tgbot.Bot, tgID, chatID int64, msgID int) {
	p.setState(ctx, tgID, stateAwaitUserID)
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: msgID,
		Text:      "👤 Введите *Telegram ID* пользователя:",
		ParseMode: "Markdown",
	})
}

func (p *adminPanel) handleUserLookup(ctx context.Context, b *tgbot.Bot, adminTgID, chatID int64, text string) {
	p.clearState(ctx, adminTgID)

	lookupID, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Некорректный Telegram ID. Введите /admin для меню.",
		})
		return
	}

	user, err := p.repo.FindUserByTelegramID(lookupID)
	if err != nil {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Пользователь не найден.",
		})
		return
	}

	premiumStatus := "нет"
	if user.IsDonator {
		premiumStatus = "✅ да"
	}

	card := fmt.Sprintf(`👤 *Карточка пользователя*

📛 Имя: %s %s
👤 Username: @%s
🆔 Telegram ID: %d
🆔 Internal ID: %d
💎 Premium: %s
🕐 Регистрация: %s`,
		escapeMarkdownLite(user.FirstName),
		escapeMarkdownLite(user.LastName),
		escapeMarkdownLite(user.Username),
		user.TelegramID,
		user.ID,
		premiumStatus,
		user.CreatedAt.Format("02.01.2006"))

	idStr := strconv.FormatUint(uint64(user.ID), 10)
	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{
				{Text: "💎 Выдать Premium", CallbackData: "admin_premium_grant:" + idStr},
				{Text: "🚫 Забрать Premium", CallbackData: "admin_premium_revoke:" + idStr},
			},
			{{Text: "🔙 Назад", CallbackData: "admin_back"}},
		},
	}

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:      chatID,
		Text:        card,
		ParseMode:   "Markdown",
		ReplyMarkup: &kb,
	})
}

func (p *adminPanel) handlePremiumChange(ctx context.Context, b *tgbot.Bot, data string, grant bool, chatID int64, msgID int) {
	// data = "admin_premium_grant:123" or "admin_premium_revoke:123"
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

	status := "🚫 снят"
	if grant {
		status = "💎 выдан"
	}
	b.EditMessageText(ctx, &tgbot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        fmt.Sprintf("✅ Premium %s пользователю #%d", status, uid),
		ParseMode:   "Markdown",
		ReplyMarkup: &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{{{Text: "🔙 Назад", CallbackData: "admin_back"}}}},
	})
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
			{Text: label, CallbackData: fmt.Sprintf("admin_offer_toggle:%d:%v", o.ID, !o.IsActive)},
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

func (p *adminPanel) handleOfferToggle(ctx context.Context, b *tgbot.Bot, data string, chatID int64, msgID int) {
	// data = "admin_offer_toggle:42:true"
	parts := strings.SplitN(data, ":", 3)
	if len(parts) < 3 {
		return
	}
	id, _ := strconv.ParseUint(parts[1], 10, 64)
	active := parts[2] == "true"
	p.repo.ToggleOffer(uint(id), active)
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
		for i, c := range campaigns {
			if i >= 15 {
				text += fmt.Sprintf("\n_...и ещё %d_", len(campaigns)-15)
				break
			}
			text += fmt.Sprintf("`%s` — 👆 %d клик, 🚀 %d старт, ✅ %d рег\n",
				c.Tag, c.Clicks, c.BotStarts, c.Auths)
		}
	}

	kb := models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{{Text: "🔗 Создать ссылку", CallbackData: "admin_traffic_new"}},
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

	if tag == "" || strings.ContainsAny(tag, " \t\n") {
		b.SendMessage(ctx, &tgbot.SendMessageParams{
			ChatID: chatID,
			Text:   "❌ Тег не может содержать пробелы.",
		})
		return
	}

	fullTag := "ad_" + tag

	// Upsert the campaign row
	p.repo.IncrementCampaign(fullTag, "clicks")
	// Reset the click we just added — we only want to create the row
	p.db.Model(&model.TrafficCampaign{}).Where("tag = ?", fullTag).Update("clicks", 0)

	// Get bot username for the link
	botInfo, err := b.GetMe(ctx)
	botUsername := "SubGuardBot"
	if err == nil && botInfo != nil {
		botUsername = botInfo.Username
	}

	link := fmt.Sprintf("https://t.me/%s?start=%s", botUsername, fullTag)

	b.SendMessage(ctx, &tgbot.SendMessageParams{
		ChatID:    chatID,
		Text:      fmt.Sprintf("✅ Ссылка создана!\n\n🏷 Тег: `%s`\n🔗 Ссылка: %s", fullTag, link),
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
