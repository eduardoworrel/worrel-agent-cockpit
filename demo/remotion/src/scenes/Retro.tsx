import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "../theme";
import { Caption, Kicker, Reveal } from "../ui";
import { DemoProps } from "../schema";

// "Retroactive analysis" — a scan bar sweeps a history strip, distilling insights.
export const Retro: React.FC<DemoProps> = ({ accent }) => {
  const frame = useCurrentFrame();
  const { fps, durationInFrames } = useVideoConfig();

  const scanX = interpolate(frame, [20, durationInFrames - 30], [0, 100], {
    extrapolateLeft: "clamp",
    extrapolateRight: "clamp",
  });

  const bars = Array.from({ length: 28 });
  const insightSpring = spring({ frame: frame - 70, fps, config: { damping: 16 } });

  return (
    <AbsoluteFill style={{ background: theme.bg, padding: 90, justifyContent: "center", gap: 28 }}>
      <Reveal delay={2}>
        <Kicker accent={accent}>step 3 — retroactive analysis</Kicker>
      </Reveal>
      <Caption delay={8}>
        worrel re-reads your{" "}
        <b style={{ color: theme.ink }}>past history</b> and distills what you already knew.
      </Caption>

      {/* History strip with sweeping scan bar */}
      <div
        style={{
          position: "relative",
          background: theme.surface,
          border: `1px solid ${theme.line}`,
          borderRadius: 16,
          padding: "26px 28px",
          height: 180,
          display: "flex",
          alignItems: "flex-end",
          gap: 8,
          overflow: "hidden",
          boxShadow: "0 4px 16px rgba(40,30,10,.06)",
        }}
      >
        {bars.map((_, i) => {
          const pos = (i / bars.length) * 100;
          const scanned = pos < scanX;
          const h = 30 + Math.abs(Math.sin(i * 1.3)) * 90;
          return (
            <div
              key={i}
              style={{
                flex: 1,
                height: h,
                borderRadius: 4,
                background: scanned ? accent : theme.line2,
                opacity: scanned ? 1 : 0.6,
                transition: "background .2s",
              }}
            />
          );
        })}
        {/* scan line */}
        <div
          style={{
            position: "absolute",
            top: 0,
            bottom: 0,
            left: `${scanX}%`,
            width: 3,
            background: accent,
            boxShadow: `0 0 24px 6px ${accent}88`,
          }}
        />
      </div>

      {/* Distilled insight chips */}
      <div
        style={{
          display: "flex",
          gap: 16,
          opacity: interpolate(insightSpring, [0, 1], [0, 1]),
          transform: `translateY(${interpolate(insightSpring, [0, 1], [20, 0])}px)`,
        }}
      >
        {[
          "12 reusable skills found",
          "47 memories recovered",
          "3 projects reconstructed",
        ].map((t) => (
          <div
            key={t}
            style={{
              flex: 1,
              background: theme.surfaceWarm,
              border: `1px solid ${theme.line}`,
              borderRadius: 12,
              padding: "16px 20px",
              fontFamily: font.display,
              fontSize: 22,
              fontWeight: 650,
              color: theme.ink,
              textAlign: "center",
            }}
          >
            {t}
          </div>
        ))}
      </div>
    </AbsoluteFill>
  );
};
