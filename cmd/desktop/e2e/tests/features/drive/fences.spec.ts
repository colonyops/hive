import { expect, tag, test } from "../../../fixtures/test.js";

// plans/fences.md (testdata/drive) holds one of each built-in fence and two
// wikilinks. The assertions use the fence wrapper, not a component's inner
// markup, so a renderer can change without touching this spec.

const fencesPath = "plans/fences.md";
const fenceLangs = [
  "mermaid",
  "hive-tree",
  "hive-calls",
  "hive-forge",
];

test.describe("drive fences", { tag: [tag.drive] }, () => {
  test("a Drive document renders every built-in fence and its wikilinks", async ({
    page,
  }) => {
    await page.goto(
      `/#/drive?space=e2e%2Frepo&doc=${encodeURIComponent(fencesPath)}`,
    );
    const body = page.getByTestId("drive-page-markdown");
    await expect(
      body.getByRole("heading", { name: "E2E Fence Showcase" }),
    ).toBeVisible();

    for (const lang of fenceLangs) {
      const fence = body.locator(`.canvas-fence[data-fence-lang="${lang}"]`);
      await expect(fence).toHaveCount(1);
      await expect(fence).toBeVisible();
    }
    await expect(body.locator(".canvas-fence")).toHaveCount(fenceLangs.length);

    await expect(body.locator("a.wikilink")).toHaveCount(1);
    await expect(body.locator(".wikilink.is-broken")).toHaveText("no-such-doc");
    await expect(body.locator("a.wikilink.is-broken")).toHaveCount(0);

    await body.locator("a.wikilink").click();
    await expect(page.getByTestId("drive-page-title")).toHaveText(
      "E2E Fence Target",
    );
    await expect(page).toHaveURL(/doc=plans(%2F|\/)fence-target\.md/);
  });
});
