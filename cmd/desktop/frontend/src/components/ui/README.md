# UI building blocks

This folder holds the app's reusable building blocks: buttons, overlays,
form controls, badges, banners. A building block knows nothing about feeds,
flows, sessions, or settings. Feature components compose them; they do not
copy them. Before you write a `<button>`, a dialog, an error line, or an
empty state, find the block below and use it.

## Where does a component go?

- **`components/ui/`**: a generic building block with no feature knowledge.
  It takes props and slots, renders, and emits. It imports no store, no
  binding, and no feature module. Two unrelated views could use it.
- **`components/`**: a feature component. It belongs to one surface of the
  app (`FeedList`, `TerminalMode`, `CreateSessionDialog`) and may read
  stores and call bindings.
- **A feature folder** (`components/settings/`, `pipeline/`): components that
  only make sense inside that feature. `settings/` holds the Settings page
  structure; `pipeline/fields/` holds the flow editor's form kit.

If a feature component turns out to be generic, move it here and add it to
the table.

## Index

| Block                | Use it for                                                                                                                                                                                       | Key props                                                                                                                                                                                                                    |
| -------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `BaseButton`         | Every button with a label.                                                                                                                                                                       | `variant` (`primary`, `secondary`, `danger`, `danger-outline`, `ghost`), `size` (`xs`, `sm`, `md`), `busy` (disables it and puts a `Spinner` in place of the icon), `disabled`, `type`; `#icon` slot                         |
| `BaseModal`          | A centered dialog.                                                                                                                                                                               | `title`, `icon`, `tone`, `width`, `ariaRole`, `busy`, `closeOnBackdrop`, `closeOnEscape`, `testid`; emits `close`; `#footer`, `#header-actions`                                                                              |
| `DrawerSheet`        | A side sheet (editors, integration setup). Resizable unless `width` is set.                                                                                                                      | `ariaLabel`, `testid`, `width`, `storageKey`, `closeOnEscape`, `closeOnBackdrop`, `trapFocus`, `returnFocusTo`; emits `close`; `#header`, `#footer`                                                                          |
| `HubOverlay`         | The large panel a hub view (Tasks, Activity) opens in. It does not register in the open-modal count; the view inside handles Escape.                                                             | `label`, `testid`; emits `close`; default slot                                                                                                                                                                               |
| `ConfirmationDialog` | A modal "are you sure" with optional detail rows.                                                                                                                                                | `title`, `description`, `details`, `confirmLabel`, `busy`, `error`, `testid`; emits `confirm`, `cancel`                                                                                                                      |
| `ConfirmationHost`   | The `ConfirmationDialog` for a `useConfirmation()` instance. Request a confirm with `request({ title, description, onConfirm, testid })`; a throw from `onConfirm` keeps it open with the error. | `confirmation`, `testid` (fallback when the request has none)                                                                                                                                                                |
| `InlineConfirm`      | A confirm that replaces a row or card in place, no overlay.                                                                                                                                      | `title`, `description`, `confirmLabel`, `cancelLabel`, `busy`, `error`, `testid`; emits `confirm`, `cancel`                                                                                                                  |
| `RenameDialog`       | A dialog that edits one name: focuses and selects it, trims it, Enter submits.                                                                                                                   | `title`, `label`, `name`, `hint`, `icon`, `confirmLabel`, `busy`, `error`, `locked` (form inert, no footer, stays open), `testid`, `testids`; emits `close`, `save(name)`; default slot under the field, `#footer-start`     |
| `InlineError`        | Any error message shown in a view, form, or sheet; renders `role="alert"`.                                                                                                                       | `message` (accepts `null`), `variant` (`banner`, `line`), `testid`; default slot                                                                                                                                             |
| `EmptyState`         | "Nothing here yet" text in a list or pane.                                                                                                                                                       | `message`, `variant` (`plain` centered, `boxed` as a card, `inline` as a left-aligned note the caller pads); default slot                                                                                                    |
| `ViewHeader`         | The title strip of a full-frame view (Settings, Activity, Dev).                                                                                                                                  | `#title` slot                                                                                                                                                                                                                |
| `FormField`          | A label above a control, with a hint or an error below.                                                                                                                                          | `label`, `hint`, `error` (accepts `null`), `testid`; `#default="{ id }"` (bind `:id="id"` on the control), `#label`                                                                                                          |
| `TextInput`          | A one-line text field. Other attributes (`id`, `placeholder`, `data-testid`, `@keydown`) go to the `<input>`.                                                                                    | `v-model` (always a string), `type`, `monospace`, `size` (`sm`, `md`), `invalid`; exposes `focus()`, `select()`                                                                                                              |
| `TextArea`           | A multi-line text field, styled like `TextInput`.                                                                                                                                                | `v-model`, `rows`, `monospace`, `size`, `invalid`; exposes `focus()`, `select()`                                                                                                                                             |
| `SearchField`        | A filter or search box with a search icon. Escape with text clears it; Escape when empty passes through.                                                                                         | `v-model`, `variant` (`boxed`, `bar`), `placeholder`, `ariaLabel`, `testid`; emits `escape` (on an empty field); `@keydown` and other attributes go to the `<input>`, `class` to the field; exposes `focus()`, `select()`    |
| `AppSelect`          | A dropdown or combobox.                                                                                                                                                                          | `modelValue`, `options`, `placeholder`, `searchable`, `editable`, `disabled`, `size`, `testid`, `ariaLabel`, `id`                                                                                                            |
| `AppCheckbox`        | A labelled checkbox.                                                                                                                                                                             | `modelValue`, `label`, `hint`, `disabled`, `testid`                                                                                                                                                                          |
| `AppSwitch`          | An on/off toggle that applies at once.                                                                                                                                                           | `modelValue`, `label`, `hint`, `ariaLabel`, `disabled`, `size`, `testid`                                                                                                                                                     |
| `SegmentedControl`   | A choice from a few named options, shown as a strip. `compact` is the bare strip of a toolbar, the title bar, or a dialog header.                                                                | `v-model`, `options` (`value`, `label`, `title`), `variant` (`field`, `compact`), `size` (`sm`, `md`; compact only), `label`, `hint`, `columns`, `ariaLabel`, `testid`; `#option="{ option, selected }"` for icons or counts |
| `AppMenu`            | A dropdown menu of actions.                                                                                                                                                                      | `entries`, `anchor`, `flip`, `width`, `ignore`, `testid`; emits `select`, `close`                                                                                                                                            |
| `AppTooltip`         | A hover or focus hint on any trigger.                                                                                                                                                            | `text` (empty renders no tooltip), `delay`; default slot is the trigger                                                                                                                                                      |
| `BaseBadge`          | A status pill or chip.                                                                                                                                                                           | `tone`, `variant` (`pill`, `chip`), `dot`                                                                                                                                                                                    |
| `CopyButton`         | A secondary button that copies a value and shows "Copied" for a moment.                                                                                                                          | `text` (empty does nothing), `label` (default `Copy`)                                                                                                                                                                        |
| `Spinner`            | A decorative wait indicator beside text that says what is happening. Inside a button, use `BaseButton`'s `busy`.                                                                                 | none                                                                                                                                                                                                                         |
| `Kbd`                | A key or shortcut hint (`⌘K`, `esc`, `↵`).                                                                                                                                                       | `variant` (`plain` mono text in the surrounding color, the default; `boxed` key cap; `on-accent` inside a primary button)                                                                                                    |
| `BaseCard`           | A bordered card, optionally clickable.                                                                                                                                                           | `as` (`article`, `button`), `interactive`, `padded`; `#icon`, `#actions`                                                                                                                                                     |
| `BaseIconBadge`      | A square tile behind an icon.                                                                                                                                                                    | `size` (px), `rounded`                                                                                                                                                                                                       |
| `PanelResizeHandle`  | The drag edge of a resizable panel, wired to `useResizablePanel`.                                                                                                                                | `edge`, `name`, `start`, `step`                                                                                                                                                                                              |
| `SparkLine`          | A small trend line under a number. Set its height with a class.                                                                                                                                  | `values`, `capacity`                                                                                                                                                                                                         |

