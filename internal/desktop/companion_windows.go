//go:build windows

package desktop

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"os/exec"
	"runtime"
	"strings"
	"syscall"

	"example.com/local-capture/internal/app"
	"example.com/local-capture/internal/domain"
	reminderengine "example.com/local-capture/internal/reminders"
	"github.com/lxn/walk"
	"github.com/lxn/win"
)

// CaptureService is the application boundary used by the desktop adapter. Implementations
// must validate and persist through the application layer, never through UI-owned SQL.
type CaptureService interface {
	CreateNote(context.Context, app.CreateNoteInput) (domain.Note, error)
}

type VaultControl interface {
	IsUnlocked() bool
	Lock() error
}

type ReminderRunner interface {
	Run(context.Context, reminderengine.Notifier) error
}

type Config struct {
	Service      CaptureService
	Vault        VaultControl
	DashboardURL string
	Collections  []domain.Collection
	Reminders    ReminderRunner
}

// Run starts the native tray companion. With no approved vault-backed service it remains
// locked and refuses to save, avoiding accidental plaintext persistence.
func Run(ctx context.Context, cfg Config) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	if cfg.DashboardURL == "" {
		cfg.DashboardURL = "http://127.0.0.1:3000"
	}
	if err := ValidateDashboardURL(cfg.DashboardURL); err != nil {
		return err
	}
	window, err := walk.NewMainWindow()
	if err != nil {
		return fmt.Errorf("create desktop window: %w", err)
	}
	window.SetTitle("Quick Capture")
	window.SetSize(walk.Size{Width: 440, Height: 390})
	_ = window.SetMinMaxSize(walk.Size{Width: 400, Height: 340}, walk.Size{Width: 0, Height: 0})
	window.Closing().Attach(func(canceled *bool, _ walk.CloseReason) {
		*canceled = true
		window.Hide()
	})

	var title *walk.LineEdit
	var body *walk.TextEdit
	var kind *walk.ComboBox
	var collection *walk.ComboBox
	var status *walk.Label
	var save *walk.PushButton
	if err := buildCaptureForm(window, &title, &body, &kind, &collection, &status, &save); err != nil {
		return fmt.Errorf("build capture form: %w", err)
	}
	collectionNames := []string{"No collection"}
	for _, c := range cfg.Collections {
		collectionNames = append(collectionNames, c.Name)
	}
	_ = collection.SetModel(collectionNames)
	_ = collection.SetCurrentIndex(0)

	icon, err := makeTrayIcon()
	if err != nil {
		return fmt.Errorf("load tray icon: %w", err)
	}
	defer icon.Dispose()
	tray, err := walk.NewNotifyIcon(window)
	if err != nil {
		return fmt.Errorf("create tray icon: %w", err)
	}
	defer tray.Dispose()
	if err = tray.SetIcon(icon); err != nil {
		return err
	}
	if err = tray.SetToolTip("Quick Capture - Ctrl+Shift+Space"); err != nil {
		return err
	}
	show := func() {
		window.Show()
		window.SetFocus()
		body.SetFocus()
	}
	var saving submissionGate
	tray.MouseDown().Attach(func(_, _ int, button walk.MouseButton) {
		if button == walk.LeftButton {
			show()
		}
	})
	addTrayAction(tray, "Open capture", show)
	addTrayAction(tray, "Open dashboard", func() { _ = openDashboard(cfg.DashboardURL) })
	addTrayAction(tray, "Lock vault", func() {
		if saving.IsBusy() {
			status.SetText("Wait for the save to finish before locking.")
			return
		}
		if cfg.Vault != nil {
			_ = cfg.Vault.Lock()
		}
		if scheduler, ok := cfg.Reminders.(interface{ Wake() }); ok {
			scheduler.Wake()
		}
		setLockedStatus(cfg, status, save)
	})
	_ = tray.ContextMenu().Actions().Add(walk.NewSeparatorAction())
	addTrayAction(tray, "Exit", func() {
		cancel()
		walk.App().Exit(0)
	})
	if err = tray.SetVisible(true); err != nil {
		return err
	}
	setLockedStatus(cfg, status, save)
	if cfg.Reminders != nil {
		go func() {
			if err := cfg.Reminders.Run(runCtx, trayReminderNotifier{tray: tray}); err != nil && runCtx.Err() == nil {
				window.Synchronize(func() { status.SetText("Reminder notifications are unavailable.") })
			}
		}()
	}

	save.Clicked().Attach(func() {
		if !saving.Acquire() {
			return
		}
		if !isUnlocked(cfg) {
			status.SetText("Vault is locked. Unlock it before saving.")
			saving.Release()
			return
		}
		content := body.Text()
		if strings.TrimSpace(content) == "" {
			status.SetText("Enter some text to save.")
			saving.Release()
			return
		}
		selectedType := NoteTypeForIndex(kind.CurrentIndex())
		noteTitle := strings.TrimSpace(title.Text())
		if noteTitle == "" {
			noteTitle = SuggestedTitle(content)
		}
		collectionID := ""
		if idx := collection.CurrentIndex(); idx > 0 && idx <= len(cfg.Collections) {
			collectionID = cfg.Collections[idx-1].ID
		}
		save.SetEnabled(false)
		status.SetText("Saving...")
		go func() {
			_, saveErr := cfg.Service.CreateNote(runCtx, app.CreateNoteInput{Title: noteTitle, Content: content, Type: selectedType, CollectionID: collectionID})
			if runCtx.Err() != nil {
				return
			}
			window.Synchronize(func() {
				defer saving.Release()
				save.SetEnabled(isUnlocked(cfg))
				if saveErr != nil {
					status.SetText("Could not save. Check vault state and try again.")
					return
				}
				title.SetText("")
				body.SetText("")
				status.SetText("Saved.")
			})
		}()
	})

	hotkeyReady, stopHotkey := startGlobalHotkey(func() { window.Synchronize(show) })
	defer stopHotkey()
	if !hotkeyReady {
		status.SetText("Shortcut unavailable (Ctrl+Shift+Space may be in use). Use the tray menu.")
	}
	// Show the capture surface once on launch. Closing it hides it to the tray;
	// this also provides a visible first-run entry point if the hotkey is occupied.
	window.Show()
	_ = window.SetFocus()
	go func() {
		<-ctx.Done()
		window.Synchronize(func() { walk.App().Exit(0) })
	}()
	window.Run()
	return nil
}

