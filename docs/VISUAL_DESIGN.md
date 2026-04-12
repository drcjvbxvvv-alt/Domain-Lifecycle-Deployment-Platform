# VISUAL_DESIGN.md — 視覺設計規範

> **Claude Code 必讀。** 每次新增或修改前端程式碼前讀完本文件。
>
> 本文件定義平台的視覺語言，與 FRONTEND_GUIDE.md 配合使用：
> - `FRONTEND_GUIDE.md` = 「用哪個元件、怎麼組裝」
> - `VISUAL_DESIGN.md`  = 「看起來長什麼樣、用什麼顏色和間距」
>
> **Last updated**: 2026-04-12 (ADR-0003 pivot, Operational Clarity redesign)

---

## 設計方向 — Operational Clarity

本平台是一個企業級運維工具，管理 10+ 專案、1 萬+ 域名、多台 Nginx 主機。
設計哲學參考 **Linear、Vercel Dashboard、Grafana**：

- **深色側欄 + 亮色內容區**：側欄 `#0f172a` deep navy，讓使用者眼睛自然聚焦在右側資料區。
- **資料密集但清晰**：48px 表格列高，11px 大寫欄標頭，字重層次明確。
- **語意色彩豐富**：6 個狀態語意各有專屬顏色（text / bg / border），不僅有底色，還有清晰邊框。
- **技術值用等寬字體**：UUID、checksum、release ID、agent ID 一律使用 `var(--font-mono)` 以利閱讀。
- **陰影體系分層**：sm → card → elevated → modal，體現深度層次，不是扁平也不是過度立體。

### 不適用場景

以下場景不遵循本文件的深色側欄規則：

- 登入頁（`/login`）— 全頁居中卡片，無側欄
- 403/404/500 錯誤頁 — 全頁背景

---

## 色彩系統

所有顏色值來自 `web/src/styles/tokens.ts` 並對應 `web/src/styles/global.css` 中的 CSS 變數。
**禁止在元件中直接寫 hex 值。**

### 側欄色彩

| 用途 | Token (JS) | CSS 變數 | 值 |
|------|-----------|----------|----|
| 側欄背景 | `colors.bgSidebar` | `--bg-sidebar` | `#0f172a` |
| 側欄項目懸浮 | `colors.bgSidebarHover` | `--bg-sidebar-hover` | `rgba(255,255,255,0.06)` |
| 側欄項目啟用 | `colors.bgSidebarActive` | `--bg-sidebar-active` | `rgba(79,126,248,0.18)` |
| 側欄文字 | `colors.sidebarText` | `--sidebar-text` | `#94a3b8` |
| 側欄啟用文字 | `colors.sidebarTextActive` | `--sidebar-text-active` | `#ffffff` |
| 側欄分隔線 | `colors.sidebarBorder` | `--sidebar-border` | `rgba(255,255,255,0.08)` |

### 頁面/內容區色彩

| 用途 | Token (JS) | CSS 變數 | 值 |
|------|-----------|----------|----|
| 頁面背景 | `colors.bgPage` | `--bg-page` | `#f0f4f8` |
| 卡片/面板表面 | `colors.bgSurface` | `--bg-surface` | `#ffffff` |
| 微隆表面（表頭） | `colors.bgSurfaceRaised` | `--bg-surface-raised` | `#f8fafc` |
| 輸入框背景 | `colors.bgInput` | `--bg-input` | `#f8fafc` |
| 列懸浮背景 | `colors.bgHover` | `--bg-hover` | `rgba(79,126,248,0.05)` |
| 邊框 | `colors.border` | `--border` | `#e4e9f2` |
| 次要分隔線 | `colors.borderSub` | `--border-sub` | `#f1f5fb` |

### 文字色彩

| 用途 | CSS 變數 | 值 |
|------|----------|----|
| 主要文字 | `--text-primary` | `#1a2233` |
| 次要文字 | `--text-secondary` | `#4b5a6e` |
| 輔助文字 | `--text-muted` | `#8b97a8` |

### 品牌色

| 用途 | CSS 變數 | 值 |
|------|----------|----|
| 主品牌色 | `--primary` | `#4f7ef8` |
| 懸浮 | `--primary-hover` | `#3b6ef0` |
| 按下 | `--primary-pressed` | `#2c5de0` |

### 狀態語意色彩（六種）

