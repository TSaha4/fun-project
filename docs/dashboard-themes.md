# Dashboard theme system

The dashboard uses a small React `ThemeProvider` and a CSS-variable design system. The active theme is exposed as `data-theme` on the root HTML element. The selected theme ID is the only dashboard preference written to browser `localStorage`, under `local-capture-theme`; note content and other captured data are never stored there. If storage is disabled or fails, the chosen theme remains active for the current session and the app falls back to Warm house on the next visit.

Available themes are Warm house (default), Coder / Terminal, Cutesy Pastel, Anime-inspired Editorial, and Focus / Minimal. Each theme defines semantic CSS tokens for ink, muted ink, canvas, surfaces, borders, accent, focus ring, typography, corner radii, shadows, and motion duration. Theme classes consume tokens rather than loading theme-specific image/font bundles. No theme asset bundle is downloaded until selected because the current design uses CSS and system fonts only.

## Adding a theme

1. Add its stable ID to `themes` in `web/src/lib/theme/themes.ts` and add a display label to `themeLabels`.
2. Add a `:root[data-theme="your-id"]` block in `web/src/app/globals.css` with every semantic token from the existing theme blocks.
3. Add optional small theme selectors for card or typography treatments only when token changes cannot express the design.
4. Check text, control, selected-navigation, accent-button, and focus-ring contrast against each surface. Keep focus indicators visible in every theme.
5. Keep reduced-motion behavior in the shared `prefers-reduced-motion` rule. Set `--motion-duration: 0ms` for themes that should have no transitions.
6. Run `npm test` in `web`; the theme registry test checks that each registered ID has a display label.

## Accessibility and future companion support

Theme selection is a labeled native select, works by keyboard, and is present in the compact mobile navigation. The CSS focus ring uses a theme token. A global reduced-motion media query suppresses motion regardless of selected theme. Palette tokens use dark readable text on light surfaces or light text on dark surfaces; accent buttons define a separate foreground token. New palettes should be checked with a contrast analyzer before release.

The CSS tokens `--pet-accent`, `--pet-outline`, `--pet-shape`, and `--pet-motion` provide theme inputs for a future companion component. No pet currently exists in the dashboard, so these tokens do not load or render pet art. Future pet assets should be loaded by the companion component only when that feature is enabled.
