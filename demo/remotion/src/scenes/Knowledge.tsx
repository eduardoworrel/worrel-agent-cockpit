import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "../theme";
import { Caption, Kicker, Reveal } from "../ui";
import { DemoProps } from "../schema";

export const Knowledge: React.FC<DemoProps> = ({ accent, sampleProjects, skills }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();

  const card = (delay: number) => {
    const s = spring({ frame: frame - delay, fps, config: { damping: 18, mass: 0.7 } });
    return {
      opacity: interpolate(s, [0, 1], [0, 1]),
      transform: `translateY(${interpolate(s, [0, 1], [28, 0])}px) scale(${interpolate(
        s,
        [0, 1],
        [0.96, 1]
      )})`,
    };
  };

  return (
    <AbsoluteFill style={{ background: theme.bg, padding: 90, justifyContent: "center", gap: 26 }}>
      <Reveal delay={2}>
        <Kicker accent={accent}>step 2 — distill</Kicker>
      </Reveal>
      <Caption delay={8}>
        It distills <b style={{ color: theme.ink }}>Projects</b>,{" "}
        <b style={{ color: theme.ink }}>versioned Memories</b>, and{" "}
        <b style={{ color: theme.ink }}>Skills</b> with lineage.
      </Caption>

      <div style={{ display: "flex", gap: 22, marginTop: 10 }}>
        {/* Projects column */}
        <div style={{ flex: 1.2, display: "flex", flexDirection: "column", gap: 14 }}>
          {sampleProjects.map((p, i) => (
            <div
              key={p.name}
              style={{
                ...card(22 + i * 8),
                background: theme.surface,
                border: `1px solid ${theme.line}`,
                borderRadius: 14,
                padding: "18px 22px",
                boxShadow: theme.surface && "0 4px 16px rgba(40,30,10,.06)",
              }}
            >
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
                <b style={{ fontFamily: font.display, fontSize: 24, color: theme.ink }}>{p.name}</b>
                <span style={{ fontFamily: font.mono, fontSize: 14, color: theme.faint }}>
                  project
                </span>
              </div>
              <div style={{ fontFamily: font.sans, fontSize: 17, color: theme.muted, marginTop: 6 }}>
                {p.blurb}
              </div>
              <div style={{ display: "flex", gap: 8, marginTop: 12 }}>
                <span style={pill(theme.fillPink, "#a01464")}>{p.memories} memories</span>
                <span style={pill(theme.fillSky, "#155f93")}>{p.skills} skills</span>
              </div>
            </div>
          ))}
        </div>

        {/* Skills column */}
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 14 }}>
          <div style={{ ...card(26), ...panel() }}>
            <div style={{ fontFamily: font.mono, fontSize: 14, color: theme.faint, marginBottom: 14 }}>
              SKILLS · with lineage
            </div>
            {skills.map((sk, i) => (
              <div
                key={sk}
                style={{
                  ...card(34 + i * 9),
                  display: "flex",
                  alignItems: "center",
                  gap: 12,
                  padding: "12px 14px",
                  borderRadius: 9,
                  background: theme.surfaceWarm,
                  marginBottom: 10,
                }}
              >
                <span style={pill(theme.fillSky, "#155f93")}>skill</span>
                <span style={{ fontFamily: font.sans, fontSize: 17, color: theme.inkSoft, fontWeight: 500 }}>
                  {sk}
                </span>
              </div>
            ))}
            <div
              style={{
                ...card(34 + skills.length * 9),
                fontFamily: font.mono,
                fontSize: 13,
                color: theme.orangeInk,
                background: theme.fillAmber,
                borderRadius: 8,
                padding: "8px 12px",
                marginTop: 4,
              }}
            >
              v3 ← v2 ← v1 · provenance kept
            </div>
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
};

const pill = (bg: string, color: string): React.CSSProperties => ({
  fontFamily: font.mono,
  fontSize: 13,
  fontWeight: 500,
  padding: "3px 11px",
  borderRadius: 999,
  background: bg,
  color,
});

const panel = (): React.CSSProperties => ({
  background: theme.surface,
  border: `1px solid ${theme.line}`,
  borderRadius: 14,
  padding: "20px 22px",
  boxShadow: "0 4px 16px rgba(40,30,10,.06)",
});
