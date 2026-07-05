# pf — Review & Refactor Plan (V3 branch)

> **وضعیت (2026-07-05): همه فازها اجرا شد. ✅**
> - Phase A ✅ `a20b947` — تقسیم ui.go به ۱۰ فایل
> - Phase C2 ✅ `a274b59` — انتقال CLI به `internal/cli`؛ فقط `main.go` در `cmd/pf`
> - Phase B ✅ `838d03e` — سه state-struct، تزریق storage با `NewUI(ctx, mgr, store)`، rename ها، `normalizeKeyToken`
> - Phase C1 ✅ `5a90761` — استارت همگام سرویس‌ها قبل از TUI (حذف println وسط TUI)
> - Phase D ✅ `a6ecfaf` — wrapText بر اساس display-width + رفع false-positive الگوی `format`
> - اضافه ✅ `45a86b5` — رفع race واقعی در `TestEnsurePortFreeReleasesHeldPort` (پروبِ والد پورت را می‌دزدید؛ حالا با خط READY همگام می‌شود)
> - P3 اضافه: `st := storage.NewStorage()` در `manage.go` (هندلر حذف) هم `u.store` شد — جمعاً ۸ محل.
> - باقی‌مانده اختیاری: امضای `RestartService` که همیشه nil برمی‌گرداند (Phase D-3).

> این سند self-contained است: هر مدل/سشن دیگری باید بتواند فقط با خواندن همین فایل، ریفکتور را دقیق اجرا کند.
> **قانون طلایی: هیچ تغییر رفتاری (behavior change) مجاز نیست مگر در Phase D که صراحتاً «fix» است. بعد از هر فاز: `go build ./... && go vet ./... && go test ./...` باید سبز باشد، سپس یک commit جدا.**

---

## 0) وضعیت فعلی (Baseline — تأیید شده در 2026-07-05)

- Branch: `V3` — ‏`go build`, `go vet`, `go test ./...` همگی **سبز** هستند.
- Go 1.25، پکیج‌ها: `charm.land/bubbletea/v2 v2.0.7`, `charm.land/lipgloss/v2`, `charm.land/bubbles/v2`, `cobra`, `go-pkcs12`.
  (توجه: import ها `charm.land/...` هستند نه `github.com/charmbracelet/...` — این همان نسخه v2 رسمی charm است، **تغییرش نده**.)
- ساختار:
  - `cmd/pf/` — ‏CLI (cobra): main, commands, run, service, group, cert, kubectl, cleanup, system, theme, icon, help, style, completion*
  - `internal/manager/` — اجرای پروسه‌ها، restart/backoff، kill process-tree (573 خط، کیفیت خوب)
  - `internal/storage/` — ‏`~/.pf/services.json`، atomic write، migration (635 خط، کیفیت خوب)
  - `internal/ui/ui.go` — **2447 خط، god-file، هدف اصلی ریفکتور**
  - `internal/cert`, `internal/configedit`, `internal/icons`, `internal/theme`, `internal/stringutil`, `internal/updater`, `internal/version`, `internal/model`

### نتیجه بررسی (Review findings)

**نقاط قوت (دست نزن):**
- storage: atomic write با temp file + rename-retry، legacy migration، تفکیک خوب.
- manager: mutex-safe snapshot، exponential backoff + jitter، bulk-kill بهینه در StopAllServices، تزریق cert به دستورات kubectl.
- تست‌های واحد خوب برای storage/manager/configedit/ui-helpers.

