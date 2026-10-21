import { expect, test } from "./fixtures.js";

// serve.sh seeds the e2e/repo space with one plan and one canvas
// (fixtures/drive). The drive is the engine's, not the Chats area's, so it
// works in this server build even though Chats is unavailable here.

const planPath = "plans/2026-10-11-plan.md";
const planTitle = "E2E Drive Plan";

test("the Drive page opens a document from a space and finds it from another", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.getByTestId("feed-item").first()).toBeVisible();

  await page.getByTestId("titlebar-menu").click();
  await page.getByTestId("titlebar-menu-drive").click();
  await expect(page.getByTestId("drive-page")).toBeVisible();
  await expect(page.getByTestId("feed-item")).toHaveCount(0);
  await expect(page).toHaveURL(/#\/drive/);

  await page.getByTestId("drive-page-space").click();
  await expect(
    page.getByTestId("drive-page-space-group-Repositories"),
  ).toBeVisible();
  await page.getByTestId("drive-page-space-option-e2e/repo").click();

  await page.getByTestId("drive-page-row-plans").click();
  await page.getByTestId(`drive-page-row-${planPath}`).click();
  await expect(page.getByTestId("drive-page-title")).toHaveText(planTitle);
  await expect(
    page
      .getByTestId("drive-page-markdown")
      .getByRole("heading", { name: planTitle }),
  ).toBeVisible();
  await expect(page.getByTestId("drive-page-frontmatter-type")).toContainText(
    "plan",
  );

  await page.getByTestId("drive-page-space").click();
  await page.getByTestId("drive-page-space-option-shared").click();
  await expect(page.getByTestId("drive-page-title")).not.toHaveText(planTitle);

  await page.getByTestId("drive-page-search").fill("E2E Drive");
  await page.getByTestId(`drive-page-hit-e2e/repo:${planPath}`).click();
  await expect(page.getByTestId("drive-page-title")).toHaveText(planTitle);
  await expect(page).toHaveURL(
    /#\/drive\?space=e2e(%2F|\/)repo&doc=plans(%2F|\/)2026-10-11-plan\.md/,
  );

  await page.keyboard.press("ControlOrMeta+Shift+P");
  await expect(page.getByTestId("drive-page")).toHaveCount(0);
  await expect(page.getByTestId("feed-item").first()).toBeVisible();
});

test("a comment on a Drive document survives a reload, then clears", async ({
  page,
}) => {
  await page.goto(
    `/#/drive?space=e2e%2Frepo&doc=${encodeURIComponent(planPath)}`,
  );
  const body = page.getByTestId("drive-page-markdown");
  await expect(body.getByRole("heading", { name: planTitle })).toBeVisible();

  // "This plan" is on line 8 of the file: the front matter takes 4.
  await body
    .locator("p")
    .first()
    .evaluate((paragraph) => {
      const text = paragraph.firstChild!;
      const range = document.createRange();
      range.setStart(text, 0);
      range.setEnd(text, "This plan".length);
      const selection = window.getSelection()!;
      selection.removeAllRanges();
      selection.addRange(range);
    });
  await expect(page.getByTestId("drive-page-comment-popover")).toContainText(
    "This plan",
  );
  await page.getByTestId("drive-page-comment-text").fill("Name the plan");
  await page.getByTestId("drive-page-comment-save").click();

  const row = page.getByTestId("drive-page-review-card-1");
  await expect(row).toContainText("L8");
  await expect(row).toContainText("Name the plan");
  await expect(body.locator(".has-review-comment")).toHaveCount(1);

  await page.reload();
  await expect(page.getByTestId("drive-page-review-card-1")).toContainText(
    "Name the plan",
  );
  await expect(body.locator(".has-review-comment")).toHaveCount(1);

  await page.getByTestId("drive-page-review-copy").click();
  await expect(page.getByTestId("drive-page-review-card-1")).toBeVisible();

  await page.getByTestId("drive-page-review-clear").click();
  await page.getByTestId("drive-page-review-clear-confirm").click();
  await expect(page.getByTestId("drive-page-review-header")).toHaveCount(0);
  await expect(body.locator(".has-review-comment")).toHaveCount(0);
});

// A srcdoc frame inherits the app's base URL, so without its own base a
// fragment or relative link loads Hive inside the frame.
test("links in an html document stay inside the document", async ({
  page,
  browserName,
}) => {
  await page.goto(
    `/#/drive?space=e2e%2Frepo&doc=${encodeURIComponent("pages/links.html")}`,
  );
  const frameElement = page.getByTestId("drive-page-frame");
  const doc = page.frameLocator('[data-testid="drive-page-frame"]');
  const frameURL = async () =>
    (await (await frameElement.elementHandle())!.contentFrame())!.url();

  await doc.locator("#to-section").click();
  await expect(doc.locator("#section")).toBeInViewport();
  await expect.poll(frameURL).toBe("about:srcdoc#section");
  await expect(doc.locator("#app")).toHaveCount(0);

  await doc.locator("#to-plan").click();
  await expect.poll(frameURL).not.toMatch(/^http/);
  await expect(page).toHaveURL(/doc=pages(%2F|\/)links\.html/);
  // A relative path cannot resolve against about:srcdoc. WebKit, which both
  // shipped webviews are, then does nothing; Chromium blanks the frame.
  if (browserName === "webkit") {
    await expect.poll(frameURL).toBe("about:srcdoc#section");
    await expect(doc.locator("#to-plan")).toBeAttached();
  }
});
