import { expect, tag, test } from "../../../fixtures/test.js";

// testdata/applets/e2e-fence declares only a markdown surface for `e2e-card`, and
// testdata/drive/plans/applet-fence.md holds one such block.

test.describe("applet markdown fence", { tag: [tag.applets, tag.drive] }, () => {
  test("a markdown applet renders its fence in a Drive document", async ({
    page,
  }) => {
    await page.goto(
      `/#/drive?space=e2e%2Frepo&doc=${encodeURIComponent("plans/applet-fence.md")}`,
    );
    const body = page.getByTestId("drive-page-markdown");
    const fence = body.locator('.canvas-fence[data-fence-lang="e2e-card"]');
    await expect(fence).toHaveCount(1);

    const content = fence.frameLocator("iframe");
    await expect(content.locator("#surface")).toHaveText("markdown");
    await expect(content.locator("#lang")).toHaveText("e2e-card");
    await expect(content.locator("#data")).toContainText('"title":"Hello card"');
    await expect(content.locator("#data")).toContainText('"count":3');

    // The page asks for 200px; the block starts at 120px.
    await expect
      .poll(async () => (await fence.locator("iframe").boundingBox())?.height)
      .toBe(200);
  });

  test("a markdown-only applet adds no palette row", async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("feed-item").first()).toBeVisible();
    await page.keyboard.press("Meta+k");
    await page.getByTestId("command-palette-input").fill("E2E fence");
    await expect(
      page
        .getByTestId("command-palette-command-title")
        .filter({ hasText: "E2E fence" }),
    ).toHaveCount(0);
  });
});
