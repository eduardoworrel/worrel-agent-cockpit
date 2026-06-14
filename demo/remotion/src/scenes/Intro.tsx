import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "../theme";
import { Brand, Kicker, Reveal } from "../ui";
import { DemoProps } from "../schema";

export const Intro: React.FC<DemoProps> = ({ leadName, leadInitials, accent }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const badge = spring({ frame: frame - 28, fps, config: { damping: 14, mass: 0.6 } });
  const badgeScale = interpolate(badge, [0, 1], [0.6, 1]);

  return (
    <AbsoluteFill
      style={{
        background: `radial-gradient(120% 80% at 50% 0%, ${theme.surface} 0%, ${theme.bg} 70%)`,
        alignItems: "center",
        justifyContent: "center",
        gap: 40,
      }}
    >
      <Reveal delay={2} y={30}>
        <Brand accent={accent} size={1.9} />
      </Reveal>

      <Reveal delay={12} y={26} style={{ textAlign: "center" }}>
        <Kicker accent={accent}>local-first agent cockpit</Kicker>
        <h1
          style={{
            fontFamily: font.display,
            fontWeight: 680,
            fontSize: 76,
            letterSpacing: "-.025em",
            lineHeight: 1.05,
            color: theme.ink,
            margin: "18px 0 0",
            maxWidth: 1000,
          }}
        >
          Your AI sessions, turned into{" "}
          <em style={{ color: accent, fontStyle: "italic", fontWeight: 600 }}>
            durable knowledge
          </em>
          .
        </h1>
      </Reveal>

      <div
        style={{
          transform: `scale(${badgeScale})`,
          opacity: interpolate(badge, [0, 1], [0, 1]),
          display: "flex",
          alignItems: "center",
          gap: 16,
          background: theme.surface,
          border: `1px solid ${theme.line}`,
          borderRadius: 999,
          padding: "12px 22px 12px 12px",
          boxShadow: "0 24px 60px -28px rgba(40,30,10,.30)",
        }}
      >
        <div
          style={{
            width: 44,
            height: 44,
            borderRadius: 999,
            background: accent,
            color: "#fff",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            fontFamily: font.display,
            fontWeight: 700,
            fontSize: 18,
            textTransform: "uppercase",
          }}
        >
          {leadInitials}
        </div>
        <span style={{ fontFamily: font.sans, fontSize: 22, color: theme.inkSoft }}>
          Prepared for{" "}
          <b style={{ color: theme.ink, fontWeight: 700 }}>{leadName}</b>
        </span>
      </div>
    </AbsoluteFill>
  );
};