func makeTrayIcon() (*walk.Icon, error) {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	blue := color.RGBA{R: 35, G: 98, B: 190, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 2; y < 30; y++ {
		for x := 2; x < 30; x++ {
			img.SetRGBA(x, y, blue)
		}
	}
	for y := 8; y < 24; y++ {
		for x := 9; x < 23; x++ {
			if y == 8 || y == 23 || x == 9 || x == 22 || y == 13 || y == 18 {
				img.SetRGBA(x, y, white)
			}
		}
	}
	return walk.NewIconFromImage(img)
}

func buildCaptureForm(w *walk.MainWindow, title **walk.LineEdit, body **walk.TextEdit, kind **walk.ComboBox, collection **walk.ComboBox, status **walk.Label, save **walk.PushButton) error {
	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 16, VNear: 16, HFar: 16, VFar: 14})
	if err := w.SetLayout(layout); err != nil {
		return err
	}
	addLabel := func(text string) error {
		l, e := walk.NewLabel(w)
		if e == nil {
			l.SetText(text)
		}
		return e
	}
	if err := addLabel("Title (optional)"); err != nil {
		return err
	}
	var err error
	if *title, err = walk.NewLineEdit(w); err != nil {
		return err
	}
	if err = addLabel("Capture"); err != nil {
		return err
	}
	if *body, err = walk.NewTextEdit(w); err != nil {
		return err
	}
	_ = (*body).SetMinMaxSize(walk.Size{Width: 0, Height: 130}, walk.Size{Width: 0, Height: 180})
	if err = addLabel("Type"); err != nil {
		return err
	}
	if *kind, err = walk.NewDropDownBox(w); err != nil {
		return err
	}
	if err = (*kind).SetModel([]string{"Thought", "Idea", "Task", "Link", "Image", "Quote", "Code snippet", "Reference"}); err != nil {
		return err
	}
	_ = (*kind).SetCurrentIndex(0)
	if err = addLabel("Collection"); err != nil {
		return err
	}
	if *collection, err = walk.NewDropDownBox(w); err != nil {
		return err
	}
	if *status, err = walk.NewLabel(w); err != nil {
		return err
	}
	if *save, err = walk.NewPushButton(w); err != nil {
		return err
	}
	(*save).SetText("Save")
	return nil
}

func addTrayAction(tray *walk.NotifyIcon, label string, fn func()) {
	a := walk.NewAction()
	_ = a.SetText(label)
	a.Triggered().Attach(fn)
	_ = tray.ContextMenu().Actions().Add(a)
}

type trayReminderNotifier struct{ tray *walk.NotifyIcon }

func (n trayReminderNotifier) Notify(ctx context.Context, _ reminderengine.Notice) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return n.tray.ShowInfo("Reminder", "A reminder is due.")
}

func setLockedStatus(cfg Config, status *walk.Label, save *walk.PushButton) {
	if isUnlocked(cfg) {
		status.SetText("Ready to capture.")
		save.SetEnabled(true)
		return
	}
	status.SetText("Vault is locked. Unlock it before saving.")
	save.SetEnabled(false)
}

func isUnlocked(cfg Config) bool {
	return cfg.Service != nil && cfg.Vault != nil && cfg.Vault.IsUnlocked()
}

func openDashboard(raw string) error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw).Start()
}

const (
	hotkeyID   = 73
	modControl = 0x0002
	modShift   = 0x0004
	vkSpace    = 0x20
	wmHotkey   = 0x0312
	wmQuit     = 0x0012
)

var user32 = syscall.NewLazyDLL("user32.dll")
var registerHotKey = user32.NewProc("RegisterHotKey")
var unregisterHotKey = user32.NewProc("UnregisterHotKey")
var postThreadMessage = user32.NewProc("PostThreadMessageW")
var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getCurrentThreadID = kernel32.NewProc("GetCurrentThreadId")

func startGlobalHotkey(onHotkey func()) (bool, func()) {
	completed := make(chan struct{})
	ready := make(chan bool, 1)
	var threadID uint32
	go func() {
		defer close(completed)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		id, _, _ := getCurrentThreadID.Call()
		threadID = uint32(id)
		ok, _, _ := registerHotKey.Call(0, hotkeyID, modControl|modShift, vkSpace)
		ready <- ok != 0
		if ok == 0 {
			return
		}
		defer unregisterHotKey.Call(0, hotkeyID)
		var msg win.MSG
		for {
			ret := win.GetMessage(&msg, 0, 0, 0)
			if ret <= 0 {
				return
			}
			if msg.Message == wmHotkey && msg.WParam == hotkeyID {
				onHotkey()
			}
		}
	}()
	ok := <-ready
	if !ok {
		<-completed
		return false, func() {}
	}
	return true, func() {
		postThreadMessage.Call(uintptr(threadID), wmQuit, 0, 0)
		<-completed
	}
}