### Also available outside this folder

The flow editor's form kit, `pipeline/fields/` (import from
`pipeline/fields`). Each is a control inside a `FormField` and takes
`label`, `hint`, `error`, `testid` and a `v-model`:

| Field            | Use it for                                            |
| ---------------- | ----------------------------------------------------- |
| `TextField`      | A one-line text input; `monospace` for ids and globs. |
| `TextareaField`  | A multi-line input; `rows`, `monospace`.              |
| `NumberField`    | A number input; `min`, `max`, `step`.                 |
| `SelectField`    | `AppSelect` inside a `FormField`.                     |
| `ToggleField`    | `AppCheckbox` with field props.                       |
| `GlobListField`  | One glob per line, bound to `string[]`.               |
| `IntervalField`  | A source node's poll interval.                        |
| `CodeField`      | A syntax-colored code editor.                         |
| `MarkImageField` | A source's feed-mark image picker.                    |

The Settings page structure, `components/settings/`:

| Component         | Use it for                                                     |
| ----------------- | -------------------------------------------------------------- |
| `SettingsLayout`  | The Settings frame: nav plus content, closes on Escape.        |
| `SettingsNavItem` | One entry in the Settings nav.                                 |
| `SettingsPage`    | The content column of one settings page.                       |
| `SettingsSection` | A titled group of settings; `boxed` stacks rows into one card. |
| `SettingsHeading` | A section or group label.                                      |
| `SettingsRow`     | One setting: label on the left, control on the right.          |
| `SettingsPathRow` | An on-disk path with copy, open, and reveal actions.           |
| `SettingsStepper` | A number setting stepped between `min` and `max`.              |

