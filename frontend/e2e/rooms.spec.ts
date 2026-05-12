import { test, expect, TEST_INVITE_CODE } from "./fixtures/telegram-mock";

/**
 * E2E tests for SubGuard shared rooms flow.
 *
 * Prerequisites:
 *   Backend running with APP_ENV=test on port 3001
 *   Test DB seeded (happens automatically on backend start)
 */

test.describe("Shared Rooms", () => {
  test.beforeEach(async ({ ownerPage }) => {
    await ownerPage.goto("/");
    await ownerPage.waitForLoadState("networkidle");
  });

  // ─── Test 1: Deep Link Join ──────────────────────────────────
  test("deep link opens room via startapp param", async ({ ownerPage }) => {
    // Inject start_param into the mock before navigating
    await ownerPage.addInitScript(`
      if (window.Telegram && window.Telegram.WebApp) {
        window.Telegram.WebApp.initDataUnsafe.start_param = "room_${TEST_INVITE_CODE}";
      }
    `);

    // Navigate to trigger deep link handler
    await ownerPage.goto("/");
    await ownerPage.waitForLoadState("networkidle");

    // The room should be visible on the dashboard
    const roomCard = ownerPage.getByText("Test Room");
    await expect(roomCard.first()).toBeVisible({ timeout: 10_000 });
  });

  // ─── Test 2: Open Room & Remind ──────────────────────────────
  test("remind button sends notification and shows cooldown", async ({
    ownerPage,
  }) => {
    // Click on the room card to open the detail sheet
    await ownerPage.getByText("Test Room").first().click();

    // Wait for room sheet to open — look for "MEMBERS — PAYMENT STATUS" heading
    await expect(
      ownerPage.getByText(/payment status/i)
    ).toBeVisible({ timeout: 5000 });

    // Find and click the Remind button by text
    const remindButton = ownerPage.getByRole("button", { name: /remind/i });
    await expect(remindButton).toBeVisible({ timeout: 3000 });
    await remindButton.click();

    // Should show success toast
    const toast = ownerPage.locator("[data-sonner-toast]");
    await expect(toast.first()).toBeVisible({ timeout: 5000 });

    // After clicking, button should show "Sent" text (cooldown state)
    await expect(
      ownerPage.getByRole("button", { name: /sent|cooldown/i })
    ).toBeVisible({ timeout: 3000 });
  });

  // ─── Test 3: Mark Paid Confirmation Dialog ───────────────────
  test("mark paid requires confirmation dialog", async ({ ownerPage }) => {
    // Open room detail
    await ownerPage.getByText("Test Room").first().click();
    await expect(
      ownerPage.getByText(/payment status/i)
    ).toBeVisible({ timeout: 5000 });

    // Find an unpaid member's "Mark Paid" button (amber text with Clock icon)
    const markPaidButton = ownerPage.getByRole("button", { name: /mark paid/i });

    // There should be at least 1 unpaid member (TestDebtor)
    await expect(markPaidButton.first()).toBeVisible({ timeout: 5000 });

    // Click it — should open confirmation dialog
    await markPaidButton.first().click();

    // AlertDialog should appear with confirmation text
    const alertDialog = ownerPage.locator("[role='alertdialog']");
    await expect(alertDialog).toBeVisible({ timeout: 3000 });

    // Test CANCEL — status should NOT change
    const cancelButton = ownerPage.getByRole("button", { name: /cancel/i });
    await cancelButton.click();

    // Dialog should close
    await expect(alertDialog).not.toBeVisible({ timeout: 3000 });

    // Unpaid "Mark Paid" button should still be there
    await expect(markPaidButton.first()).toBeVisible();

    // Now click again and CONFIRM
    await markPaidButton.first().click();
    await expect(alertDialog).toBeVisible({ timeout: 3000 });

    const confirmButton = ownerPage.getByRole("button", { name: /^confirm$/i });
    await confirmButton.click();

    // Dialog should close
    await expect(alertDialog).not.toBeVisible({ timeout: 3000 });

    // Success toast should appear
    const toast = ownerPage.locator("[data-sonner-toast]");
    await expect(toast.first()).toBeVisible({ timeout: 5000 });

    // The member should now show "Paid" badge (green)
    const paidBadges = ownerPage.getByText("Paid");
    // All members should now be paid — at least the one we just changed
    await expect(paidBadges.first()).toBeVisible({ timeout: 3000 });
  });

  // ─── Test 4: Kick Member ─────────────────────────────────────
  test("owner can remove a member with confirmation", async ({
    ownerPage,
  }) => {
    // Open room detail
    await ownerPage.getByText("Test Room").first().click();
    await expect(
      ownerPage.getByText(/payment status/i)
    ).toBeVisible({ timeout: 5000 });

    // Count member count shown in the stats card
    const memberCountBefore = await ownerPage
      .locator("text=/Members/i")
      .locator("..")
      .locator("p.text-2xl")
      .first()
      .textContent();

    // Find kick button by aria-label "Remove ..."
    const kickButton = ownerPage.locator("button[aria-label^='Remove']");
    await expect(kickButton.first()).toBeVisible({ timeout: 5000 });

    // Click kick
    await kickButton.first().click();

    // Confirmation dialog should appear
    const alertDialog = ownerPage.locator("[role='alertdialog']");
    await expect(alertDialog).toBeVisible({ timeout: 3000 });

    // Confirm the kick — click the action button (last button in footer)
    const confirmButton = alertDialog.getByRole("button").last();
    await confirmButton.click();

    // Dialog should close
    await expect(alertDialog).not.toBeVisible({ timeout: 3000 });

    // Wait for UI to update
    await ownerPage.waitForTimeout(1500);

    // Member count should decrease
    const memberCountAfter = await ownerPage
      .locator("text=/Members/i")
      .locator("..")
      .locator("p.text-2xl")
      .first()
      .textContent();

    expect(Number(memberCountAfter)).toBeLessThan(Number(memberCountBefore));
  });
});
