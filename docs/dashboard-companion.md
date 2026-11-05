# Optional dashboard companion

The dashboard companion is an optional personality feature. It is disabled by default, can be hidden separately from being disabled, and has no sound. Preferences are stored locally under `local-capture-companion-preferences`; only the `enabled` and `visible` booleans are stored. The note API and note model do not depend on the companion.

The first character is a lightweight inline SVG panda. Its drawing uses CSS variables for colors and simple state class transitions; it does not fetch image assets or load an animation/game engine. The dock is part of the sidebar layout rather than a floating overlay, and the character art itself ignores pointer events. The reminder/success announcements use a polite live region only for meaningful events.

## Event and state contract

Application code calls `useCompanion().emit(...)` with semantic states. The current event set is `activity`, `working`, `saving`, `save_success`, `sleepy`, `inbox_cleared`, and `reminder_notification`; the provider exposes the full state enum including `idle` and `blink`. Save operations emit saving/success, note actions emit working/activity, clearing the last visible Inbox note emits an inbox celebration, and metadata refresh emits one notification state per due reminder instant. The provider detects page-local keyboard/pointer activity and changes to a short blink; after two minutes without page activity it switches to sleepy. It does not observe clipboard contents or input outside the dashboard.

Animations are one-shot CSS keyframes for blink, save, celebration, and reminder attention. No loop runs while the companion is idle. The shared reduced-motion media query suppresses these keyframes. There is no sound path.

## Replacing characters

`CompanionDock.tsx` owns the `characterRegistry` and `themeCharacters` mapping. Add a character component that accepts `{ state: CompanionState }`, register it under a new `CharacterID`, then point one or more theme IDs at it. Keep semantic state names and event wiring in the provider/dashboard; character components should render visual state only. Theme-dependent paint can use `--pet-accent`, `--pet-outline`, `--pet-shape`, `--pet-motion`, `--panda-dark`, and `--panda-light`. Load any future character asset only when its character component is selected, and keep it optional so disabling the companion avoids rendering the art.

No dedicated companion art asset is bundled; the provided reference was used as a general cute-character cue, and the current panda is drawn directly as SVG. Verify the art at small and narrow viewport sizes when changing it.
