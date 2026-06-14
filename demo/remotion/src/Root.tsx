import React from "react";
import { Composition } from "remotion";
import { Demo, TOTAL_SECONDS } from "./Demo";
import { defaultProps, demoPropsSchema } from "./schema";

const FPS = 30;

export const RemotionRoot: React.FC = () => {
  return (
    <Composition
      id="Demo"
      component={Demo}
      durationInFrames={Math.round(TOTAL_SECONDS * FPS)}
      fps={FPS}
      width={1920}
      height={1080}
      schema={demoPropsSchema}
      defaultProps={defaultProps}
    />
  );
};