**مشکلات (به ترتیب اولویت):**
| # | مشکل | محل | فاز |
|---|------|-----|-----|
| P1 | `ui.go` تک‌فایل 2447 خطی: state سه صفحه (dashboard، manage overlay، دو فرم) + همه رندرها در یک فایل | `internal/ui/ui.go` | A |
| P2 | فیلدهای UI struct با پیشوندهای گنگ (`addForm*` برای فرم سرویس) و ~25 فیلد تخت | `internal/ui/ui.go:127-175` | B |
| P3 | `storage.NewStorage()` به صورت پنهان ~7 بار داخل UI ساخته می‌شود (hidden dependency، هر بار migration-check و disk I/O) | `buildManageRows`, `launchEditor`, `openEditServiceFormFor`, `openNewGroupForm`, `openEditGroupFormFor`, `submitServiceForm`, `submitGroupForm` | B |
| P4 | `ui.NewUI(mgr, ctx)` — طبق کانونشن Go باید `ctx` پارامتر اول باشد | `internal/ui/ui.go:179`, `cmd/pf/run.go` | B |
| P5 | بلوک نرمال‌سازی کلید ۴ بار copy/paste شده (`keyRaw != "space" → NormalizeToken`) | `Update`, `updateManageMode`, `updateAddForm`, `updateGroupForm` | B |
| P6 | `cmd/pf/main.go`: دو بار `storage.NewStorage()` ساخته می‌شود (خط 14 و 19) | `cmd/pf/main.go` | C |
| P7 | `cmd/pf/run.go`: سرویس‌ها داخل goroutine استارت می‌شوند و `fmt.Printf` در حین اجرای TUI صفحه را خراب می‌کند؛ goroutine اصلاً لازم نیست چون `StartService` خودش non-blocking است | `runStartCommand` | C |
| P8 | `wrapText` بر اساس `len()` بایتی می‌شکند نه display-width (متن غیر ASCII غلط wrap می‌شود) | `internal/ui/ui.go:1715` | D |
| P9 | `ensureValidCommand` الگوی `format` را بلاک می‌کند → false positive برای هر دستوری که کلمه format دارد | `internal/manager/manager.go:170` | D |

---

## Phase A — تقسیم مکانیکی `internal/ui/ui.go` (بدون هیچ rename/تغییر رفتار)

فقط **جابه‌جایی کد** بین فایل‌های جدید در همان پکیج `ui`. هیچ امضایی عوض نمی‌شود، پس هیچ فایل دیگری تغییر نمی‌کند و تست‌ها بدون دست‌کاری سبز می‌مانند.

نگاشت دقیق تابع→فایل (ترتیب فعلی در ui.go حفظ شود):

### `internal/ui/ui.go` (باقی‌مانده — هستهٔ مدل)
- types/msgs: `tickMsg`, `spinnerTickMsg`, `shutdownDoneMsg`, `clearStatusMsg`, `statusClearDelay`, `spinnerFrames`, `editResultMsg`
- `Controller` interface، ‏`UI` struct، ‏`uiTickInterval`, `NewUI`, `Init`, `Update`
- `shutdownCmd`, `setStatus`, `spinnerTick`, `tickCmd`
- `View`, `viewContent`

### `internal/ui/theme.go`
- متغیرهای رنگ package-level: `colorText` … `colorSelected`
- `statusHealthyColor/ConnectingColor/ErrorColor`, `init()`, `ApplyTheme`

### `internal/ui/dashboard.go` (state/logic صفحه اصلی مانیتورینگ)
- `ensureCursorInRange`, `maxVisibleServices`, `ensureCursorVisible`
- `refreshViewportContent`, `onCursorMoved`, `logScopeLabel`
- `ensureViewportSize`, `chromeBelowLog`, `calculateViewportHeight`

### `internal/ui/dashboard_view.go` (رندر صفحه اصلی)
- `renderEmptyState`, `renderServiceTable`, `formatUptime`, `renderLogsContent`
- `helpLines`, `renderHelp`, `balancedHelpLines`, `greedyHelpLines`
- `renderShutdownScreen`

### `internal/ui/manage.go` (state/logic اورلی مدیریت گروه+سرویس)
- `manageRowKind` consts, `manageRow`, `selectable`, `overlayIcons`
- `enterManageMode`, `exitManageMode`, `buildManageRows`, `rebuildManageRows`
- `clampManageCursor`, `moveManageCursor`, `focusFirstSelectable`, `focusFirstService`, `focusManage`
- `currentManageRow`, `runningNameSet`, `updateManageInput`, `updateManageMode`, `runManageSelection`
- `manageVisibleRows`, `ensureManageVisible`

