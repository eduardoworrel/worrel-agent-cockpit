import React from "react";
import { AbsoluteFill, interpolate, spring, useCurrentFrame, useVideoConfig } from "remotion";
import { font, theme } from "../theme";
import { Brand, Reveal } from "../ui";
import { DemoProps } from "../schema";

export const Outro: React.FC<DemoProps> = ({ accent, leadName, ctaText }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const cta = spring({ frame: frame - 30, fps, config: { damping: 13, mass: 0.7 } });

  return (
    <AbsoluteFill
      style={{
        background: `radial-gradient(120% 90% at 50% 100%, ${theme.surfaceWarm} 0%, ${theme.bg} 70%)`,
        alignItems: "center",
        justifyContent: "center",
        gap: 44,
      }}
    >
      <Reveal delay={2} y={26}>
        <Brand accent={accent} size={1.5} />
      </Reveal>

      <Reveal delay={12} style={{ textAlign: "center" }}>
        <h1
          style={{
            fontFamily: font.display,
            fontWeight: 680,
            fontSize: 66,
            letterSpacing: "-.025em",
            color: theme.ink,
            margin: 0,
            maxWidth: 980,
          }}
        >
          Private by default. Yours{" "}
          <em style={{ color: accent, fontStyle: "italic", fontWeight: 600 }}>forever</em>.
        </h1>
        <p
          style={{
            fontFamily: font.sans,
            fontSize: 24,
            color: theme.muted,
            marginTop: 16,
          }}
        >
          Ready when you are, {leadName}.
        </p>
      </Reveal>

      <div
        style={{
          transform: `scale(${interpolate(cta, [0, 1], [0.8, 1])})`,
          opacity: interpolate(cta, [0, 1], [0, 1]),
          fontFamily: font.mono,
          fontSize: 30,
          fontWeight: 600,
          color: "#fff",
          background: theme.ink,
          borderRadius: 12,
          padding: "20px 36px",
          boxShadow: "0 24px 60px -28px rgba(40,30,10,.4)",
        }}
      >
        <span style={{ color: accent }}>$</span> {ctaText}
      </div>
    </AbsoluteFill>
  );
};
