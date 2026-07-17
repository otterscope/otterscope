import { chromium } from "/home/kalin/otterscope/web/node_modules/playwright/index.mjs";

const BASE = "http://localhost:8324";
const OUT = "/home/kalin/otterscope/docs/screenshots";

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });

// Headless chromium has no emoji font; swap the header otter for a Twemoji img.
const styled = async (url) => {
  await page.goto(url);
  await page.evaluate(() => {
    const h1 = document.querySelector("header h1");
    if (h1 && h1.textContent.includes("\u{1F9A6}")) {
      h1.innerHTML = h1.innerHTML.replace(
        "\u{1F9A6}",
        '<img src="https://cdn.jsdelivr.net/gh/jdecked/twemoji@latest/assets/72x72/1f9a6.png" style="height:1.1em;vertical-align:-0.15em" />',
      );
    }
  });
  await page.waitForTimeout(400);
};

await styled(BASE + "/");
await page.waitForSelector("table.runs");
await page.screenshot({ path: `${OUT}/runs-list.png` });

const runs = await (await fetch(BASE + "/api/runs?limit=100")).json();
const rich = runs.runs.find((r) => r.llmCalls >= 2 && r.toolCalls >= 2) ?? runs.runs[0];
await styled(`${BASE}/runs/${rich.id}`);
await page.waitForSelector(".timeline");
await page.click(".step:has(.kind.llm)");
await page.waitForSelector(".inspector");
await page.screenshot({ path: `${OUT}/run-detail.png`, fullPage: true });

await styled(BASE + "/compare?a_model=claude-sonnet-5&b_model=gpt-5.4-mini");
await page.waitForSelector(".compare-table");
await page.waitForTimeout(600);
await page.screenshot({ path: `${OUT}/compare.png` });

await styled(BASE + "/alerts");
await page.waitForSelector("table.runs");
await page.waitForTimeout(300);
await page.screenshot({ path: `${OUT}/alerts.png` });

await styled(BASE + "/audit");
await page.waitForSelector("table.runs");
await page.waitForTimeout(300);
await page.screenshot({ path: `${OUT}/audit.png` });

await browser.close();
console.log("screenshots written");
