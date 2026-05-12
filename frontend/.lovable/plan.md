
# SubGuard: План доработки фронтенда

Проект уже имеет хорошую базу: дашборд, карточки подписок, каталог (8 сервисов), Shared Rooms (базовый UI), календарь, аналитика, админ-панель, i18n (en/ru). Ниже — что нужно доработать, разбито на этапы.

---

## Этап 1: Фундамент — Zustand + расширенный каталог + удаление подписок

**Цель:** Перевести локальный state на Zustand, расширить каталог до 35+ сервисов, добавить удаление подписок.

- Установить `zustand`
- Создать `src/stores/subscriptionStore.ts` — подписки, фильтр, CRUD
- Создать `src/stores/settingsStore.ts` — настройки пользователя, локаль, валюта
- Создать `src/stores/roomStore.ts` — Shared Rooms
- Расширить `POPULAR_SERVICES` в `mockData.ts` до 35+ (ChatGPT, Midjourney, Figma, Canva, Dropbox, Google One, Xbox Game Pass, PlayStation Plus, Twitch, HBO Max, Crunchyroll, NordVPN, 1Password, Todoist, Linear, Slack, Zoom, Duolingo, Strava, Headspace и т.д.)
- Добавить категории: "AI", "Games", "Cloud", "VPN", "Health & Fitness"
- Добавить кнопку удаления подписки в форму редактирования (AddSubscriptionSheet)
- Подключить Dashboard к Zustand вместо локального useState

## Этап 2: Мультивалютность и конвертация

**Цель:** Автоконвертация стоимости подписок в базовую валюту пользователя.

- Добавить валюту KZT в список
- Создать `src/lib/currencyRates.ts` — захардкоженные кросс-курсы (будут заменены на API-данные позже с бэкендом)
- Обновить `SummaryHeader` — показывать Total в базовой валюте пользователя
- Обновить `AnalyticsView` — все суммы конвертируются в базовую валюту
- Добавить выбор базовой валюты в `SettingsView` (USD, EUR, RUB, GBP, KZT)

## Этап 3: Telegram Mini App интеграция

**Цель:** Полноценная интеграция с TMA SDK.

- Установить `@twa-dev/sdk`
- Создать `src/lib/telegram.ts` — обёртка: `initTelegramApp()`, `getTelegramUser()`, `hapticFeedback()`, `openLink()`, `openInvoice()`
- Автоопределение темы Telegram (CSS variables от TMA)
- Автоопределение локали пользователя из `WebApp.initDataUnsafe.user.language_code`
- Определение timezone через `Intl.DateTimeFormat().resolvedOptions().timeZone`
- Haptic feedback на основных действиях (добавление, удаление, навигация)
- Обновить `SummaryHeader` — показывать имя и аватар из Telegram
- Safe area insets из TMA (уже частично есть в CSS)

## Этап 4: Shared Rooms — полный функционал

**Цель:** Создание комнат, приглашения, расчёт долгов.

- Форма создания новой комнаты (название, выбор сервисов из подписок)
- Экран комнаты: список участников с аватарами
- Расчёт "кто кому сколько должен" (split-bill логика)
- Кнопка "Напомнить об оплате" (пинг) — mock, позже через Telegram Bot API
- Генерация deep link для приглашения (`t.me/SubGuardBot?start=room_XXX`) — UI кнопка "Поделиться ссылкой"
- Статусы оплаты участников (оплатил / не оплатил)

## Этап 5: Статус Donator (Telegram Stars)

**Цель:** UI для донатов и бейдж донатера.

- Кнопка "Поддержать проект ⭐" в Settings
- Модалка с выбором суммы Stars
- Вызов `WebApp.openInvoice()` (mock в браузере)
- Анимированный бейдж ✨ в профиле и на дашборде для донатеров
- Zustand store для статуса донатера

## Этап 6: Админ-панель — расширение

**Цель:** Добавить недостающие функции админки.

- **Deep Link Generator**: форма для создания UTM-ссылок `t.me/SubGuardBot?start=ad_CHANNEL`, таблица кампаний с воронкой (starts → auths → subs → donations)
- **Feature Toggles**: переключатели CPA-модуля и Soft-Gate прямо в UI (сохранение в store)
- **Soft-Gate UI**: баннер "Подпишитесь на канал" при попытке создать комнату (когда soft-gate включён)
- **Сегментированные рассылки**: фильтр "исключить пользователей с подпиской на X" в Broadcast-табе

---

## Техническая реализация

### Новые файлы
```
src/stores/subscriptionStore.ts
src/stores/settingsStore.ts
src/stores/roomStore.ts
src/stores/donationStore.ts
src/lib/telegram.ts
src/lib/currencyRates.ts
src/components/subguard/CreateRoomSheet.tsx
src/components/subguard/DonationSheet.tsx
src/components/subguard/DeepLinkGenerator.tsx
src/components/subguard/SoftGateBanner.tsx
```

### Изменяемые файлы
- `src/components/subguard/Dashboard.tsx` — Zustand вместо useState
- `src/components/subguard/AddSubscriptionSheet.tsx` — кнопка удаления
- `src/components/subguard/SummaryHeader.tsx` — Telegram user, конвертация
- `src/components/subguard/SettingsView.tsx` — базовая валюта, донат-кнопка
- `src/components/subguard/SharedRoomSheet.tsx` — долги, пинг, статусы
- `src/components/subguard/SharedRooms.tsx` — кнопка создания комнаты
- `src/components/subguard/AnalyticsView.tsx` — конвертация в базовую валюту
- `src/components/subguard/AdminPanel.tsx` — deep links, toggles
- `src/lib/mockData.ts` — 35+ сервисов, новые категории
- `src/lib/i18n.ts` — новые строки для всех фич
- `src/types/subscription.ts` — новые BrandKey'и, типы комнат/донатов

### Зависимости
- `zustand` — state management
- `@twa-dev/sdk` — Telegram Web App SDK

---

## Рекомендуемый порядок

Предлагаю реализовывать **по одному этапу за раз** — так проще контролировать результат и тестировать. Начнём с Этапа 1 (Zustand + каталог + удаление).