## Rules

### Overlays are `BaseModal` or `DrawerSheet`

Never hand-roll a `role="dialog"`. Both blocks do three things a copy forgets:

- **Escape closes only the top overlay.** `useEscapeToClose` keeps one stack
  of enabled callers and fires only the topmost, so a confirm inside a drawer
  inside Settings closes one layer per keypress. A caller moves to the top
  when it registers and again each time its `enabled` turns true.
- **Global keys stop while an overlay is open.** Both register with
  `useRegisterOpenModal`; `useOpenModalCount()` is the shared count, and
  `App.vue` and `TasksView` skip their keybindings while it is above zero.
- **Focus.** They trap focus inside and return it to the trigger on close.

`HubOverlay` is the one exception: it traps and returns focus but does not
register, because App.vue and TasksView read a non-zero count as a dialog
stacked over the hub and would never let it close.

A popover (`AppMenu`, `AppSelect`, `AppTooltip`) is not a modal and does not
register.

### Buttons are `BaseButton`

A labelled button is a `BaseButton`. An icon-only button may be a plain
`<button>` until an icon-button block exists; give it an `aria-label`.

`busy` is for the button whose action is running: it draws the spinner. A
Cancel beside it that must not fire meanwhile takes `disabled`, not `busy`.
`xs` is the compact button of a toolbar or a settings row.

### Form fields are `FormField` plus `TextInput`

A labelled control is a `FormField` around the control, and a text field
is a `TextInput` or a `TextArea`. Do not write a `<div class="mb-1.5 ...">`
label, a `<label>` that wraps its input, or a `rounded-lg border ...` class
list on a raw `<input>`. Take the `id` from the default slot and bind it on
the control, so a click on the label focuses it:

```vue
<FormField v-slot="{ id }" label="Name" :error="nameError" testid="thing-name">
  <TextInput :id="id" v-model="name" data-testid="thing-name-input" />
</FormField>
```

Use `size="sm"` in drawers and in a row beside a small button. Layout
classes (`flex-1`, `mb-3`) go on the component; for a fixed width, wrap it
in a `<div>`, because `TextInput` is `w-full`.

