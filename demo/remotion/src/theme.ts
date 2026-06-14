// Brand tokens lifted from landing/styles.css to keep the demo on-brand.
export const theme = {
  bg: "#fbf8f1",
  surface: "#ffffff",
  surfaceWarm: "#f4eee1",
  surfaceSunk: "#f7f2e8",
  ink: "#191510",
  inkSoft: "#3a342b",
  muted: "#6e6557",
  faint: "#98907f",
  line: "#e7e0d2",
  line2: "#d8cfbd",
  orange: "#ff6a1a",
  orangeInk: "#c34800",
  pink: "#ee2e96",
  amber: "#ffc02e",
  sky: "#2fa4ee",
  green: "#1f9d57",
  red: "#e23b3b",
  fillPink: "#fce0ee",
  fillAmber: "#fff1c2",
  fillSky: "#dceefc",
  fillGreen: "#d9f1e2",
} as const;

export const font = {
  display: '"Inter Tight", system-ui, sans-serif',
  sans: '"Inter", system-ui, -apple-system, "Segoe UI", sans-serif',
  mono: '"JetBrains Mono", "SFMono-Regular", ui-monospace, Menlo, monospace',
} as const;

export const shadow = {
  md: "0 4px 16px rgba(40,30,10,.06), 0 1px 3px rgba(40,30,10,.05)",
  lg: "0 24px 60px -28px rgba(40,30,10,.30)",
};

export const radius = { sm: 8, md: 12, lg: 18, pill: 999 };