每個狀態語意都有三個值：`color`（文字）、`bg`（背景）、`border`（邊框）。
三種狀態機（Domain Lifecycle / Release / Agent）共用此色板。

| 語意 | 對應狀態 | color | bg | border |
|------|---------|-------|-----|--------|
| `success` | active, online, succeeded, idle | `#15803d` | `rgba(21,128,61,0.10)` | `rgba(21,128,61,0.20)` |
| `progress` | executing, busy, provisioned, pending, planning, ready | `#b45309` | `rgba(180,83,9,0.10)` | `rgba(180,83,9,0.20)` |
| `warning` | paused, draining, requested, approved | `#c2410c` | `rgba(194,65,12,0.10)` | `rgba(194,65,12,0.20)` |
| `danger` | failed, error, rolling_back, rolled_back, disabled, offline | `#b91c1c` | `rgba(185,28,28,0.10)` | `rgba(185,28,28,0.20)` |
| `neutral` | retired, cancelled, registered | `#64748b` | `rgba(100,116,139,0.10)` | `rgba(100,116,139,0.20)` |
| `upgrading` | upgrading | `#6d28d9` | `rgba(109,40,217,0.10)` | `rgba(109,40,217,0.20)` |

---

## 排版系統

### 字體堆疊

- **UI 文字**：`system-ui, -apple-system, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif`
- **技術值（等寬）**：`ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace`
  — 對應 CSS 變數 `--font-mono`，JS token `fontMono`
  — 使用 class `.mono` 或 `.mono-cell`

### 字體大小

| 用途 | 大小 | CSS 變數/class |
|------|------|---------------|
| 頁面標題（PageHeader） | 20px | — |
| 卡片標題 | 15–16px | — |
| 側欄分組標籤 | 11px | `.label-section` |
| 表格欄位標頭 | 11px | AppTable 內建 |
| 狀態標籤（StatusTag） | 12px | StatusTag 內建 |
| 表格內容 | 14px | — |
| 輔助說明文字 | 13px | `.text-sm` |
| 技術值等寬 | 13px | `.mono` |
| 最小輔助 | 12px | `.text-xs` |
| 統計數字（StatCard） | 28px | StatCard 內建 |

### 字重

| 用途 | 字重 |
|------|------|
| 頁面標題 | 700 |
| 卡片標題、按鈕 | 600 |
| 標籤、徽章 | 500 |
| 正文 | 400 |

### 行高

- 標題 / 緊湊文字：1.3
- 正文：1.5
- 寬鬆內文：1.6

---

## 間距系統

本平台使用 4px 基底網格，僅允許以下值（`--space-*`）：

| 變數 | 值 | 使用場景 |
|------|----|---------|
| `--space-1` | 4px | 最小間距、icon 與文字的 gap |
| `--space-2` | 8px | 元素間小間距（icon + label） |
| `--space-3` | 12px | 篩選器間距、行內元素 |
| `--space-4` | 16px | 卡片 padding、按鈕間距 |
| `--space-5` | 20px | 卡片 padding（較寬鬆）|
| `--space-6` | 24px | 區塊間距、`--content-padding` |
| `--space-8` | 32px | 大區塊間距 |
| `--space-10` | 40px | 頁面垂直大間距 |

**禁止直接寫 px 數值。** 使用 `var(--space-x)` 或 `spacing[x]` from tokens。

---

## 陰影系統

| 名稱 | CSS 變數 | 使用場景 |
|------|----------|---------|
| sm | `--shadow-sm` | Header 底部陰影 |
| card | `--shadow-card` | 卡片、表格容器、StatCard |
| elevated | `--shadow-elevated` | 懸浮卡片、下拉選單 |
| modal | `--shadow-modal` | Modal、Dialog |
| glow | `--shadow-glow` | Focus ring（primary color） |

---

## 佈局系統

### 整體結構

```
┌─────────────────────────────────────────────────────┐
│  Sidebar (220px / 56px collapsed)                   │
│  bg: #0f172a                                        │
│                        Main Content Area            │
│  ┌──────────────┐  ┌───────────────────────────────┐│
│  │ Logo (56px)  │  │  Header (56px)                ││
│  ├──────────────┤  │  bg: white, shadow-sm          ││
│  │ Nav Groups   │  ├───────────────────────────────┤│
│  │              │  │  Page Content                 ││
│  │              │  │  padding: 24px                ││
│  │              │  │  bg: #f0f4f8                  ││
│  ├──────────────┤  │                               ││
│  │ User (bottom)│  │                               ││
│  └──────────────┘  └───────────────────────────────┘│
└─────────────────────────────────────────────────────┘
```

