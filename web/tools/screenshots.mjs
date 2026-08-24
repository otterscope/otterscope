// Regenerates the README screenshots. Run it from web/ (`npm run screenshots`)
// so the bare `playwright` specifier resolves against web/node_modules — this
// file lives inside web/ for exactly that reason, and playwright is a
// devDependency there. It was previously in a top-level tools/ directory
// importing playwright by absolute path, which meant nothing connected the
// dependency to its only consumer, and the dependency got removed as unused
// (#124).
//
// Needs an otterscope already serving sample data:
//   otterscope sample -db demo.db && otterscope serve -db demo.db -listen 127.0.0.1:8324
// Override the address with OTTERSCOPE_URL.
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";
import { fileURLToPath } from "node:url";

const BASE = process.env.OTTERSCOPE_URL ?? "http://localhost:8324";
const OUT = fileURLToPath(new URL("../../docs/screenshots", import.meta.url));
mkdirSync(OUT, { recursive: true });

// Fail with something readable rather than a Playwright timeout if the server
// is not up — the previous version just hung on waitForSelector.
try {
  const res = await fetch(BASE + "/healthz");
  if (!res.ok) throw new Error("status " + res.status);
} catch (err) {
  console.error(`No otterscope at ${BASE} (${err.message}).
Start one with sample data first, or set OTTERSCOPE_URL.`);
  process.exit(1);
}

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
