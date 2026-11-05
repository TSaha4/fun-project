// Manifest source is kept as TypeScript; package it to manifest.json when extension features are added.
export const manifest = {
  manifest_version: 3,
  name: "Local Capture Extension",
  version: "0.1.0",
  description: "Browser extension scaffold",
  action: { default_title: "Capture" },
} as const;