### `internal/ui/manage_view.go` (رندر اورلی)
- `renderManageOverlay`, `renderManageSearchLine`
- `renderManageGroupRow`, `renderManageServiceRow`, `renderSelectCheckbox`, `overlayIconCell`
- `summarizeMembers`

### `internal/ui/service_form.go` (فرم افزودن/ویرایش سرویس — state+update+render)
- `formInputWidth`, `newServiceTextInput`
- `openNewServiceForm`, `openEditServiceFormFor`, `closeAddForm`, `toggleAddFormFocus`
- `updateAddForm`, `updateAddFormInput`, `submitServiceForm`, `renderServiceForm`

### `internal/ui/group_form.go` (فرم گروه)
- `openNewGroupForm`, `openEditGroupFormFor`, `closeGroupForm`, `toggleGroupFormFocus`
- `updateGroupForm`, `updateGroupNameInput`, `submitGroupForm`, `renderGroupForm`

### `internal/ui/editor.go` (ویرایش کانفیگ در $EDITOR از داخل TUI)
- `launchEditor`

### `internal/ui/textutil.go` (کمکی‌های متن/رندر خالص)
- `wrapText`, `truncateRunes`, `padRightRunes`, `padRightDisplayWidth`
- `serviceIcon`, `renderIconCell`, `renderActionChips`

**چک Phase A:** `go build ./... && go vet ./... && go test ./...` → commit با پیام `refactor(ui): split ui.go into focused files (no behavior change)`

---

## Phase B — نام‌گذاری دقیق + گروه‌بندی state + تزریق وابستگی

### B1. گروه‌بندی فیلدهای UI struct در سه sub-struct (در فایل‌های مربوطه تعریف شوند):

```go
// service_form.go
type serviceFormState struct {
    mode         string // "" | "new" | "edit"
    originalName string // نام قبلی در حالت edit (قبلاً addFormOrig)
    nameInput    textinput.Model
    commandInput textinput.Model
    focusedField int    // 0 = name, 1 = command
    errorMsg     string
}

// group_form.go
type groupFormState struct {
    mode          string // "" | "new" | "edit"
    originalName  string
    nameInput     textinput.Model
    errorMsg      string
    focusedField  int // 0 = name, 1 = services list
    serviceNames  []string
    selected      map[string]bool
    serviceCursor int
}

// manage.go
type manageState struct {
    active            bool
    rows              []manageRow
    cursor, offset    int
    groups            map[string][]string
    groupNames        []string
    serviceNames      []string
    icons             overlayIcons
    selectedGroups    map[string]bool
    selectedServices  map[string]bool
    confirmDeleteName string
    confirmDeleteKind string // "group" | "service"
    errorMsg, infoMsg string
    searchQuery       string
    showNewPrompt     bool
}
```

در `UI`: فیلدهای تخت حذف و به `serviceForm serviceFormState`, `groupForm groupFormState`, `manage manageState` تبدیل شوند. نگاشت نام قدیم→جدید:

| قدیم | جدید |
|------|------|
| `addFormMode/Orig/Name/Cmd/Focus/Err` | `serviceForm.mode/originalName/nameInput/commandInput/focusedField/errorMsg` |
| `groupFormMode/Orig/Name/Err/Focus/Services/Selected/SvcCursor` | `groupForm.mode/originalName/nameInput/errorMsg/focusedField/serviceNames/selected/serviceCursor` |
| `manageMode` | `manage.active` |
| `manageRows/Cursor/Offset/Groups/GroupNames/Services/Icons` | `manage.rows/cursor/offset/groups/groupNames/serviceNames/icons` |
| `manageSelGroups/SelSvcs` | `manage.selectedGroups/selectedServices` |
| `manageConfirmDelete/ConfirmKind` | `manage.confirmDeleteName/confirmDeleteKind` |
| `manageErr/Info/Search/NewPrompt` | `manage.errorMsg/infoMsg/searchQuery/showNewPrompt` |

