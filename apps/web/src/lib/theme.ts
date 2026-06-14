import type { Branding } from "./api";

export type ThemeId = "blue-white" | "green-white";

const THEME_TOKENS: Record<
  ThemeId,
  Record<string, string>
> = {
  "blue-white": {
    "--color-primary": "#1677ff",
    "--color-primary-light": "#69b1ff",
    "--color-bg": "#f5f8ff",
    "--color-surface": "#ffffff",
    "--color-nav-bg": "#001529",
    "--color-nav-text": "#ffffff",
    "--color-text": "#1f1f1f",
    "--button-radius": "6px",
    "--font-size-base": "14px",
  },
  "green-white": {
    "--color-primary": "#13a065",
    "--color-primary-light": "#5cdba8",
    "--color-bg": "#f0faf5",
    "--color-surface": "#ffffff",
    "--color-nav-bg": "#0d3d2a",
    "--color-nav-text": "#ffffff",
    "--color-text": "#1f1f1f",
    "--button-radius": "6px",
    "--font-size-base": "14px",
  },
};

export function applyTheme(themeId: ThemeId, branding?: Branding) {
  const tokens = { ...THEME_TOKENS[themeId] };
  if (branding?.primary_color) tokens["--color-primary"] = branding.primary_color;
  if (branding?.secondary_color)
    tokens["--color-primary-light"] = branding.secondary_color;
  if (branding?.button_radius) tokens["--button-radius"] = branding.button_radius;
  if (branding?.font_size) tokens["--font-size-base"] = branding.font_size;

  const root = document.documentElement;
  for (const [key, value] of Object.entries(tokens)) {
    root.style.setProperty(key, value);
  }
  root.dataset.theme = themeId;
}
