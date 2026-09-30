---
icon: lucide/pin
description: Pin a few feeds to the menu bar and act on their items without opening the main window.
---

# Menu bar

The Hive icon in the menu bar lists the feeds you pin, so you can see what is waiting and act on it without opening the main window.

## Pin feeds

Open **Settings ▸ Menu bar** and choose up to three feeds. Each pinned feed lists its newest items, three by default. You can set the limit per feed, from 1 to 10. Reorder the list with the arrows. The menu shows feeds in the same order.

Each feed's heading shows where it lives, as **Profile › Folder › Feed**. A feed outside a folder shows **Profile › Feed**. Click the heading to open the whole feed. When a feed has more items than its limit, the menu adds a **more** entry that also opens it.

## The dropdown

- Each item row shows the item's title. A dot marks an unread item.
- **Refresh** polls every source now.
- **Profiles** lists your profiles. Click one to open it in Hive.
- The footer shows when Hive last polled your sources.

## Item actions

Each item has a submenu:

- **Open in Browser** and **Copy Link**, when the item has a URL.
- **Open in Hive** selects the item in the main window.
- Your configured [actions](actions.md) that apply to the item.

The menu runs only actions that need nothing more from you. Clipboard actions copy their text straight away. An action that asks for input, or one that opens the New Session dialog, is available only from the item in the main window. If an action already ran for the item and needs confirming, or if it fails, Hive opens the item so you can take it from there.

## settings.yaml

Pins are stored in `settings.yaml`, so they sync with the rest of your settings:

```yaml
menu_bar:
  feeds:
    - feed: work/reviews # <profile id>/<feed node id>
      limit: 5
    - feed: work/mentions # three items, the default
```

A pin that names a feed that no longer exists is skipped.