Use `#label` when the label needs markup, such as an "(optional)" suffix.
A field that has no single control (a group of switches, a list) can still
use `FormField` for its label and hint.

### Search boxes are `SearchField`

Use `boxed` in a view toolbar and `bar` inside a flush `h-9 border-b` row
that the host draws. Escape behaves the same in every search box: with
text, it clears the text and stops there, so the view or overlay stays
open; on an empty field it passes through to the Escape stack, and the
field emits `escape` for a host that moves focus out. The search inside a
popover (`AppSelect`, `RepositorySelect`) stays part of that popover,
because there Escape closes the popover.

### Errors are `InlineError`, empty states are `EmptyState`

Do not style a red `<p>` or a grey "No items" line by hand. Pass a nullable
error ref straight to `InlineError`'s `message` and gate it with `v-if`.

`InlineError` has two forms. The default `banner` boxes the message; use it
for a failure that belongs to a whole view, form, or sheet. `line` is bare
text; use it under a field, inside a list row, or beside a button. Both use
`text-severity-error` and nothing else: never `text-kind-issue` for an error.
Pass spacing (`mt-2`, `px-3`) and `leading-relaxed` as a class.

`EmptyState` sets the color and the size of the message, so do not pass
those as a class. Use `plain` where the list or pane itself is empty and
`inline` for a note in a card or a sidebar section, padded like its rows.
An empty pane with a heading and an action (the feed's "You're all caught
up", the chat pane's "No chat open") is a panel of its own, not an
`EmptyState`.

### Key hints are `Kbd`

A shortcut shown to the user is a `Kbd`, never a styled `<kbd>` or
`<span>`. The default `plain` sets only the mono font; pass the color the line needs
as a class. The key caps in Settings > Keyboard are the binding editor's
own capture surface, not hints, and keep their local style.

### Test ids

`data-testid` is the API for both the e2e suite and agents driving the app.
Naming follows `<surface>-<object>[-<verb>]` in kebab case; see
`.agents/skills/desktop-ui-audit/SKILL.md` ("Authoring views an agent can
drive") for the full convention.

- A **compound** block takes a `testid` prop and derives its parts from it:
  `BaseModal` renders `${testid}`, `${testid}-backdrop`, `${testid}-close`;
  `InlineConfirm` adds `-confirm`, `-cancel`, `-error`. One prop names the
  whole widget.
- A **leaf** block (`BaseButton`, `BaseBadge`, `BaseCard`) takes no `testid`
  prop. Put `data-testid` on it and it passes through to the root element.
- Never rename an existing id without grepping `cmd/desktop/e2e/tests` and
  the unit specs.

### Icons

Icons come from `~icons/lucide/*` (`import IconX from '~icons/lucide/x'`).
Do not paste SVG markup for an icon Lucide has.

### Styling

Tailwind utilities only. Add a scoped `<style>` block only for a selector
Tailwind cannot express (a pseudo-element, a keyframe, a child of
`v-html`), as `AppTooltip` and `PanelResizeHandle` do.

## Tokens

Color tokens live in `src/styles/main.css` under `@theme` (`bg-raised`,
`text-text-2`, `border-card`, `text-severity-error`) and are the only colors
to use. Typography and radius tokens do not exist yet, so the code still
carries arbitrary sizes such as `text-[12.5px]`. #537 adds them; until it
does, copy the size the nearest existing block uses rather than picking a
new one.

## Adding a building block

1. Confirm nothing in the index already does the job. Extend a block with a
   prop before you add a near-copy.
2. Put it in `components/ui/`. It must not import a store, a binding, or a
   feature module.
3. Follow the test-id rule: compound blocks take `testid`, leaves pass
   `data-testid` through.
4. Add a spec in `components/ui/__tests__/` that covers behavior: emits,
   `v-model`, disabled and busy states, derived test ids. Do not assert on
   classes.
5. Add a row to the index above.
