import { chromium } from "playwright";

const browser = await chromium.launch();
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
// Headless chromium ships no emoji font; swap the otter for a Twemoji image
// so the header doesn't show a tofu box.
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
const out = "docs/screenshots";

await styled("http://localhost:8317/");
await page.waitForSelector("table.runs");
await page.screenshot({ path: `${out}/runs-list.png` });

const runs = await (await fetch("http://localhost:8317/api/runs?limit=100")).json();
const rich = runs.runs.find((r) => r.llmCalls >= 2 && r.toolCalls >= 2) ?? runs.runs[0];
await styled(`http://localhost:8317/runs/${rich.id}`);
await page.waitForSelector(".timeline");
await page.click(".step:has(.kind.llm)");
await page.waitForSelector(".inspector");
await page.screenshot({ path: `${out}/run-detail.png`, fullPage: true });

await styled(
  "http://localhost:8317/compare?a_model=claude-sonnet-5&b_model=gpt-5.4-mini",
);
await page.waitForSelector(".compare-table");
await page.waitForTimeout(600);
await page.screenshot({ path: `${out}/compare.png` });

await browser.close();
console.log("screenshots written");
