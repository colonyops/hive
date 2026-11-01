import type { Page } from "@playwright/test";
import { screenshotPath, testdataPath } from "../../../fixtures/paths.js";
import { expect, tag, test } from "../../../fixtures/test.js";

async function openApplet(page: Page, title: string): Promise<void> {
  await page.keyboard.press("Meta+k");
  await page.getByTestId("command-palette-input").fill(title);
  await expect(
    page.getByTestId("command-palette-command-title").first(),
  ).toHaveText(title);
  await page.keyboard.press("Enter");
}

test.describe("applets", { tag: [tag.applets] }, () => {
  test.beforeEach(async ({ page }) => {
    await page.goto("/");
    await expect(page.getByTestId("feed-item").first()).toBeVisible();
  });

  test("an applet whose title has the query comes first under its own header", async ({
    page,
  }) => {
    await page.keyboard.press("Meta+k");
    await page.getByTestId("command-palette-input").fill("E2E page");
    await expect(
      page.getByTestId("command-palette-command-title").first(),
    ).toHaveText("E2E page");
    await expect(
      page
        .getByTestId("command-palette")
        .getByText("Applets", { exact: true })
        .first(),
    ).toBeVisible();
  });

  test("the palette has a tab of only applets", async ({ page }) => {
    await page.keyboard.press("Meta+k");
    await page.getByTestId("command-palette-input").fill("+");
    const titles = page.getByTestId("command-palette-command-title");
    await expect(titles.filter({ hasText: "Applet Builder" })).toHaveCount(1);
    await expect(titles.filter({ hasText: "E2E page" })).toHaveCount(1);
    await expect(titles.filter({ hasText: /^E2E bar$/ })).toHaveCount(1);
    await expect(titles.filter({ hasText: "E2E bar ping" })).toHaveCount(1);
    await expect(titles).toHaveCount(4);
  });

  test("the builder fits on one screen and copies a prompt that asks what to build", async ({
    page,
  }) => {
    await openApplet(page, "Applet Builder");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    await expect(frame.getByTestId("builder-style-hive")).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    await frame.getByTestId("builder-style-reference").click();
    await expect(frame.getByTestId("builder-copy")).toBeDisabled();
    await frame.getByTestId("builder-reference").fill("an old Mac");
    await frame.getByTestId("builder-copy").click();

    await expect(page.getByTestId("applet-page-overlay")).toHaveCount(0);
    await expect(page.getByText("Prompt copied")).toBeVisible();
  });

  test("settings lists the applets and what they have running", async ({
    page,
    browserName,
  }) => {
    await page.goto("/#/settings/applets");
    await expect(
      page.getByTestId("settings-applet-applet-builder"),
    ).toContainText("Ships with Hive");
    await expect(page.getByTestId("settings-applet-e2e-page")).toBeVisible();
    await page
      .getByTestId("settings-applets")
      .screenshot({ path: screenshotPath(`applets-settings-${browserName}.png`) });
    await expect(
      page.getByTestId("settings-applets-processes-empty"),
    ).toBeVisible();

    await page.getByTestId("settings-applet-applet-builder-open").click();
    await expect(
      page
        .frameLocator('[data-testid="applet-frame"]')
        .getByTestId("builder-copy"),
    ).toBeVisible();
  });

  test("a page reaches the machine through window.hive and keeps its localStorage", async ({
    page,
  }) => {
    await openApplet(page, "E2E page");
    const result = page
      .frameLocator('[data-testid="applet-frame"]')
      .locator("#result");
    await expect(result).toContainText('"file":"written"');

    const first = JSON.parse((await result.textContent()) ?? "{}") as Record<
      string,
      unknown
    >;
    expect(first).toMatchObject({
      fromModule: "module loaded",
      bypassed: false,
      context: "e2e-page",
      exec: { stdout: "out\n", stderr: "err\n", code: 3 },
      file: "written",
      spawn: { out: "got hello\n", code: 5 },
    });
    await expect(page.getByText("page toast")).toBeVisible();

    // The save is debounced, and closing drops a pending one.
    const visits = Number(first.visits);
    await expect
      .poll(async () =>
        (await page.request.get("/__applet/e2e-page/index.html")).text(),
      )
      .toContain(`"visits":"${visits + 1}"`);

    await page.getByTestId("applet-page-close").click();
    await expect(page.getByTestId("applet-page-overlay")).toHaveCount(0);

    await openApplet(page, "E2E page");
    await expect(result).toContainText(`"visits":${visits + 1}`);
  });

  test("a page the frame navigates to gets no answer from the bridge", async ({
    page,
  }) => {
    await openApplet(page, "E2E page");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    await expect(frame.locator("#result")).toContainText('"file":"written"');

    await frame.locator("#navigate").click();
    await expect(frame.locator("#left")).toHaveText("ignored");

    await page.getByTestId("applet-page-close").click();
    await openApplet(page, "E2E page");
    await expect(frame.locator("#result")).toContainText('"bypassed":false');
  });

  test("an applet that is off does not run, serve or show in the palette", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    const enabled = page.getByTestId("settings-applet-e2e-page-enabled");
    await enabled.click();
    await expect(page.getByTestId("settings-applet-e2e-page-state")).toHaveText(
      "Off",
    );
    await expect(page.getByTestId("settings-applet-e2e-page-open")).toBeDisabled();
    expect(
      (await page.request.get("/__applet/e2e-page/index.html")).status(),
    ).toBe(404);

    await page.keyboard.press("Meta+k");
    await page.getByTestId("command-palette-input").fill("E2E page");
    await expect(
      page
        .getByTestId("command-palette-command-title")
        .filter({ hasText: "E2E page" }),
    ).toHaveCount(0);
    await page.keyboard.press("Escape");

    await enabled.click();
    await expect(page.getByTestId("settings-applet-e2e-page-state")).toHaveCount(
      0,
    );
    expect(
      (await page.request.get("/__applet/e2e-page/index.html")).status(),
    ).toBe(200);
    await openApplet(page, "E2E page");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    await expect(frame.locator("#result")).toContainText('"file":"written"');
  });

  test("a foreign page framing the applet's own page gets no token", async ({
    page,
  }) => {
    await openApplet(page, "E2E page");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    await expect(frame.locator("#result")).toContainText('"file":"written"');

    await frame.locator("#frame-own-page").click();
    await expect(frame.locator("#left")).toHaveText("no token");

    await page.getByTestId("applet-page-close").click();
    await openApplet(page, "E2E page");
    await expect(frame.locator("#result")).toContainText('"bypassed":false');
  });

  test("a page reads its own data file, and a foreign page in its frame cannot", async ({
    page,
    browserName,
  }) => {
    await openApplet(page, "E2E page");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    // Chromium sends an opaque frame no Referer, so it cannot tell the two apart and refuses
    // both. WebKit, which the app runs on, sends one.
    await expect(frame.locator("#result")).toContainText(
      browserName === "webkit"
        ? '"ownData":{"from":"data file"}'
        : '"ownData":"blocked"',
    );

    await frame.locator("#read-data").click();
    await expect(frame.locator("#left")).toHaveText("blocked");
  });

  test("the Wails runtime refuses a call that is not the app's own", async ({
    page,
  }) => {
    const call = `/wails/runtime?object=0&method=0&args=${encodeURIComponent(
      JSON.stringify({
        "call-id": "e2e",
        methodID: 2190116655,
        args: ["e2e-page", { command: "touch bypassed" }],
      }),
    )}`;
    const refused = [
      await page.request.get(call),
      await page.request.post(call),
      await page.request.post(call, {
        headers: { "x-wails-client-id": "e2e", Origin: "null" },
      }),
      await page.request.post(call, {
        headers: { "x-wails-client-id": "e2e", Origin: "https://evil.example" },
      }),
    ];
    expect(refused.map((response) => response.status())).toEqual([
      403, 403, 403, 403,
    ]);

    await openApplet(page, "E2E page");
    const frame = page.frameLocator('[data-testid="applet-frame"]');
    await expect(frame.locator("#result")).toContainText('"bypassed":false');
  });

  test("a title bar page runs once turned on, opens a dropdown, and talks to it", async ({
    page,
    browserName,
  }) => {
    const slot = page.getByTestId("titlebar-applet-e2e-bar");
    await expect(slot).toHaveCount(0);

    await page.goto("/#/settings/applets");
    const toggle = page.getByTestId("settings-applet-e2e-bar-titlebar");
    await expect(
      page.getByTestId("settings-applet-e2e-page-titlebar"),
    ).toHaveCount(0);
    await toggle.click();
    await expect(slot).toBeVisible();

    const bar = page.frameLocator('[data-testid="applet-titlebar-frame"]');
    await expect(bar.locator("#surface")).toHaveText("titlebar");
    await expect(slot).toHaveCSS("width", "60px");
    const header = await page.getByTestId("titlebar").boundingBox();
    const box = await slot.boundingBox();
    expect(box!.y - header!.y).toBe(
      header!.y + header!.height - (box!.y + box!.height),
    );

    await bar.locator("#grow").click();
    await expect(slot).toHaveCSS("width", "90px");

    await bar.locator("#menu").click();
    const dropdown = page.getByTestId("applet-dropdown");
    await expect(dropdown).toBeVisible();
    const menu = page.frameLocator('[data-testid="applet-dropdown-frame"]');
    await expect(menu.locator("#surface")).toHaveText("dropdown");
    await menu.locator("#send").click();
    await expect(bar.locator("#got")).toHaveText("hello from the dropdown");
    await expect(bar.locator("#stored")).toHaveText("from the dropdown");

    await bar.locator("#menu").click();
    await expect(dropdown).toHaveCount(0);

    await bar.locator("#menu").click();
    await expect(dropdown).toBeVisible();
    await page.getByTestId("titlebar").click({ position: { x: 400, y: 20 } });
    await expect(dropdown).toHaveCount(0);

    await page.screenshot({
      path: screenshotPath(`applet-titlebar-${browserName}.png`),
      clip: header!,
    });

    await bar.locator("#chord").focus();
    await page.keyboard.press("Meta+k");
    await expect(page.getByTestId("command-palette-input")).toBeVisible();
    await page.keyboard.press("Escape");

    await bar.locator("#command").click();
    await expect(page).toHaveURL(/#\/settings\/keybindings/);

    await page.goto("/#/settings/applets");
    await toggle.click();
    await expect(slot).toHaveCount(0);
  });

  test("a page adds to the palette, and a declared command reaches the page that is running", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    const toggle = page.getByTestId("settings-applet-e2e-bar-titlebar");
    await toggle.click();
    const bar = page.frameLocator('[data-testid="applet-titlebar-frame"]');
    await expect(bar.locator("#surface")).toHaveText("titlebar");

    await openApplet(page, "E2E bar ping");
    await expect(bar.locator("#command-run")).toHaveText("ping");
    await expect(page.getByTestId("applet-page-overlay")).toHaveCount(0);

    await openApplet(page, "E2E bar live row");
    await expect(bar.locator("#command-run")).toHaveText("live");

    await page.goto("/#/settings/applets");
    await toggle.click();
    await expect(page.getByTestId("titlebar-applet-e2e-bar")).toHaveCount(0);

    await page.keyboard.press("Meta+k");
    await page.getByTestId("command-palette-input").fill("E2E bar live row");
    await expect(
      page
        .getByTestId("command-palette-command-title")
        .filter({ hasText: "live row" }),
    ).toHaveCount(0);
    await page.keyboard.press("Escape");

    await openApplet(page, "E2E bar ping");
    await expect(
      page.frameLocator('[data-testid="applet-frame"]').locator("#command"),
    ).toHaveText("ping");
    await page.getByTestId("applet-page-close").click();
  });

  test("a bound chord works from inside the page, and only there for what a panel allows", async ({
    page,
  }) => {
    const bind = async (title: string, chord: string) => {
      await page.goto("/#/settings/keybindings");
      await page.getByTestId("keybinding-filter").fill(title);
      const row = page
        .getByTestId("keybinding-row")
        .filter({ hasText: title })
        .first();
      await row.getByTestId("keybinding-add").click();
      await page.keyboard.press(chord);
      await row.getByTestId("keybinding-capture").click();
      await expect(row.getByTestId("keybinding-combo")).toHaveCount(1);
      return row;
    };
    const unbind = async (title: string) => {
      await page.goto("/#/settings/keybindings");
      await page.getByTestId("keybinding-filter").fill(title);
      await page
        .getByTestId("keybinding-row")
        .filter({ hasText: title })
        .first()
        .getByTestId("keybinding-remove")
        .click();
    };

    await bind("E2E page", "Meta+Shift+y");
    await bind("E2E bar ping", "Meta+Shift+u");
    await page.goto("/#/settings/applets");
    const toggle = page.getByTestId("settings-applet-e2e-bar-titlebar");
    await toggle.click();
    const bar = page.frameLocator('[data-testid="applet-titlebar-frame"]');
    await expect(bar.locator("#surface")).toHaveText("titlebar");

    // Focus is in the title bar page, so these only arrive if it hands them on.
    await bar.locator("#chord").focus();
    await page.keyboard.press("Meta+Shift+u");
    await expect(bar.locator("#command-run")).toHaveText("ping");
    await bar.locator("#chord").focus();
    await page.keyboard.press("Meta+Shift+y");
    const overlay = page.getByTestId("applet-page-overlay");
    await expect(overlay).toBeVisible();

    // The panel has taken focus.
    const panel = page.frameLocator('[data-testid="applet-frame"]');
    await panel.locator("body").click();
    await page.keyboard.press("Meta+k");
    await expect(page.getByTestId("command-palette-input")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("command-palette-input")).toHaveCount(0);
    await panel.locator("body").click();
    await page.keyboard.press("Meta+Shift+y");
    await expect(overlay).toHaveCount(0);

    await page.goto("/#/settings/applets");
    await toggle.click();
    await unbind("E2E page");
    await unbind("E2E bar ping");
  });

  test("an import warns first, cancels by default, and cancel leaves nothing behind", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    await page
      .getByTestId("settings-applets-import-input")
      .setInputFiles(
        testdataPath("applet-archives", "e2e-imported.hiveapplet"),
      );

    const dialog = page.getByTestId("applet-import");
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByText("Only import applets from people you trust"),
    ).toBeVisible();
    const warning = page.getByTestId("applet-import-warning");
    await expect(warning).toContainText("run any command or program");
    await expect(warning).toContainText(
      "read, change and delete any of your files",
    );
    await expect(warning).toContainText("reach the network");

    await expect(page.getByTestId("applet-import-title")).toHaveText(
      "E2E imported",
    );
    const files = page.getByTestId("applet-import-files");
    for (const file of ["applet.yaml", "index.html", "bar.html"])
      await expect(files).toContainText(file);
    const runs = page.getByTestId("applet-import-runs");
    await expect(runs).toContainText("Nothing runs on import");
    await expect(runs).toContainText("bar.html");
    await expect(runs).toContainText("stays off");
    await expect(runs).toContainText("E2E imported go");

    // Enter on arrival is a cancel, and importing takes the tick first.
    await expect(page.getByTestId("applet-import-cancel")).toBeFocused();
    const confirm = page.getByTestId("applet-import-confirm");
    await expect(confirm).toBeDisabled();
    await page.getByTestId("applet-import-trust").check();
    await expect(confirm).toBeEnabled();
    await page.getByTestId("applet-import-trust").uncheck();
    await page.getByTestId("applet-import-cancel").focus();
    await page.keyboard.press("Enter");
    await expect(dialog).toHaveCount(0);

    await page.goto("/#/settings/applets");
    await expect(page.getByTestId("settings-applet-e2e-page")).toBeVisible();
    await expect(page.getByTestId("settings-applet-e2e-imported")).toHaveCount(
      0,
    );
    await expect(page.getByTestId("titlebar-applet-e2e-imported")).toHaveCount(
      0,
    );
    const onDisk = await page.request.get("/__applet/e2e-imported/applet.yaml");
    expect(onDisk.status()).toBe(404);
  });

  test("deleting an applet asks first, then removes its files and its row", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    await page
      .getByTestId("settings-applets-import-input")
      .setInputFiles(
        testdataPath("applet-archives", "e2e-imported.hiveapplet"),
      );
    await page.getByTestId("applet-import-trust").check();
    await page.getByTestId("applet-import-confirm").click();
    const row = page.getByTestId("settings-applet-e2e-imported");
    await expect(row).toBeVisible();
    await expect(
      page.getByTestId("settings-applet-applet-builder-delete"),
    ).toHaveCount(0);

    await page.getByTestId("settings-applet-e2e-imported-delete").click();
    await page.getByTestId("settings-applet-delete-cancel").click();
    await expect(row).toBeVisible();
    expect(
      (await page.request.get("/__applet/e2e-imported/index.html")).status(),
    ).toBe(200);

    await page.getByTestId("settings-applet-e2e-imported-delete").click();
    await page.getByTestId("settings-applet-delete-confirm").click();
    await expect(row).toHaveCount(0);
    expect(
      (await page.request.get("/__applet/e2e-imported/index.html")).status(),
    ).toBe(404);
  });

  test("an import never replaces an applet, and a hostile file is refused", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    const input = page.getByTestId("settings-applets-import-input");

    await input.setInputFiles(
      testdataPath("applet-archives", "e2e-conflict.hiveapplet"),
    );
    await expect(page.getByTestId("applet-import-conflict")).toContainText(
      "will not be replaced",
    );
    await expect(page.getByTestId("applet-import-id")).toHaveValue(
      "e2e-page-2",
    );
    await page.getByTestId("applet-import-trust").check();
    await page.getByTestId("applet-import-id").fill("e2e-page");
    await page.getByTestId("applet-import-confirm").click();
    await expect(page.getByTestId("applet-import-error")).toContainText(
      "already exists",
    );
    await page.getByTestId("applet-import-cancel").click();
    await expect(page.getByTestId("settings-applet-e2e-page")).toContainText(
      "E2E page",
    );

    await input.setInputFiles(
      testdataPath("applet-archives", "e2e-traversal.hiveapplet"),
    );
    await expect(
      page.getByTestId("settings-applets-share-error"),
    ).toContainText("outside the applet");
    await expect(page.getByTestId("applet-import")).toHaveCount(0);
  });

  test("an export lists what goes in and what stays, and a built-in has none", async ({
    page,
  }) => {
    await page.goto("/#/settings/applets");
    await expect(
      page.getByTestId("settings-applet-applet-builder-export"),
    ).toHaveCount(0);
    await page.getByTestId("settings-applet-e2e-bar-export").click();
    const files = page.getByTestId("applet-export-files");
    await expect(files).toContainText("applet.yaml");
    await expect(files).toContainText("bar.html");
    await expect(files).not.toContainText(".localstorage.json");
    await page.getByTestId("applet-export-cancel").click();
    await expect(page.getByTestId("applet-export")).toHaveCount(0);
  });

  test("the asset route hides saved storage and gives a page no CORS", async ({
    page,
  }) => {
    const hidden = await page.request.get(
      "/__applet/e2e-page/.localstorage.json",
    );
    expect(hidden.status()).toBe(404);
    const outside = await page.request.get(
      "/__applet/e2e-page/..%2fe2e-other%2fapplet.yaml",
    );
    expect(outside.status()).toBe(404);

    const pageFile = await page.request.get("/__applet/e2e-page/index.html");
    expect(pageFile.headers()["access-control-allow-origin"]).toBeUndefined();
    const moduleFile = await page.request.get("/__applet/e2e-page/helper.mjs");
    expect(moduleFile.headers()["access-control-allow-origin"]).toBe("null");
  });
});
