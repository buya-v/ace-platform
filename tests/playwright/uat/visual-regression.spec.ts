/**
 * GarudaX Visual Regression UAT — Screenshot Baseline Tests
 *
 * Captures full-page screenshot baselines for each major admin portal page
 * using Playwright's built-in toHaveScreenshot() which:
 *   - Creates .png baseline files on first run (--update-snapshots)
 *   - Diffs against baselines on subsequent runs
 *   - Reports pixel-level differences as test failures
 *
 * Viewports tested:
 *   - Desktop: 1920×1080 — 10 pages, 3% max pixel diff
 *   - Tablet:  1024×768  — 2 pages, 5% max pixel diff (more rendering variance)
 *
 * All tests skip gracefully when the admin portal is unreachable.
 * SPA navigation via sidebar clicks preserves in-memory React auth tokens.
 */

import { test, expect } from '@playwright/test';
import { loginAsAdmin, navigateTo, waitForData, isPortalAvailable } from './helpers';

// ---------------------------------------------------------------------------
// Desktop Visual Regression — 1920×1080
// ---------------------------------------------------------------------------

test.describe('Visual Regression UAT', () => {
  test.use({ viewport: { width: 1920, height: 1080 } });

  test.beforeEach(async ({ page }) => {
    test.skip(!(await isPortalAvailable(page)), 'Portal not reachable');
    await loginAsAdmin(page);
  });

  test('Dashboard monitoring — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'monitoring');
    await waitForData(page);
    await expect(page).toHaveScreenshot('monitoring-desktop.png', {
      maxDiffPixelRatio: 0.03,
      timeout: 15000,
    });
  });

  test('Participants page — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'participants');
    await waitForData(page);
    await expect(page).toHaveScreenshot('participants-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Circuit Breakers — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'circuit-breakers');
    await waitForData(page);
    await expect(page).toHaveScreenshot('circuit-breakers-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Margin Calls — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'margin');
    await waitForData(page);
    await expect(page).toHaveScreenshot('margin-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Settlement — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'settlement');
    await waitForData(page);
    await expect(page).toHaveScreenshot('settlement-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Surveillance — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'surveillance');
    await waitForData(page);
    await expect(page).toHaveScreenshot('surveillance-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Tickets — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'tickets');
    await waitForData(page);
    await expect(page).toHaveScreenshot('tickets-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Fee Management — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'fees');
    await waitForData(page);
    await expect(page).toHaveScreenshot('fees-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Reports — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'reports');
    await waitForData(page);
    await expect(page).toHaveScreenshot('reports-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });

  test('Audit Log — desktop baseline', async ({ page }) => {
    await navigateTo(page, 'audit');
    await waitForData(page);
    await expect(page).toHaveScreenshot('audit-desktop.png', {
      maxDiffPixelRatio: 0.03,
    });
  });
});

// ---------------------------------------------------------------------------
// Tablet Visual Regression — 1024×768
// ---------------------------------------------------------------------------

test.describe('Visual Regression UAT — Tablet', () => {
  test.use({ viewport: { width: 1024, height: 768 } });

  test.beforeEach(async ({ page }) => {
    test.skip(!(await isPortalAvailable(page)), 'Portal not reachable');
    await loginAsAdmin(page);
  });

  test('Dashboard monitoring — tablet baseline', async ({ page }) => {
    await navigateTo(page, 'monitoring');
    await waitForData(page);
    await expect(page).toHaveScreenshot('monitoring-tablet.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('Participants — tablet baseline', async ({ page }) => {
    await navigateTo(page, 'participants');
    await waitForData(page);
    await expect(page).toHaveScreenshot('participants-tablet.png', {
      maxDiffPixelRatio: 0.05,
    });
  });
});
