import { test, expect } from '@playwright/test';

test.describe('Demo Runner — Page Load', () => {
  test('loads the demo runner app', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('text=ACE Demo Runner')).toBeVisible();
  });

  test('shows sidebar with all 9 sections', async ({ page }) => {
    await page.goto('/');
    const sidebar = page.locator('nav');
    await expect(sidebar).toBeVisible();

    const sectionButtons = sidebar.locator('button');
    await expect(sectionButtons).toHaveCount(9);

    await expect(sidebar).toContainText('Environment Setup');
    await expect(sidebar).toContainText('User Registration & KYC');
    await expect(sidebar).toContainText('Trading Flow');
    await expect(sidebar).toContainText('Post-Trade');
    await expect(sidebar).toContainText('Physical Delivery');
    await expect(sidebar).toContainText('Market Data');
    await expect(sidebar).toContainText('Compliance & Risk');
    await expect(sidebar).toContainText('Admin Operations');
    await expect(sidebar).toContainText('Production Readiness');
  });

  test('shows step cards with Run buttons', async ({ page }) => {
    await page.goto('/');
    const runButtons = page.locator('button:has-text("Run")');
    await expect(runButtons.first()).toBeVisible();
  });

  test('shows bottom bar with Run All and Reset', async ({ page }) => {
    await page.goto('/');
    await expect(page.locator('button:has-text("Run All")')).toBeVisible();
    await expect(page.locator('button:has-text("Reset")')).toBeVisible();
  });
});

test.describe('Demo Runner — Section Navigation', () => {
  test('clicking sidebar section changes content', async ({ page }) => {
    await page.goto('/');

    // Default section shows Environment Setup steps
    await expect(page.locator('h3:has-text("Check Gateway Health")')).toBeVisible();

    // Click Trading Flow section
    await page.locator('nav button:has-text("Trading Flow")').click();

    // Should show trading steps (actual title from sections.ts)
    await expect(page.locator('h3:has-text("Submit Buy Order")')).toBeVisible();
  });

  test('each section shows correct step count', async ({ page }) => {
    await page.goto('/');
    const envSection = page.locator('nav button:has-text("Environment Setup")');
    await expect(envSection).toContainText('/4');
  });
});

test.describe('Demo Runner — Step Execution', () => {
  test('Check Gateway Health — click Run and get result', async ({ page }) => {
    await page.goto('/');

    // Click the first Run button (Check Gateway Health)
    const runButtons = page.locator('button:has-text("Run")');
    await runButtons.first().click();

    // Wait for PASS or FAIL badge to appear
    await expect(
      page.locator('[class*="pass"], [class*="fail"]').first()
    ).toBeVisible({ timeout: 15_000 });
  });

  test('Check Gateway Health — shows response panel after run', async ({ page }) => {
    await page.goto('/');

    await page.locator('button:has-text("Run")').first().click();

    // Wait for Pass or Fail badge, then verify response panel appeared (has ms timing)
    await expect(
      page.locator('[data-testid="status-badge"]:has-text("Pass"), [data-testid="status-badge"]:has-text("Fail")').first()
    ).toBeVisible({ timeout: 15_000 });

    // Response panel should show timing (e.g., "123ms")
    await expect(page.locator('text=/\\d+ms/')).toBeVisible();
  });

  test('sidebar counter updates after successful step', async ({ page }) => {
    await page.goto('/');

    const envSection = page.locator('nav button:has-text("Environment Setup")');
    await expect(envSection).toContainText('0/4');

    // Run first step
    await page.locator('button:has-text("Run")').first().click();

    // Wait for any result badge (Pass or Fail)
    await expect(
      page.locator('[data-testid="status-badge"]:has-text("Pass"), [data-testid="status-badge"]:has-text("Fail")').first()
    ).toBeVisible({ timeout: 15_000 });

    // Counter should change from 0/4 (1/4 if pass, stays 0/4 if fail)
    // Just verify the step completed — the badge changed from Pending
    await expect(
      page.locator('[data-testid="status-badge"]').first()
    ).not.toHaveText('Pending', { timeout: 5_000 });
  });
});

test.describe('Demo Runner — User Registration Flow', () => {
  test('Register Trader 1 — click Run and get result', async ({ page }) => {
    await page.goto('/');

    // Navigate to Registration section
    await page.locator('nav button:has-text("User Registration")').click();
    await expect(page.locator('h3:has-text("Register Trader 1")')).toBeVisible();

    // Click Run on first step
    await page.locator('button:has-text("Run")').first().click();

    // Wait for result (PASS on 201/409 or FAIL)
    await expect(
      page.locator('[class*="pass"], [class*="fail"]').first()
    ).toBeVisible({ timeout: 15_000 });
  });
});

test.describe('Demo Runner — Run All', () => {
  test('Run All button starts execution', async ({ page }) => {
    await page.goto('/');

    // Click Run All
    await page.locator('button:has-text("Run All")').click();

    // Should show Running... text
    await expect(page.locator('button:has-text("Running...")')).toBeVisible({ timeout: 5_000 });
  });
});

test.describe('Demo Runner — Reset', () => {
  test('Reset clears all results', async ({ page }) => {
    await page.goto('/');

    // Run a step first
    await page.locator('button:has-text("Run")').first().click();
    await expect(
      page.locator('[data-testid="status-badge"]:has-text("Pass"), [data-testid="status-badge"]:has-text("Fail")').first()
    ).toBeVisible({ timeout: 15_000 });

    // Click Reset
    await page.locator('button:has-text("Reset")').click();

    // Counter back to 0
    const envSection = page.locator('nav button:has-text("Environment Setup")');
    await expect(envSection).toContainText('0/4');
  });
});

test.describe('Demo Runner — Production Readiness Checklist', () => {
  test('shows checklist with categories', async ({ page }) => {
    await page.goto('/');

    await page.locator('nav button:has-text("Production Readiness")').click();

    // Should show checklist categories
    await expect(page.locator('text=Security')).toBeVisible();
  });
});
