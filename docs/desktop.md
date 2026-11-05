# Windows desktop companion

The first companion is a native Windows window and system-tray process built with Go and Walk. It uses no Electron runtime or webview. The window appears once on launch, then hides to the tray when closed. The tray menu provides capture, dashboard, vault lock, and exit actions. The process does not add itself to Windows startup.

## Capture behavior

- Global shortcut: `Ctrl+Shift+Space`, registered through Windows `RegisterHotKey`. This asks Windows to deliver a hotkey message; the application does not install a keyboard hook or read keys typed in other applications. If another application owns the shortcut, the companion reports the conflict and tray access remains available.
- The capture window accepts a title, text, note type, and an optional collection. A blank title is suggested from the first line of text.
- Save delegates to the injected `CaptureService`, which is an application-layer boundary. An in-flight gate prevents repeat clicks from submitting a second request.
- The tray dashboard action opens the configured HTTP(S) URL through the Windows URL handler. The default is `http://127.0.0.1:3000`.
- Clipboard access is only through normal user paste. The application does not poll or read the clipboard on its own.
- When an approved vault-backed reminder scheduler is injected, the tray displays a generic Windows notification: “A reminder is due.” Note titles and contents are never passed to the notification API.

## Vault integration boundary

The vault design and key-management plan remain unapproved, and the current SQLite adapter stores plaintext. The desktop command therefore supplies no note service or vault controller; the UI reports that the vault is locked and disables Save. Do not connect the current plaintext adapter to this entry point. A vault-backed composition root must be added after the encryption design is approved. No personal content should be used in the current build.

## Native dependency limitation

The UI uses `github.com/lxn/walk` at the pinned 2021 revision. During testing, upstream Walk failed while registering optional tooltip controls (`TTM_ADDTOOL`) on the current Windows environment. A small local copy under `third_party/walk` disables automatic tooltip registration; standard controls and tray behavior were then able to initialize. Tooltips are not available in this local build. Revisit this fork when selecting a maintained Windows UI dependency for a later release.

The global hotkey can conflict with another application or policy. If so, the app does not fall back to a low-level keyboard hook. Tray and UI behavior depends on the Windows notification area and may vary with shell settings.
