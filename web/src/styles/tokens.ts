/**
 * Design Tokens — Single source of truth for all visual values.
 *
 * Theme: Operational Clarity — dark sidebar, bright content, clean data presentation.
 * Reference aesthetic: Linear, Vercel dashboard, Grafana.
 *
 * Rules:
 *  - NEVER hardcode a color/spacing/font-size in a component.
 *  - ALWAYS use a value from this file or the corresponding CSS variable.
 *  - Adding a new token requires updating BOTH this file AND global.css.
 */

// ── Colors ─────────────────────────────────────────────────────────────────

export const colors = {
  // Backgrounds
  bgPage:         '#f0f4f8',              // page root — soft blue-gray wash
  bgCard:         '#ffffff',              // card, table, modal surface
  bgSurface:      '#ffffff',              // card/panel surface (alias for bgCard)
  bgSurfaceRaised:'#f8fafc',             // slightly raised (table header, sub-sections)
  bgInput:        '#f8fafc',              // input, select background
  bgHover:        'rgba(79, 126, 248, 0.05)',  // row hover — barely-there blue tint

  // Sidebar — dark slate theme
  bgSidebar:       '#0f172a',             // deep navy slate
  bgSidebarHover:  'rgba(255,255,255,0.06)',
  bgSidebarActive: 'rgba(79,126,248,0.18)',
  sidebarText:     '#94a3b8',
  sidebarTextActive:'#ffffff',
  sidebarBorder:   'rgba(255,255,255,0.08)',

  // Borders & dividers
  border:    '#e4e9f2',
  borderSub: '#f1f5fb',  // subtler divider inside cards

  // Text
  textPrimary:   '#1a2233',
  textSecondary: '#4b5a6e',
  textMuted:     '#8b97a8',

  // Brand / primary
  primary:        '#4f7ef8',  // clear sky blue — confident, approachable
  primaryHover:   '#3b6ef0',
  primaryPressed: '#2c5de0',

  // Status semantic colors — richer, higher-contrast for dark-sidebar context.
  // Three state machines (Domain Lifecycle / Release / Agent) all share
  // these six semantic buckets via the StatusTag component's semanticMap.
  // See FRONTEND_GUIDE.md §"顏色使用規範".
  statusSemantic: {
    success:   { color: '#15803d', bg: 'rgba(21,128,61,0.10)',   border: 'rgba(21,128,61,0.20)'   },  // active, online, succeeded, idle
    progress:  { color: '#b45309', bg: 'rgba(180,83,9,0.10)',    border: 'rgba(180,83,9,0.20)'    },  // executing, busy, provisioned, pending, planning, ready
    warning:   { color: '#c2410c', bg: 'rgba(194,65,12,0.10)',   border: 'rgba(194,65,12,0.20)'   },  // paused, draining, requested, approved
    danger:    { color: '#b91c1c', bg: 'rgba(185,28,28,0.10)',   border: 'rgba(185,28,28,0.20)'   },  // failed, error, rolling_back, rolled_back, disabled, offline
    neutral:   { color: '#64748b', bg: 'rgba(100,116,139,0.10)', border: 'rgba(100,116,139,0.20)' },  // retired, cancelled, registered
    upgrading: { color: '#6d28d9', bg: 'rgba(109,40,217,0.10)',  border: 'rgba(109,40,217,0.20)'  },  // upgrading
  },

  // Alert severity (PRD §16 — P1 / P2 / P3 / INFO)
  severity: {
    P1:   { color: '#b91c1c', bg: 'rgba(185,28,28,0.10)'   },
    P2:   { color: '#c2410c', bg: 'rgba(194,65,12,0.10)'   },
    P3:   { color: '#2563eb', bg: 'rgba(37,99,235,0.08)'   },
    INFO: { color: '#15803d', bg: 'rgba(21,128,61,0.10)'   },
  },
} as const

// ── Spacing (4px base grid) ─────────────────────────────────────────────────

export const spacing = {
  1:  '4px',
  2:  '8px',
  3:  '12px',
  4:  '16px',
  5:  '20px',
  6:  '24px',
  8:  '32px',
  10: '40px',
  12: '48px',
  16: '64px',
} as const

// ── Typography ──────────────────────────────────────────────────────────────

export const fontSize = {
  xs:   '12px',
  sm:   '13px',
  base: '14px',
  md:   '16px',
  lg:   '20px',
  xl:   '24px',
  '2xl':'28px',
} as const

export const fontWeight = {
  normal:   400,
  medium:   500,
  semibold: 600,
  bold:     700,
} as const

export const lineHeight = {
  tight:   1.3,
  normal:  1.5,
  relaxed: 1.6,
} as const

