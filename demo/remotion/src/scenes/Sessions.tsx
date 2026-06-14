import React from "react";
import { AbsoluteFill, interpolate, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "../theme";
import { Caption, Kicker, Reveal } from "../ui";
import { DemoProps } from "../schema";

const lines = [
  "$ claude code: refactor checkout service",
  "$ opencode: write e2e tests for payments",
  "$ gemini: explain webhook retry policy",
  "$ codex: scaffold migration for orders",
  "$ pi: summarize last deploy incident",
];

export const Sessions: React.FC<DemoProps> = ({ accent, cliList }) => {
  const frame = useCurrentFrame();
  const { fps, durationInFrames } = useVideoConfig();

  return (
    <AbsoluteFill
      style={{
        background: theme.bg,
        padding: 90,
        gap: 28,
        justifyContent: "center",
      }}
    >
      <Reveal delay={2}>
        <Kicker accent={accent}>step 1 — observe</Kicker>
      </Reveal>
      <Caption delay={8}>
        worrel watches sessions from{" "}
        <b style={{ color: theme.ink }}>{cliList.join(", ")}</b> — flowing in,
        100% on your machine.
      </Caption>

      {/* CLI chips */}
      <div style={{ display: "flex", gap: 12, flexWrap: "wrap", marginTop: 6 }}>
        {cliList.map((c, i) => (
          <Reveal key={c} delay={20 + i * 5} y={16}>
            <span
              style={{
                fontFamily: font.mono,
                fontSize: 18,
                color: theme.inkSoft,
                background: theme.surface,
                border: `1px solid ${theme.line}`,
                borderRadius: 999,
                padding: "8px 18px",
              }}
            >
              {c}
            </span>
          </Reveal>
        ))}
      </div>

      {/* Terminal feed */}
      <div
        style={{
          marginTop: 18,
          background: theme.ink,
          borderRadius: 16,
          padding: "22px 26px",
          boxShadow: "0 24px 60px -28px rgba(40,30,10,.45)",
          maxWidth: 1100,
        }}
      >
        <div style={{ display: "flex", gap: 8, marginBottom: 18 }}>
          {["#ff5f57", "#febc2e", "#28c840"].map((c) => (
            <div key={c} style={{ width: 13, height: 13, borderRadius: 999, background: c }} />
          ))}
        </div>
        {lines.map((l, i) => {
          const start = 30 + i * 14;
          const op = interpolate(frame, [start, start + 10], [0, 1], {
            extrapolateLeft: "clamp",
            extrapolateRight: "clamp",
          });
          // gentle exit so the feed feels alive
          const exit = interpolate(
            frame,
            [durationInFrames - 16, durationInFrames - 2],
            [1, 0.4],
            { extrapolateLeft: "clamp", extrapolateRight: "clamp" }
          );
          return (
            <div
              key={l}
              style={{
                fontFamily: font.mono,
                fontSize: 21,
                lineHeight: 1.9,
                opacity: op * exit,
                color: i === lines.length - 1 ? accent : "#d8cfbd",
                transform: `translateX(${interpolate(op, [0, 1], [-14, 0])}px)`,
              }}
            >
              {l}
              {i === lines.length - 1 && (
                <span style={{ opacity: Math.round(frame / (fps / 2)) % 2 }}> ▋</span>
              )}
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
};