### 佈局常數

| 常數 | CSS 變數 | 值 |
|------|----------|----|
| 側欄寬度 | `--sidebar-width` | 220px |
| 側欄收合寬度 | `--sidebar-collapsed-width` | 56px |
| 頂部 Header 高度 | `--header-height` | 56px |
| 頁面 Header 高度 | `--page-header-height` | 64px |
| 表格列高 | `--table-row-height` | 48px |
| 搜尋欄高度 | `--search-bar-height` | 52px |
| 內容 padding | `--content-padding` | 24px |
| 最大內容寬度 | — | 1400px |

---

## 元件視覺規格

### MainLayout — 側欄 + 頂部 Header

**側欄**（`web/src/views/layouts/MainLayout.vue`）

| 部位 | 規格 |
|------|------|
| 背景 | `#0f172a` （`var(--bg-sidebar)`）|
| 右邊框 | `1px solid rgba(255,255,255,0.08)` |
| Logo 區高度 | 56px |
| Logo 文字 | 15px / 700 / white / letter-spacing 0.5px |
| 分組標籤 | 11px / 600 / uppercase / letter-spacing 0.8px / `--sidebar-text` opacity 0.5 |
| 導覽項目高度 | 36px |
| 導覽項目圓角 | 8px |
| 導覽項目文字 | 13px / 500 / `--sidebar-text` |
| 懸浮背景 | `rgba(255,255,255,0.06)` |
| 啟用背景 | `rgba(79,126,248,0.18)` |
| 啟用文字 | white |
| 啟用左指示條 | 3px / `var(--primary)` / 圓角右側 |
| 圖示大小 | 16px |
| 底部使用者區 | 分隔線 + avatar + 使用者名 + 登出圖示 |

**頂部 Header**

| 部位 | 規格 |
|------|------|
| 高度 | 56px (`--header-height`) |
| 背景 | `var(--bg-surface)` = white |
| 底部邊框 | `1px solid var(--border)` |
| 陰影 | `var(--shadow-sm)` |
| 左側 | 當前頁面標題（15px / 600 / `--text-primary`）|
| 右側 | 通知鈴鐺（icon btn 32px）+ 使用者 avatar + 名稱 |

---

### PageHeader

**用途**：每個功能頁面的頂部標題列。

| 部位 | 規格 |
|------|------|
| 底部間距 | `padding-bottom: 16px` |
| 底部分隔線 | `1px solid var(--border)` |
| 標題字體 | 20px / 700 / `--text-primary` / letter-spacing -0.3px |
| 副標題字體 | 13px / `--text-muted` / margin-top 2px |
| 操作區 gap | 8px |
| 自身 margin-bottom | 不設定 — 由父元件控制 |

---

### StatusTag

**用途**：三種狀態機（Domain Lifecycle / Release / Agent）的所有狀態值。

設計：彩色小圓點 + 標籤文字，pill 形狀，帶邊框。

| 部位 | 規格 |
|------|------|
| 形狀 | `border-radius: 9999px`（pill）|
| Padding | `2px 10px 2px 8px` |
| 字體 | 12px / 500 |
| 背景 | 語意 bg（半透明，見色彩系統）|
| 邊框 | `1px solid` + 語意 border 色 |
| 文字色 | 語意 color |
| 圓點 | 6px / 語意 color / `border-radius: 50%` / margin-right 6px |

**狀態 → 語意對照（完整）**

| 狀態值 | 語意 | 中文標籤 |
|--------|------|---------|
| `active` | success | 運行中 |
| `online` | success | 在線 |
| `succeeded` | success | 成功 |
| `idle` | success | 空閒 |
| `executing` | progress | 執行中 |
| `busy` | progress | 忙碌 |
| `provisioned` | progress | 已佈建 |
| `pending` | progress | 待執行 |
| `planning` | progress | 規劃中 |
| `ready` | progress | 就緒 |
| `paused` | warning | 已暫停 |
| `draining` | warning | 排空中 |
| `requested` | warning | 待審核 |
| `approved` | warning | 已批准 |
| `failed` | danger | 失敗 |
| `error` | danger | 異常 |
| `rolling_back` | danger | 回滾中 |
| `rolled_back` | danger | 已回滾 |
| `disabled` | danger | 已停用 |
| `offline` | danger | 離線 |
| `retired` | neutral | 已退役 |
| `cancelled` | neutral | 已取消 |
| `registered` | neutral | 已註冊 |
| `upgrading` | upgrading | 升級中 |