### B2. rename متدها (فقط این‌ها؛ بقیه نام‌ها خوبند):

| قدیم | جدید | دلیل |
|------|------|------|
| `updateAddForm` | `updateServiceForm` | فرم «سرویس» است نه «add» (edit هم دارد) |
| `updateAddFormInput` | `forwardServiceFormInput` | ورودی را به textinput فوکوس‌شده می‌فرستد |
| `closeAddForm` | `closeServiceForm` | همسان با بقیه |
| `toggleAddFormFocus` | `toggleServiceFormFocus` | — |
| `updateGroupNameInput` | `forwardGroupFormInput` | — |
| `updateManageInput` | `forwardOverlayInput` | مبهم بود |
| `buildManageRows` | `reloadManageRowsFromStorage` | تمایز با rebuild (که فقط فیلتر است) |
| `rebuildManageRows` | `applyManageFilter` | دقیقاً همین کار را می‌کند |

### B3. تزریق storage به UI (حذف P3):
- امضای جدید: `func NewUI(ctx context.Context, mgr Controller, store *storage.Storage) *UI` — فیلد `store *storage.Storage` به UI اضافه شود.
- همه `storage.NewStorage()` های داخل `internal/ui` با `u.store` جایگزین شوند (۷ محل ذکرشده در P3؛ `launchEditor` هم `st := u.store`).
- در `cmd/pf/run.go`: ‏`ui.NewUI(ctx, mgr, st)` (همان `st` که ساخته شده پاس داده شود) — این P4 را هم حل می‌کند.

### B4. حذف تکرار نرمال‌سازی کلید (P5) — در `ui.go`:
```go
// normalizeKeyToken maps a key message to the canonical token used in the
// keymap switches. "space" is kept verbatim so it stays distinguishable from
// a literal space rune inserted into search/text inputs.
func normalizeKeyToken(msg tea.KeyMsg) string {
    raw := msg.String()
    if raw == "space" {
        return raw
    }
    return stringutil.NormalizeToken(raw)
}
```
و چهار بلوک تکراری با `key := normalizeKeyToken(msg)` جایگزین شود (توجه: در `updateManageMode` متغیر `keyRaw` برای live-search لازم است — نگهش دار: `keyRaw := msg.String()`).

**چک Phase B:** build/vet/test (تست‌های `ui_test.go` اگر به نام‌های قدیمی ارجاع دارند، همان‌جا آپدیت شوند) → commit `refactor(ui): group screen state into structs, inject storage, precise names`

---

## Phase C — پاکسازی `cmd/pf`

1. **main.go (P6):** یک بار `st := storage.NewStorage()` بساز؛ `st.EnsureExists()` و `st.RegisterCustomThemes()` و `st.ThemeName()` روی همان.
2. **run.go (P7):** حذف goroutine های استارت:
```go
for _, name := range serviceNames {
    if err := mgr.StartService(ctx, name); err != nil {
        fmt.Printf("Error starting '%s': %v\n", name, err)
        os.Exit(1) // قبل از اجرای TUI؛ خطای واقعی start async است و در UI با status=ERROR دیده می‌شود
    }
}
_, runErr := program.Run()
```
(`StartService` non-blocking است؛ خطاهای runtime از طریق status در UI نمایش داده می‌شوند — println وسط TUI حذف می‌شود.)
3. `runStartCommand` همین حالا قبل از استارت، وجود همه سرویس‌ها و تداخل پورت را چک می‌کند — دست نزن.

**چک Phase C:** build/vet/test + تست دستی: `go run ./cmd/pf run <name>` → commit `refactor(cmd): single storage instance, remove racy start goroutines`

### C2 — انتقال کل CLI از `cmd/pf` به `internal/cli` (درخواست کاربر)