// Monospace stack for technical values: UUIDs, checksums, release IDs, agent IDs
export const fontMono = "ui-monospace, 'SF Mono', 'Cascadia Code', Consolas, monospace"

// ── Borders ─────────────────────────────────────────────────────────────────

export const borderRadius = {
  sm:   '4px',
  base: '6px',
  md:   '8px',
  lg:   '12px',
  full: '9999px',
} as const

// ── Shadows — layered, modern depth ──────────────────────────────────────────

export const shadow = {
  sm:       '0 1px 2px rgba(15,23,42,0.04), 0 1px 4px rgba(15,23,42,0.04)',
  card:     '0 0 0 1px rgba(15,23,42,0.06), 0 2px 8px rgba(15,23,42,0.06)',
  elevated: '0 0 0 1px rgba(15,23,42,0.08), 0 4px 16px rgba(15,23,42,0.10)',
  modal:    '0 0 0 1px rgba(15,23,42,0.10), 0 16px 48px rgba(15,23,42,0.16)',
  glow:     '0 0 0 3px rgba(79,126,248,0.18)',
} as const

// ── Layout constants ─────────────────────────────────────────────────────────

export const layout = {
  sidebarWidth:         '220px',
  sidebarCollapsedWidth:'56px',
  headerHeight:         '56px',
  pageHeaderHeight:     '64px',
  tableRowHeight:       '48px',
  searchBarHeight:      '52px',
  contentMaxWidth:      '1400px',
  contentPadding:       '24px',
  // Sidebar group label typography
  sidebarGroupLabel: {
    fontSize:      '11px',
    textTransform: 'uppercase' as const,
    letterSpacing: '0.8px',
  },
} as const

// ── Naive UI theme overrides (imported in App.vue) ──────────────────────────
// Light theme — do NOT pass darkTheme in App.vue

import type { GlobalThemeOverrides } from 'naive-ui'

export const naiveThemeOverrides: GlobalThemeOverrides = {
  common: {
    primaryColor:        colors.primary,
    primaryColorHover:   colors.primaryHover,
    primaryColorPressed: colors.primaryPressed,
    primaryColorSuppl:   colors.primaryPressed,

    bodyColor:          colors.bgPage,
    cardColor:          colors.bgCard,
    modalColor:         colors.bgCard,
    popoverColor:       colors.bgCard,
    tableColor:         colors.bgCard,
    tableColorHover:    colors.bgHover,
    tableHeaderColor:   colors.bgSurfaceRaised,
    inputColor:         colors.bgInput,
    inputColorDisabled: '#f1f5f9',

    borderColor:   colors.border,
    dividerColor:  colors.border,

    textColorBase: colors.textPrimary,
    textColor1:    colors.textPrimary,
    textColor2:    colors.textSecondary,
    textColor3:    colors.textMuted,
    placeholderColor:    colors.textMuted,
    scrollbarColor:      '#d1d9e6',
    scrollbarColorHover: '#aab5c8',

    fontFamily: "'Inter', system-ui, -apple-system, 'Segoe UI', sans-serif",
    fontSize:   '14px',
    borderRadius: '8px',
  },
  Button: {
    borderRadiusMedium: '8px',
    borderRadiusLarge:  '8px',
    borderRadiusSmall:  '6px',
    fontWeightStrong:   '600',
  },
  Input: {
    borderRadius: '8px',
    color:        colors.bgInput,
    colorFocus:   colors.bgCard,
    border:       `1px solid ${colors.border}`,
    borderHover:  `1px solid ${colors.primary}`,
    borderFocus:  `1px solid ${colors.primary}`,
    boxShadowFocus: `0 0 0 3px rgba(79,126,248,0.15)`,
  },
  Select: {
    peers: {
      InternalSelection: {
        borderRadius: '8px',
        border:       `1px solid ${colors.border}`,
        borderHover:  `1px solid ${colors.primary}`,
        borderFocus:  `1px solid ${colors.primary}`,
        boxShadowFocus: `0 0 0 3px rgba(79,126,248,0.15)`,
      },
    },
  },
  DataTable: {
    thPaddingMedium:    '0 16px',
    tdPaddingMedium:    '0 16px',
    thFontWeight:       '600',
    thTextColor:        colors.textSecondary,   // more readable than textMuted
    thColor:            '#f4f6fb',              // slightly deeper table header bg
    borderColor:        colors.border,
    borderRadius:       '10px',
    tdTextColor:        colors.textPrimary,
    fontSizeMedium:     '13.5px',
  },
  Card: {
    borderRadius: '12px',
    boxShadow:    shadow.card,
    paddingMedium:'20px 24px',
  },
  Modal: {
    borderRadius: '14px',
    boxShadow:    shadow.modal,
  },
  Tag: {
    borderRadius: '6px',
  },
}