---

### StatCard

**用途**：Dashboard 統計卡片，一行固定 4 欄。

| 部位 | 規格 |
|------|------|
| 背景 | `var(--bg-surface)` = white |
| 邊框 | `1px solid var(--border)` |
| 圓角 | 10px |
| 陰影 | `var(--shadow-card)` |
| 懸浮陰影 | `var(--shadow-elevated)`，0.15s 過渡 |
| 左側強調條 | 3px 寬 / 全高 / `border-radius: 10px 0 0 10px` / 顏色由 `color` prop 決定 |
| 主體 padding | 20px 24px |
| 標籤 | 12px / 500 / `--text-muted` / uppercase / letter-spacing 0.4px |
| 數值 | 28px / 700 / `--text-primary` / line-height 1.2 / margin-top 4px |
| 後綴 | 13px / `--text-secondary` |
| 趨勢正 | `#15803d` ▲ |
| 趨勢負 | `#b91c1c` ▼ |

---

### AppTable

**用途**：所有列表頁的資料表格，必須透過 AppTable 而非直接使用 NDataTable。

| 部位 | 規格 |
|------|------|
| 容器圓角 | 10px |
| 容器邊框 | `1px solid var(--border)` |
| 容器背景 | `var(--bg-surface)` |
| 容器陰影 | `var(--shadow-card)` |
| 表頭背景 | `var(--bg-surface-raised)` = `#f8fafc` |
| 表頭字體 | 11px / 600 / uppercase / letter-spacing 0.4px / `--text-muted` |
| 列高 | `var(--table-row-height)` = 48px |
| 懸浮背景 | `var(--bg-hover)` = `rgba(79,126,248,0.05)` |
| 技術值 class | `.mono-cell` → `--font-mono` 13px（用於 UUID、checksum、ID）|
| 分頁器位置 | 右下，`border-top: 1px solid var(--border)` |

---

### SearchBar

**用途**：列表頁的篩選列，位於 PageHeader 與 AppTable 之間。

| 部位 | 規格 |
|------|------|
| 高度 | `var(--search-bar-height)` = 52px |
| 背景 | `var(--bg-surface)` |
| 邊框 | `1px solid var(--border)` |
| 圓角 | 8px |
| 內部 gap | `--space-3` = 12px |
| padding | `--space-3` `--space-4` |

---

### ConfirmModal

**用途**：所有不可逆操作（刪除、執行 release、rollback）的確認對話框。

| 部位 | 規格 |
|------|------|
| 背景 | `var(--bg-surface)` |
| 圓角 | 14px |
| 陰影 | `var(--shadow-modal)` |
| 標題 | 16px / 600 / `--text-primary` |
| 內容文字 | 14px / `--text-secondary` |
| 危險類型按鈕 | `type="error"` |
| 警告類型按鈕 | `type="warning"` |

---

## 技術值的視覺處理

**規則**：以下欄位類型必須使用 `.mono` class（`var(--font-mono)` 等寬字體）：

- UUID / uuid 欄位
- Checksum / SHA256 / MD5
- Release ID（形如 `rel-xxxxxxxx`）
- Agent ID
- Artifact ID
- 任何 16 進制雜湊值

**實作**：

```vue
<!-- 在 column render 中 -->
{
  title: 'UUID',
  key: 'uuid',
  render: (row) => h('span', { class: 'mono' }, row.uuid)
}

<!-- 在詳情頁中 -->
<span class="mono">{{ artifact.checksum }}</span>
```

等寬字體讓使用者更容易對齊、比較技術值，也是運維工具的標準視覺語言。

---

## 禁止事項

在提交任何前端程式碼前，確認以下都不存在：

