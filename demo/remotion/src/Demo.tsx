import React from "react";
import { AbsoluteFill, Sequence, useVideoConfig } from "remotion";
import { DemoProps } from "./schema";
import { theme } from "./theme";
import { Intro } from "./scenes/Intro";
import { Sessions } from "./scenes/Sessions";
import { Knowledge } from "./scenes/Knowledge";
import { Retro } from "./scenes/Retro";
import { Outro } from "./scenes/Outro";

// Scene durations in seconds (sums to ~26s at the configured fps).
export const SCENE_SECONDS = {
  intro: 4.5,
  sessions: 6,
  knowledge: 6.5,
  retro: 5,
  outro: 4,
};

export const Demo: React.FC<DemoProps> = (props) => {
  const { fps } = useVideoConfig();
  const f = (s: number) => Math.round(s * fps);

  let cursor = 0;
  const next = (s: number) => {
    const from = cursor;
    cursor += f(s);
    return from;
  };

  return (
    <AbsoluteFill style={{ background: theme.bg }}>
      <Sequence from={next(SCENE_SECONDS.intro)} durationInFrames={f(SCENE_SECONDS.intro)}>
        <Intro {...props} />
      </Sequence>
      <Sequence from={next(SCENE_SECONDS.sessions)} durationInFrames={f(SCENE_SECONDS.sessions)}>
        <Sessions {...props} />
      </Sequence>
      <Sequence from={next(SCENE_SECONDS.knowledge)} durationInFrames={f(SCENE_SECONDS.knowledge)}>
        <Knowledge {...props} />
      </Sequence>
      <Sequence from={next(SCENE_SECONDS.retro)} durationInFrames={f(SCENE_SECONDS.retro)}>
        <Retro {...props} />
      </Sequence>
      <Sequence from={next(SCENE_SECONDS.outro)} durationInFrames={f(SCENE_SECONDS.outro)}>
        <Outro {...props} />
      </Sequence>
    </AbsoluteFill>
  );
};

export const TOTAL_SECONDS = Object.values(SCENE_SECONDS).reduce((a, b) => a + b, 0);
