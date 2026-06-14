import { z } from "zod";

export const sampleProjectSchema = z.object({
  name: z.string(),
  blurb: z.string(),
  memories: z.number(),
  skills: z.number(),
});

export const demoPropsSchema = z.object({
  // Personalization slot for the lead (name + optional logo glyph/initials).
  leadName: z.string(),
  leadInitials: z.string(),
  // Accent color overrides the brand orange in highlights.
  accent: z.string(),
  // CLIs whose sessions flow into worrel.
  cliList: z.array(z.string()),
  // Synthetic example projects distilled by worrel.
  sampleProjects: z.array(sampleProjectSchema),
  // Skills surfaced in the skills scene.
  skills: z.array(z.string()),
  // Closing call to action.
  ctaText: z.string(),
});

export type DemoProps = z.infer<typeof demoPropsSchema>;
export type SampleProject = z.infer<typeof sampleProjectSchema>;

export const defaultProps: DemoProps = {
  leadName: "your team",
  leadInitials: "we",
  accent: "#ff6a1a",
  cliList: ["Claude Code", "OpenCode", "Gemini", "Codex", "Pi"],
  sampleProjects: [
    { name: "acme-payments", blurb: "PCI scope + webhooks", memories: 18, skills: 5 },
    { name: "blog-engine", blurb: "SSG + content pipeline", memories: 9, skills: 3 },
    { name: "mobile-checkout", blurb: "RN flow + analytics", memories: 12, skills: 4 },
  ],
  skills: ["Deploy staging pipeline", "Rotate API keys", "Run e2e before merge"],
  ctaText: "npx worrel",
};
