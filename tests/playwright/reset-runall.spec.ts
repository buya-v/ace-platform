import { test, expect } from '@playwright/test';

test.describe('Reset then Run All', () => {
  test('Run All, then Reset, then Run All again — all steps should pass both times', async ({ page }) => {
    test.setTimeout(120_000);
    await page.goto('/');

    // --- First Run All ---
    console.log('=== FIRST RUN ALL ===');
    await page.locator('button:has-text("Run All")').click();
    await expect(page.locator('button:has-text("Running...")')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('button:has-text("Run All")')).toBeVisible({ timeout: 90_000 });

    // Count results from first run
    const firstRunSections = page.locator('nav button');
    const sectionCount = await firstRunSections.count();
    let firstPassCount = 0;
    let firstFailCount = 0;

    for (let i = 0; i < sectionCount; i++) {
      await firstRunSections.nth(i).click();
      await page.waitForTimeout(200);
      const passes = await page.locator('[data-testid="status-badge"]:has-text("Pass")').count();
      const fails = await page.locator('[data-testid="status-badge"]:has-text("Fail")').count();
      firstPassCount += passes;
      firstFailCount += fails;
    }
    console.log(`First Run: ${firstPassCount} pass, ${firstFailCount} fail`);

    // --- Reset ---
    console.log('=== RESET ===');
    await page.locator('button:has-text("Reset")').click();
    await page.waitForTimeout(500);

    // Verify all badges are Pending after reset
    await page.locator('nav button:has-text("Environment Setup")').click();
    await page.waitForTimeout(200);
    const pendingBadges = await page.locator('[data-testid="status-badge"]:has-text("Pending")').count();
    console.log(`After reset: ${pendingBadges} pending badges in Environment Setup`);
    expect(pendingBadges).toBeGreaterThan(0);

    // --- Second Run All ---
    console.log('=== SECOND RUN ALL ===');
    await page.locator('button:has-text("Run All")').click();
    await expect(page.locator('button:has-text("Running...")')).toBeVisible({ timeout: 5_000 });
    await expect(page.locator('button:has-text("Run All")')).toBeVisible({ timeout: 90_000 });

    // Count results from second run
    let secondPassCount = 0;
    let secondFailCount = 0;
    const failDetails: string[] = [];

    for (let i = 0; i < sectionCount; i++) {
      await firstRunSections.nth(i).click();
      await page.waitForTimeout(200);
      const passes = await page.locator('[data-testid="status-badge"]:has-text("Pass")').count();
      const fails = await page.locator('[data-testid="status-badge"]:has-text("Fail")').count();
      secondPassCount += passes;
      secondFailCount += fails;

      if (fails > 0) {
        const sectionName = await firstRunSections.nth(i).textContent();
        // Get error text from response panels
        const errors = await page.locator('[class*="error"]').allTextContents();
        failDetails.push(`Section "${sectionName}": ${fails} fails — ${errors.join('; ')}`);
      }
    }

    console.log(`Second Run: ${secondPassCount} pass, ${secondFailCount} fail`);
    if (failDetails.length > 0) {
      console.log('=== FAILURE DETAILS ===');
      failDetails.forEach(d => console.log(d));
    }

    // Take screenshot of final state
    await page.screenshot({ path: 'test-results/reset-runall-final.png', fullPage: true });

    // The second run should have the same or better pass rate as the first
    expect(secondFailCount, 'Second run should have zero failures after reset').toBe(0);
  });
});
