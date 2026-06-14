import type { Branding } from "./api";

export type ThemeId = "blue-white" | "green-white" | "dark";

export const THEME_LABELS: Record<ThemeId, string> = {
  "blue-white": "临床蓝",
  "green-white": "人文绿",
  dark: "科技深色",
};

// Design tokens ported from the Claude Design prototype (docs/design/).
// Each theme provides the full token set; components read only var(--token).
const THEME_TOKENS: Record<ThemeId, Record<string, string>> = {
  "blue-white": {
    "--color-primary": "#1677ff",
    "--color-primary-hover": "#4096ff",
    "--color-primary-active": "#0a5fe0",
    "--color-primary-soft": "#e2eeff",
    "--color-primary-softer": "#f2f7ff",
    "--color-bg": "#eef3fb",
    "--color-surface": "#ffffff",
    "--color-surface-2": "#f6f9fd",
    "--color-surface-3": "#eef3f9",
    "--color-nav-bg": "#0d1f38",
    "--color-nav-bg-2": "#0a1830",
    "--color-nav-text": "#9bacc9",
    "--color-nav-active-text": "#ffffff",
    "--color-nav-active-bg": "rgba(22,119,255,0.20)",
    "--color-nav-hover-bg": "rgba(255,255,255,0.06)",
    "--color-nav-border": "rgba(255,255,255,0.08)",
    "--color-text": "#16233b",
    "--color-text-2": "#56657f",
    "--color-text-3": "#8a98b0",
    "--color-text-disabled": "#b6c0d2",
    "--color-border": "#e3e9f2",
    "--color-border-strong": "#cdd6e4",
    "--color-divider": "#eef2f8",
    "--color-success": "#16a05c",
    "--color-success-soft": "#e6f6ed",
    "--color-warning": "#e2920f",
    "--color-warning-soft": "#fbf1de",
    "--color-danger": "#e23b3b",
    "--color-danger-soft": "#fdecec",
    "--color-info": "#1677ff",
    "--color-info-soft": "#e8f1ff",
    "--shadow-sm": "0 1px 2px rgba(16,32,64,.07)",
    "--shadow-md": "0 6px 20px rgba(16,32,64,.09)",
    "--shadow-lg": "0 16px 40px rgba(16,32,64,.16)",
    "--ring": "0 0 0 3px rgba(22,119,255,.18)",
    "--button-radius": "8px",
    "--font-size-base": "14px",
  },
  "green-white": {
    "--color-primary": "#13a065",
    "--color-primary-hover": "#1bb574",
    "--color-primary-active": "#0e8553",
    "--color-primary-soft": "#d6f0e2",
    "--color-primary-softer": "#f0faf5",
    "--color-bg": "#eef8f2",
    "--color-surface": "#ffffff",
    "--color-surface-2": "#f4faf6",
    "--color-surface-3": "#eaf4ee",
    "--color-nav-bg": "#0d3d2a",
    "--color-nav-bg-2": "#0a3122",
    "--color-nav-text": "#9cbfae",
    "--color-nav-active-text": "#ffffff",
    "--color-nav-active-bg": "rgba(19,160,101,0.26)",
    "--color-nav-hover-bg": "rgba(255,255,255,0.06)",
    "--color-nav-border": "rgba(255,255,255,0.08)",
    "--color-text": "#173024",
    "--color-text-2": "#566b60",
    "--color-text-3": "#869b90",
    "--color-text-disabled": "#b3c6bc",
    "--color-border": "#dceae2",
    "--color-border-strong": "#c4d8cc",
    "--color-divider": "#eaf3ee",
    "--color-success": "#16a05c",
    "--color-success-soft": "#e0f3e9",
    "--color-warning": "#e2920f",
    "--color-warning-soft": "#fbf1de",
    "--color-danger": "#e23b3b",
    "--color-danger-soft": "#fdecec",
    "--color-info": "#13a065",
    "--color-info-soft": "#e2f3ea",
    "--shadow-sm": "0 1px 2px rgba(16,48,32,.07)",
    "--shadow-md": "0 6px 20px rgba(16,48,32,.09)",
    "--shadow-lg": "0 16px 40px rgba(16,48,32,.16)",
    "--ring": "0 0 0 3px rgba(19,160,101,.20)",
    "--button-radius": "8px",
    "--font-size-base": "14px",
  },
  dark: {
    "--color-primary": "#4d8dff",
    "--color-primary-hover": "#6ba1ff",
    "--color-primary-active": "#3a7cf0",
    "--color-primary-soft": "rgba(77,141,255,.18)",
    "--color-primary-softer": "rgba(77,141,255,.10)",
    "--color-bg": "#0a0f1c",
    "--color-surface": "#121a2c",
    "--color-surface-2": "#0e1525",
    "--color-surface-3": "#1b2540",
    "--color-nav-bg": "#070b16",
    "--color-nav-bg-2": "#05080f",
    "--color-nav-text": "#8696b5",
    "--color-nav-active-text": "#ffffff",
    "--color-nav-active-bg": "rgba(77,141,255,0.20)",
    "--color-nav-hover-bg": "rgba(255,255,255,0.05)",
    "--color-nav-border": "rgba(255,255,255,0.07)",
    "--color-text": "#e7edf9",
    "--color-text-2": "#9fadc7",
    "--color-text-3": "#6b7a96",
    "--color-text-disabled": "#48556e",
    "--color-border": "#26324e",
    "--color-border-strong": "#36456a",
    "--color-divider": "#1c2740",
    "--color-success": "#2ec27e",
    "--color-success-soft": "rgba(46,194,126,.16)",
    "--color-warning": "#f0a83c",
    "--color-warning-soft": "rgba(240,168,60,.16)",
    "--color-danger": "#ff5a5a",
    "--color-danger-soft": "rgba(255,90,90,.16)",
    "--color-info": "#4d8dff",
    "--color-info-soft": "rgba(77,141,255,.16)",
    "--shadow-sm": "0 1px 2px rgba(0,0,0,.4)",
    "--shadow-md": "0 6px 20px rgba(0,0,0,.45)",
    "--shadow-lg": "0 16px 44px rgba(0,0,0,.6)",
    "--ring": "0 0 0 3px rgba(77,141,255,.32)",
    "--button-radius": "8px",
    "--font-size-base": "14px",
  },
};

export function applyTheme(themeId: ThemeId, branding?: Branding) {
  const tokens = { ...THEME_TOKENS[themeId] };
  if (branding?.primary_color) tokens["--color-primary"] = branding.primary_color;
  if (branding?.secondary_color)
    tokens["--color-primary-soft"] = branding.secondary_color;
  if (branding?.button_radius) tokens["--button-radius"] = branding.button_radius;
  if (branding?.font_size) tokens["--font-size-base"] = branding.font_size;

  const root = document.documentElement;
  for (const [key, value] of Object.entries(tokens)) {
    root.style.setProperty(key, value);
  }
  root.dataset.theme = themeId;
}
