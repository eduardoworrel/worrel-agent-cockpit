import React from "react";
import { interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "./theme";

// Fade + rise on entry, optional exit fade near the end of the local frame window.
export const Reveal: React.FC<{
  children: React.ReactNode;
  delay?: number;
  y?: number;
  style?: React.CSSProperties;
}> = ({ children, delay = 0, y = 24, style }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const s = spring({ frame: frame - delay, fps, config: { damping: 200 } });
  const opacity = interpolate(s, [0, 1], [0, 1]);
  const translate = interpolate(s, [0, 1], [y, 0]);
  return (
    <div style={{ opacity, transform: `translateY(${translate}px)`, ...style }}>
      {children}
    </div>
  );
};

// Brand logo lockup (orange dot + wordmark), mirrors the landing header.
export const Brand: React.FC<{ accent: string; size?: number }> = ({
  accent,
  size = 1,
}) => (
  <div style={{ display: "flex", alignItems: "center", gap: 12 * size }}>
    <div
      style={{
        width: 34 * size,
        height: 34 * size,
        borderRadius: 10 * size,
        background: accent,
        boxShadow: `0 6px 18px -6px ${accent}aa`,
      }}
    />
    <div style={{ display: "flex", flexDirection: "column", lineHeight: 1 }}>
      <span
        style={{
          fontFamily: font.display,
          fontWeight: 700,
          fontSize: 28 * size,
          letterSpacing: "-.03em",
          color: theme.ink,
        }}
      >
        worrel
      </span>
      <span
        style={{
          fontFamily: font.mono,
          fontSize: 10 * size,
          letterSpacing: ".14em",
          textTransform: "uppercase",
          color: theme.faint,
          marginTop: 4 * size,
        }}
      >
        agent cockpit
      </span>
    </div>
  </div>
);

// Mono kicker label used across scenes.
export const Kicker: React.FC<{ children: React.ReactNode; accent: string }> = ({
  children,
  accent,
}) => (
  <span
    style={{
      fontFamily: font.mono,
      fontSize: 16,
      fontWeight: 600,
      letterSpacing: ".18em",
      textTransform: "uppercase",
      color: accent,
    }}
  >
    {children}
  </span>
);

export const Caption: React.FC<{ children: React.ReactNode; delay?: number }> = ({
  children,
  delay = 0,
}) => (
  <Reveal delay={delay} y={14}>
    <p
      style={{
        fontFamily: font.sans,
        fontSize: 26,
        color: theme.muted,
        margin: 0,
        maxWidth: 820,
        lineHeight: 1.4,
      }}
    >
      {children}
    </p>
  </Reveal>
);
