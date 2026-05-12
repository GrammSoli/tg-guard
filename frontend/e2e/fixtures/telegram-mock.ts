import { test as base, type Page } from "@playwright/test";

/**
 * Test user IDs — must match backend seed/testdata.go constants.
 */
export const TEST_OWNER_TG_ID = 111111;
export const TEST_MEMBER_TG_ID = 222222;
export const TEST_DEBTOR_TG_ID = 333333;

export const TEST_INVITE_CODE = "test_invite_e2e";
export const TEST_ROOM_ID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee";

/**
 * Injects a mock `window.Telegram.WebApp` object before page load.
 * This allows all frontend code that relies on Telegram Mini App API
 * to function correctly in a regular Chromium browser.
 */
async function injectTelegramMock(page: Page, telegramId: number) {
  const script = `
    window.Telegram = {
      WebApp: {
        initData: "test_user_${telegramId}",
        initDataUnsafe: {
          user: {
            id: ${telegramId},
            first_name: "TestOwner",
            last_name: "",
            username: "test_owner",
            language_code: "en"
          },
          auth_date: Math.floor(Date.now() / 1000),
          hash: "test_hash"
        },
        version: "7.0",
        platform: "tdesktop",
        colorScheme: "dark",
        themeParams: {
          bg_color: "#1a1a2e",
          text_color: "#ffffff",
          hint_color: "#aaaaaa",
          link_color: "#6c5ce7",
          button_color: "#6c5ce7",
          button_text_color: "#ffffff",
          secondary_bg_color: "#16213e"
        },
        isExpanded: true,
        viewportHeight: 844,
        viewportStableHeight: 844,
        ready: function() {},
        expand: function() {},
        close: function() {},
        openTelegramLink: function(url) { console.log("[mock] openTelegramLink:", url); },
        openLink: function(url) { console.log("[mock] openLink:", url); },
        showPopup: function() {},
        showAlert: function(msg) { console.log("[mock] showAlert:", msg); },
        showConfirm: function(msg, cb) { cb(true); },
        HapticFeedback: {
          impactOccurred: function() {},
          notificationOccurred: function() {},
          selectionChanged: function() {}
        },
        BackButton: {
          show: function() {},
          hide: function() {},
          onClick: function() {},
          offClick: function() {},
          isVisible: false
        },
        MainButton: {
          show: function() {},
          hide: function() {},
          enable: function() {},
          disable: function() {},
          setText: function() {},
          onClick: function() {},
          offClick: function() {},
          isVisible: false,
          isActive: true,
          text: "",
          color: "",
          textColor: ""
        }
      }
    };
    localStorage.setItem("subguard.onboarded.v1", "true");
  `;
  // Block the real Telegram WebApp SDK — it overwrites window.Telegram
  await page.route("**/telegram.org/js/telegram-web-app.js", (route) =>
    route.fulfill({ status: 200, contentType: "text/javascript", body: "// blocked in test" })
  );
  await page.addInitScript(script);
}

/**
 * Extended test fixture that provides a page with Telegram WebApp mocked.
 * Use `ownerPage` for tests that need owner permissions.
 */
export const test = base.extend<{
  ownerPage: Page;
  memberPage: Page;
}>({
  ownerPage: async ({ page }, use) => {
    await injectTelegramMock(page, TEST_OWNER_TG_ID);
    await use(page);
  },
  memberPage: async ({ browser }, use) => {
    const ctx = await browser.newContext();
    const page = await ctx.newPage();
    await injectTelegramMock(page, TEST_MEMBER_TG_ID);
    await use(page);
    await ctx.close();
  },
});

export { expect } from "@playwright/test";
