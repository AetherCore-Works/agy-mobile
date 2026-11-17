import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(import.meta.url);
const { chromium } = require('/pw/node_modules/playwright');

const BASE_URL = process.env.BASE_URL || 'http://127.0.0.1:8089';

async function assertNoHorizontalOverflow(page, label) {
  const { scrollWidth, innerWidth } = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    innerWidth: window.innerWidth,
  }));
  assert.ok(
    scrollWidth <= innerWidth,
    `[${label}] Horizontal overflow detected: scrollWidth=${scrollWidth} > innerWidth=${innerWidth}`
  );
}

async function run() {
  const browser = await chromium.launch({ headless: true });
  const errors = [];
  const serverErrors = [];

  try {
    // 1. iOS Mobile Viewport (390x844)
    const iosContext = await browser.newContext({
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
    });
    const iosPage = await iosContext.newPage();
    iosPage.on('pageerror', (err) => errors.push(`iOS pageerror: ${err.message}`));
    iosPage.on('response', (res) => {
      if (res.status() >= 500) serverErrors.push(`iOS ${res.status()} ${res.url()}`);
    });

    await iosPage.goto(BASE_URL, { waitUntil: 'networkidle' });
    await assertNoHorizontalOverflow(iosPage, 'iOS initial');

    // Verify textarea font-size >= 16px on mobile to prevent iOS Safari auto-zoom
    const taFontSize = await iosPage.locator('textarea').first().evaluate((el) => {
      return parseFloat(getComputedStyle(el).fontSize);
    });
    assert.ok(taFontSize >= 16, `iOS textarea fontSize=${taFontSize}px (expected >= 16px)`);

    // Open and close Sidebar
    await iosPage.getByRole('button', { name: 'Menu' }).click();
    await iosPage.waitForTimeout(250);
    await assertNoHorizontalOverflow(iosPage, 'iOS sidebar open');
    await iosPage.getByRole('button', { name: 'Fechar menu' }).dispatchEvent('click');
    await iosPage.waitForTimeout(250);

    // Open and close Outbox modal
    await iosPage.getByRole('button', { name: /Outbox/i }).click();
    await iosPage.waitForTimeout(250);
    await assertNoHorizontalOverflow(iosPage, 'iOS outbox open');
    await iosPage.locator('button[title="Fechar"]').click();
    await iosPage.waitForTimeout(250);

    // Verify Theme menu works
    await iosPage.getByRole('button', { name: /Tema e paleta/i }).click();
    await iosPage.waitForTimeout(150);
    await assertNoHorizontalOverflow(iosPage, 'iOS theme menu');
    await iosPage.keyboard.press('Escape');

    await iosContext.close();
    console.log('PASS: iOS mobile viewport checks (390x844)');

    // 2. Desktop Viewport (1280x800)
    const desktopContext = await browser.newContext({
      viewport: { width: 1280, height: 800 },
    });
    const desktopPage = await desktopContext.newPage();
    desktopPage.on('pageerror', (err) => errors.push(`Desktop pageerror: ${err.message}`));
    desktopPage.on('response', (res) => {
      if (res.status() >= 500) serverErrors.push(`Desktop ${res.status()} ${res.url()}`);
    });

    await desktopPage.goto(BASE_URL, { waitUntil: 'networkidle' });
    await assertNoHorizontalOverflow(desktopPage, 'Desktop initial');

    // Toggle Split View
    const splitBtn = desktopPage.locator('button[title*="Dividir"]').first();
    if (await splitBtn.count()) {
      await splitBtn.click();
      await desktopPage.waitForTimeout(200);
      await assertNoHorizontalOverflow(desktopPage, 'Desktop split view');
    }

    await desktopContext.close();
    console.log('PASS: Desktop viewport checks (1280x800)');

    assert.equal(errors.length, 0, `Uncaught page errors: ${errors.join('; ')}`);
    assert.equal(serverErrors.length, 0, `5xx server responses: ${serverErrors.join('; ')}`);
    console.log('ALL PLAYWRIGHT E2E CHECKS PASSED!');
  } finally {
    await browser.close();
  }
}

run().catch((err) => {
  console.error('E2E FAILED:', err);
  process.exit(1);
});
