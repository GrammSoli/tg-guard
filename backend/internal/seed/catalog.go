package seed

import (
	"log"

	"gorm.io/gorm"

	"github.com/subguard/backend/internal/model"
)

// SeedCatalog inserts the default service catalog if the table is empty.
// These match the POPULAR_SERVICES from the frontend mockData.ts.
func SeedCatalog(db *gorm.DB) {
	var count int64
	db.Model(&model.ServiceCatalog{}).Count(&count)
	if count > 0 {
		return
	}

	items := []model.ServiceCatalog{
		{ID: "netflix", Name: "Netflix", Category: "Entertainment", Domain: "netflix.com", DefaultAmount: 15.49, DefaultCurrency: "USD", Active: true},
		{ID: "spotify", Name: "Spotify", Category: "Music", Domain: "spotify.com", DefaultAmount: 11.99, DefaultCurrency: "USD", Active: true},
		{ID: "youtube", Name: "YouTube Premium", Category: "Entertainment", Domain: "youtube.com", DefaultAmount: 13.99, DefaultCurrency: "USD", Active: true},
		{ID: "applemusic", Name: "Apple Music", Category: "Music", Domain: "music.apple.com", DefaultAmount: 10.99, DefaultCurrency: "USD", Active: true},
		{ID: "disney", Name: "Disney+", Category: "Entertainment", Domain: "disneyplus.com", DefaultAmount: 13.99, DefaultCurrency: "USD", Active: true},
		{ID: "hbo", Name: "HBO Max", Category: "Entertainment", Domain: "max.com", DefaultAmount: 15.99, DefaultCurrency: "USD", Active: true},
		{ID: "hulu", Name: "Hulu", Category: "Entertainment", Domain: "hulu.com", DefaultAmount: 17.99, DefaultCurrency: "USD", Active: true},
		{ID: "primevideo", Name: "Prime Video", Category: "Entertainment", Domain: "primevideo.com", DefaultAmount: 14.99, DefaultCurrency: "USD", Active: true},
		{ID: "appletv", Name: "Apple TV+", Category: "Entertainment", Domain: "tv.apple.com", DefaultAmount: 9.99, DefaultCurrency: "USD", Active: true},
		{ID: "telegram", Name: "Telegram Premium", Category: "Social", Domain: "telegram.org", DefaultAmount: 4.99, DefaultCurrency: "USD", Active: true},
		{ID: "chatgpt", Name: "ChatGPT Plus", Category: "Productivity", Domain: "openai.com", DefaultAmount: 20.00, DefaultCurrency: "USD", Active: true},
		{ID: "claude", Name: "Claude Pro", Category: "Productivity", Domain: "claude.ai", DefaultAmount: 20.00, DefaultCurrency: "USD", Active: true},
		{ID: "notion", Name: "Notion", Category: "Productivity", Domain: "notion.so", DefaultAmount: 10.00, DefaultCurrency: "USD", Active: true},
		{ID: "figma", Name: "Figma", Category: "Design", Domain: "figma.com", DefaultAmount: 15.00, DefaultCurrency: "USD", Active: true},
		{ID: "github", Name: "GitHub Pro", Category: "Developer", Domain: "github.com", DefaultAmount: 4.00, DefaultCurrency: "USD", Active: true},
		{ID: "icloud", Name: "iCloud+", Category: "Utilities", Domain: "icloud.com", DefaultAmount: 2.99, DefaultCurrency: "USD", Active: true},
		{ID: "dropbox", Name: "Dropbox Plus", Category: "Utilities", Domain: "dropbox.com", DefaultAmount: 11.99, DefaultCurrency: "USD", Active: true},
		{ID: "nordvpn", Name: "NordVPN", Category: "Utilities", Domain: "nordvpn.com", DefaultAmount: 12.99, DefaultCurrency: "USD", Active: true},
		{ID: "canva", Name: "Canva Pro", Category: "Design", Domain: "canva.com", DefaultAmount: 12.99, DefaultCurrency: "USD", Active: true},
		{ID: "midjourney", Name: "Midjourney", Category: "AI", Domain: "midjourney.com", DefaultAmount: 10.00, DefaultCurrency: "USD", Active: true},
		{ID: "microsoft365", Name: "Microsoft 365", Category: "Productivity", Domain: "microsoft.com", DefaultAmount: 9.99, DefaultCurrency: "USD", Active: true},
		{ID: "googledrive", Name: "Google One", Category: "Utilities", Domain: "one.google.com", DefaultAmount: 2.99, DefaultCurrency: "USD", Active: true},
		{ID: "twitch", Name: "Twitch Turbo", Category: "Entertainment", Domain: "twitch.tv", DefaultAmount: 11.99, DefaultCurrency: "USD", Active: true},
		{ID: "linkedin", Name: "LinkedIn Premium", Category: "Social", Domain: "linkedin.com", DefaultAmount: 29.99, DefaultCurrency: "USD", Active: true},
		{ID: "duolingo", Name: "Duolingo Plus", Category: "Education", Domain: "duolingo.com", DefaultAmount: 6.99, DefaultCurrency: "USD", Active: true},
	}

	if err := db.Create(&items).Error; err != nil {
		log.Printf("[seed] catalog seed error: %v", err)
		return
	}
	log.Printf("[seed] inserted %d catalog items", len(items))
}