هدف: در `cmd/pf` فقط **یک فایل اجرایی** (`main.go`) بماند؛ همه‌ی منطق CLI به پکیج `internal/cli` برود (لایه‌بندی استاندارد Go: `cmd/` فقط entrypoint نازک).

1. `git mv` این فایل‌ها از `cmd/pf/` به `internal/cli/` و در همه `package main` → `package cli`:
   `commands.go`(→`root.go`), `run.go`, `service.go`, `group.go`, `cert.go`, `kubectl.go`, `cleanup.go`, `system.go`, `help.go`, `style.go`, `icon.go`, `theme.go`, `completion.go`, `completion_cmd.go`, `main_test.go`(→`run_test.go`), `completion_test.go`
2. منطق فعلی `main()` (پاکسازی آپدیتر، ساخت storage واحد، ثبت تم‌ها، اعمال تم، اجرای root) به تابع صادرشده‌ی `cli.Execute() error` در `internal/cli/root.go` منتقل شود.
3. `cmd/pf/main.go` جدید فقط:
```go
package main

import (
	"os"

	"github.com/alinemone/go-port-forward/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
```
4. هیچ شناسه‌ی دیگری از `cli` صادر (exported) نشود؛ فقط `Execute`.
5. توجه: `applyCLITheme` در `style.go` با `init()` صدا می‌شود و بعد از `theme.Set` هم در Execute دوباره صدا می‌شود — همین رفتار حفظ شود.

**چک C2:** build/vet/test (تست‌ها با پکیج `cli` سبز باشند) → commit `refactor(cli): move command layer to internal/cli, keep cmd/pf as thin entrypoint`

---

## Phase D — اصلاحات کوچک رفتاری (هر کدام commit جدا)

1. **P8 — wrapText با display-width:** به‌جای `len(...)` از `lipgloss.Width(...)` برای اندازه‌گیری و از `[]rune` برای برش استفاده شود. تست‌های موجود `wrapText` باید پاس بمانند؛ یک تست یونیکد اضافه کن.
2. **P9 — ValidateCommand:** الگوی `format` به `\bformat\b\s+[a-z]:` (فرمت دیسک ویندوز) محدود شود یا کلاً حذف؛ تست اضافه کن که `kubectl ... --output=custom-format` رد نشود.
3. (اختیاری) `RestartService` که همیشه `nil` برمی‌گرداند → امضا به `RestartService(ctx, name)` بدون خروجی error تغییر کند و caller ها آپدیت شوند.

---

## ترتیب اجرا و Definition of Done

1. Phase A → commit ‏(بزرگ‌ترین بازده، صفر ریسک)
2. Phase B → commit
3. Phase C → commit
4. Phase D → هر فیکس یک commit
5. پایان: `go build ./... && go vet ./... && go test ./...` سبز + smoke test دستی TUI (run یک سرویس، باز کردن overlay با `a`، ساخت/ویرایش/حذف سرویس و گروه، resize ترمینال، quit با `q` و آزاد شدن پورت‌ها).

**DoD:** هیچ فایل go بالای ~600 خط در `internal/ui`؛ هیچ `storage.NewStorage()` داخل `internal/ui`؛ تست‌ها سبز؛ رفتار TUI عیناً قبلی (به‌جز فیکس‌های Phase D).

## نکات اجرا برای مدل بعدی
- روی branch `V3` کار کن؛ `scripts/` untracked است و ربطی به ریفکتور ندارد.
- import های `charm.land/*` را دست نزن؛ اینها API نسخه v2 هستند (`tea.KeyPressMsg`, `viewport.New(viewport.WithWidth(...))`, `tea.NewView`, `lipgloss.Println`).
- در Phase A فقط cut/paste کن؛ اگر کامپایلر undefined گفت یعنی تابعی را جا انداخته‌ای — به نگاشت بالا برگرد.
- بعد از هر فاز `gofmt` (ویرایشگر Go خودش انجام می‌دهد) و commit با پیام ذکرشده.