```
❌ 白色或淺色側欄（側欄必須是 #0f172a 或更深的深色）
❌ 直接使用 NDataTable（必須用 AppTable）
❌ 直接使用 NTag 表示狀態（必須用 StatusTag）
❌ 扁平/無邊框的狀態標籤（StatusTag 必須有 border + dot 設計）
❌ inline style 中有 hex color（#xxxxxx）
❌ inline style 中有 px 數值（必須用 var(--space-x)）
❌ 使用 any 型別
❌ API call 沒有 try/catch
❌ 刪除/部署操作沒有 ConfirmModal
❌ 頁面的 import 不是從 @/components 來的
❌ UUID/hash/ID 欄位沒有使用 .mono class
❌ 字型用了 serif 或系統預設 serif（技術值必須等寬）
❌ 狀態色直接用 string literal '#16a34a'（必須用 colors.statusSemantic.xxx）
❌ StatCard 的值文字是語意色（值必須是 --text-primary，只有 accent bar 是彩色）
❌ AppTable 沒有 border-radius: 10px 的容器包裝
```

---

## 互動規範

### Hover 狀態

- 可點擊元素必須有 hover 狀態（cursor: pointer + background 微變化）
- 過渡時長：`transition: 0.12s`（快速，像 Linear）
- 卡片懸浮：`box-shadow` 從 `shadow-card` → `shadow-elevated`，`0.15s`

### Loading 狀態

- 表格載入：`AppTable :loading="true"` — NDataTable 內建 spinner
- 按鈕載入：`NButton :loading="true"` — 禁用 + spinner
- 頁面初始：skeleton 或 spinner 居中於內容區

### Focus Ring

- 使用 `var(--shadow-glow)` = `0 0 0 3px rgba(79,126,248,0.18)`
- 應用於 input focus、button focus

### 過渡時長

| 場景 | 時長 |
|------|------|
| hover 背景 | 0.12s |
| 卡片陰影 | 0.15s |
| 側欄收合 | 0.2s |
| Modal 開關 | 0.2s |
| 頁面路由切換 | 不加過渡（直接切換）|

---

## 頁面級別規範

### 列表頁（List Page）

視覺結構（從上到下）：

```
┌─────────────────────────────────────────────┐
│  PageHeader（標題 + 副標題 + 操作按鈕）       │  padding-bottom: 16px, border-bottom
├─────────────────────────────────────────────┤
│  SearchBar（篩選列）                          │  margin-top: 16px
├─────────────────────────────────────────────┤
│  AppTable（資料表格）                         │  margin-top: 16px, flex: 1
│  - 表頭：bg-surface-raised, 11px uppercase   │
│  - 列：48px 高, hover: bg-hover             │
│  - 技術值欄：.mono-cell                      │
│  - 分頁器：右下角                            │
└─────────────────────────────────────────────┘
```

父容器：`display: flex; flex-direction: column; height: 100%; overflow: hidden;`

---

### 詳情頁（Detail Page）

視覺結構：

```
┌─────────────────────────────────────────────┐
│  PageHeader（標題：release_id / fqdn / ...） │
├──────────────────┬──────────────────────────┤
│  左欄（320px）    │  右欄（flex: 1）          │
│  基本資訊卡片      │  NTabs（範圍/任務/歷史）  │
│  border-right    │  padding: 24px           │
│  overflow-y: auto│  overflow-y: auto        │
└──────────────────┴──────────────────────────┘
```

左欄資訊項目排版：
- 標籤：12px / `--text-muted` / uppercase（`.label-section`）
- 值：14px / `--text-primary`
- 技術值（UUID 等）：`.mono`
- StatusTag 佔一行

---

### Dashboard 頁

視覺結構：

```
┌─────────────────────────────────────────────┐
│  PageHeader（Dashboard 標題）                │
├─────────────────────────────────────────────┤
│  StatCard × 4（grid 1:1:1:1，gap: 16px）     │  padding: 24px
│  - 左側 3px 彩色 accent bar                  │
│  - 標籤 12px uppercase, 值 28px bold         │
├─────────────────────────────────────────────┤
│  Section 標題（.label-section）              │  14px uppercase
├─────────────────────────────────────────────┤
│  AppTable（告警 / 執行中 Release / ...）      │
└─────────────────────────────────────────────┘
```

StatCard 顏色建議（依狀態語意）：
- 執行中 Release：`#b45309`（progress）
- 今日成功：`#15803d`（success）
- 失敗 / 待處理：`#b91c1c`（danger）
- 在線 Agent：`#4f7ef8`（primary）
