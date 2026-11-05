"use client";

import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { LocalAPIError, describeAPIError } from "@/lib/api/errors";
import { localCaptureAPI } from "@/lib/api/client";
import { useTheme } from "@/lib/theme/ThemeProvider";
import { themeLabels, themes, type ThemeId } from "@/lib/theme/themes";
import { useCompanion } from "@/lib/companion/CompanionProvider";
import { CompanionDock, CompanionSettings } from "@/lib/companion/CompanionDock";
import type { Collection, Note, NoteInput, NoteQuery, NoteType, Reminder, Tag } from "@/lib/api/types";

const PAGE_SIZE = 50;
type View = "inbox" | "all" | "collections" | "tags" | "pinned" | "archived" | "reminders" | "search";
const views: { id: View; label: string; icon: string }[] = [
  { id: "inbox", label: "Inbox", icon: "↓" }, { id: "all", label: "All notes", icon: "▤" },
  { id: "collections", label: "Collections", icon: "▱" }, { id: "tags", label: "Tags", icon: "⌗" },
  { id: "pinned", label: "Pinned", icon: "◆" }, { id: "archived", label: "Archived", icon: "□" },
  { id: "reminders", label: "Reminders", icon: "◷" },
];

export default function Home() {
  const { theme, setTheme } = useTheme();
  const { emit: companionEvent, enabled: companionEnabled } = useCompanion();
  const [view, setView] = useState<View>("inbox");
  const [notes, setNotes] = useState<Note[]>([]);
  const [collections, setCollections] = useState<Collection[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [reminders, setReminders] = useState<Reminder[]>([]);
  const [loading, setLoading] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [hasMore, setHasMore] = useState(false);
  const [error, setError] = useState<unknown>(null);
  const [query, setQuery] = useState("");
  const [appliedQuery, setAppliedQuery] = useState("");
  const [activeCollection, setActiveCollection] = useState<string | null>(null);
  const [activeTag, setActiveTag] = useState<string | null>(null);
  const [editorNote, setEditorNote] = useState<Note | null | undefined>(undefined);
  const [busyID, setBusyID] = useState<string | null>(null);
  const [message, setMessage] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);
  const noteListRef = useRef<HTMLUListElement>(null);
  const loadedCount = useRef(0);
  const requestSequence = useRef(0);
  const dueReminderEvents = useRef(new Set<string>());

  const locked = error instanceof LocalAPIError && error.kind === "vault_locked";
  const errorMessage = error ? describeAPIError(error) : null;
  const collectionByID = useMemo(() => new Map(collections.map(item => [item.id, item.name])), [collections]);
  const tagByID = useMemo(() => new Map(tags.map(item => [item.id, item.name])), [tags]);
  const reminderByNote = useMemo(() => new Map(reminders.filter(item => !item.completed_at).map(item => [item.note_id, item])), [reminders]);
  const currentTitle = view === "search" ? "Search results" : view === "collections" && activeCollection ? collectionByID.get(activeCollection) ?? "Collection" : view === "tags" && activeTag ? `# ${tagByID.get(activeTag) ?? "Tag"}` : views.find(item => item.id === view)?.label ?? "Workspace";

  const loadResources = useCallback(async (reset = true) => {
    const sequence = reset ? ++requestSequence.current : requestSequence.current;
    if (reset) { setLoading(true); setNotes([]); setHasMore(false); loadedCount.current = 0; }
    else setLoadingMore(true);
    setError(null);
    try {
      const offset = reset ? 0 : loadedCount.current;
      let page: Note[];
      const pageSize = view === "inbox" ? 20 : PAGE_SIZE;
      if (view === "search") {
        if (!appliedQuery.trim()) page = [];
        else page = await localCaptureAPI.searchNotes({ text: appliedQuery, limit: PAGE_SIZE, offset });
      } else if (view === "reminders" || view === "collections" && !activeCollection || view === "tags" && !activeTag) {
        page = [];
      } else {
        const filter: NoteQuery = { limit: pageSize, offset };
        if (view === "archived") filter.archived = true;
        else if (view !== "all") filter.archived = false;
        if (view === "pinned") filter.pinned = true;
        if (view === "collections" && activeCollection) filter.collection_id = activeCollection;
        if (view === "tags" && activeTag) filter.tag_id = activeTag;
        page = await localCaptureAPI.listNotes(filter);
      }
      if (sequence !== requestSequence.current) return;
      setNotes(current => reset ? page : [...current, ...page]);
      loadedCount.current = offset + page.length;
      setHasMore(page.length === pageSize);
    } catch (reason) {
      if (sequence !== requestSequence.current) return;
      setError(reason);
      if (reason instanceof LocalAPIError && (reason.kind === "vault_locked" || reason.kind === "authentication")) {
        setNotes([]); setReminders([]); setEditorNote(undefined);
      }
    } finally { if (sequence === requestSequence.current) { setLoading(false); setLoadingMore(false); } }
  }, [view, activeCollection, activeTag, appliedQuery]);

  const loadMetadata = useCallback(async () => {
    const results = await Promise.allSettled([localCaptureAPI.listCollections(), localCaptureAPI.listTags(), localCaptureAPI.listReminders()]);
    if (results[0].status === "fulfilled") setCollections(results[0].value);
    if (results[1].status === "fulfilled") setTags(results[1].value);
    if (results[2].status === "fulfilled") setReminders(results[2].value);
    for (const result of results) if (result.status === "rejected") setError((current: unknown) => current ?? result.reason);
  }, []);

  const announceDueReminders = useCallback((items: Reminder[]) => {
    const now = Date.now();
    let hasNewDue = false;
    for (const reminder of items) {
      const dueAt = reminder.snoozed_until ?? reminder.due_at;
      const key = `${reminder.id}:${dueAt}`;
      if (!reminder.completed_at && new Date(dueAt).getTime() <= now && !dueReminderEvents.current.has(key)) {
        dueReminderEvents.current.add(key);
        hasNewDue = true;
      }
    }
    if (hasNewDue) companionEvent("reminder_notification");
  }, [companionEvent]);

  useEffect(() => {
    if (!companionEnabled) return;
    let timer = 0;
    const scheduleNext = () => {
      window.clearTimeout(timer);
      announceDueReminders(reminders);
      const now = Date.now();
      const nextDue = reminders
        .filter(item => !item.completed_at)
        .map(item => new Date(item.snoozed_until ?? item.due_at).getTime())
        .filter(time => time > now)
        .sort((left, right) => left - right)[0];
      if (nextDue !== undefined) timer = window.setTimeout(scheduleNext, Math.min(2_147_483_647, Math.max(1, nextDue - now)));
    };
    const onVisible = () => { if (document.visibilityState === "visible") scheduleNext(); };
    scheduleNext();
    document.addEventListener("visibilitychange", onVisible);
    return () => { window.clearTimeout(timer); document.removeEventListener("visibilitychange", onVisible); };
  }, [reminders, announceDueReminders, companionEnabled]);

  useEffect(() => { void loadResources(true); }, [loadResources]);
  useEffect(() => { void loadMetadata(); }, [loadMetadata]);

  const runAction = useCallback(async (id: string | null, action: () => Promise<unknown>, success: string, refresh = true, completionEvent: "activity" | "inbox_cleared" = "activity") => {
    setBusyID(id); setError(null); setMessage("");
    companionEvent("working");
    try { await action(); setMessage(success); if (refresh) { await loadResources(true); await loadMetadata(); } companionEvent(completionEvent); }
    catch (reason) { setError(reason); companionEvent("idle"); }
    finally { setBusyID(null); }
  }, [loadResources, loadMetadata, companionEvent]);

  async function openEditor(note?: Note) {
    if (!note) { setEditorNote(null); return; }
    setBusyID(note.id);
    try { setEditorNote(await localCaptureAPI.getNote(note.id)); }
    catch (reason) { setError(reason); }
    finally { setBusyID(null); }
  }

  async function openNoteByID(id: string) {
    try { setEditorNote(await localCaptureAPI.getNote(id)); }
    catch (reason) { setError(reason); }
  }

  async function saveNote(input: NoteInput, id?: string, reminderDue?: string) {
    setBusyID(id ?? "new"); setError(null);
    companionEvent("saving");
    let saved: Note;
    try {
      saved = id ? await localCaptureAPI.updateNote(id, input) : await localCaptureAPI.createNote(input);
    } catch (reason) {
      setError(reason); setBusyID(null); companionEvent("idle"); return;
    }
    setEditorNote(undefined); setMessage(id ? "Changes saved." : "Note added to your workspace.");
    companionEvent("save_success");
    if (reminderDue) {
      try {
        const dueAt = new Date(reminderDue).toISOString();
        const existing = reminders.find(item => item.note_id === saved.id);
        if (existing) await localCaptureAPI.updateReminder(existing.id, dueAt);
        else await localCaptureAPI.createReminder(saved.id, dueAt);
      } catch (reason) { setError(reason); setMessage("Note saved, but the reminder could not be saved."); }
    }
    try {
      await loadResources(true); await loadMetadata();
    } finally { setBusyID(null); }
  }

  async function changePin(note: Note) {
    const input: NoteInput = { title: note.title, content: note.content, type: note.type, collection_id: note.collection_id ?? "", tag_ids: note.tag_ids, pinned: !note.pinned, source_url: note.metadata?.source_url ?? "", attachment_ids: note.attachment_ids };
    await runAction(note.id, () => localCaptureAPI.updateNote(note.id, input), note.pinned ? "Removed from pinned." : "Pinned note.");
  }

  async function duplicate(note: Note) {
    const input: NoteInput = { title: `${note.title || "Untitled"} (copy)`, content: note.content, type: note.type, ...(note.collection_id ? { collection_id: note.collection_id } : {}), tag_ids: note.tag_ids, pinned: false, source_url: note.metadata?.source_url ?? "", attachment_ids: note.attachment_ids };
    await runAction(note.id, () => localCaptureAPI.createNote(input), "Note duplicated.");
  }

  async function moveNote(note: Note, collectionID: string) {
    const input: NoteInput = { title: note.title, content: note.content, type: note.type, collection_id: collectionID, tag_ids: note.tag_ids, pinned: note.pinned, source_url: note.metadata?.source_url ?? "", attachment_ids: note.attachment_ids };
    await runAction(note.id, () => localCaptureAPI.updateNote(note.id, input), "Collection updated.");
  }

  async function updateTags(note: Note, tagID: string, add: boolean) {
    const ids = add ? [...new Set([...note.tag_ids, tagID])] : note.tag_ids.filter(id => id !== tagID);
    const action = add ? () => localCaptureAPI.assignTags(note.id, ids) : () => localCaptureAPI.removeTag(note.id, tagID);
    await runAction(note.id, action, add ? "Tag added." : "Tag removed.");
  }

  function runCardAction(note: Note, action: () => Promise<unknown>, success: string) {
    const inboxMayClear = view === "inbox" && notes.length === 1 && (success === "Note archived." || success === "Note deleted.");
    void runAction(note.id, action, success, true, inboxMayClear ? "inbox_cleared" : "activity");
  }

  function submitSearch(event: FormEvent) { event.preventDefault(); setView("search"); setAppliedQuery(query.trim()); }

  useEffect(() => {
    function onKey(event: KeyboardEvent) {
      const target = event.target as HTMLElement;
      const typing = target.matches("input, textarea, select, [contenteditable=true]");
      if (event.key === "/" && !typing) { event.preventDefault(); searchRef.current?.focus(); }
      else if ((event.key.toLowerCase() === "n" && !typing) || (event.key === "Escape" && editorNote !== undefined)) {
        if (event.key === "n") { event.preventDefault(); setEditorNote(null); }
        else setEditorNote(undefined);
      } else if (!typing && (event.key === "j" || event.key === "k")) {
        const items = [...(noteListRef.current?.querySelectorAll<HTMLButtonElement>("[data-note-title]") ?? [])];
        const index = items.indexOf(document.activeElement as HTMLButtonElement);
        items[Math.max(0, Math.min(items.length - 1, index + (event.key === "j" ? 1 : -1))) ]?.focus();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [editorNote]);

  return <div className="workspace">
    <aside className="sidebar" aria-label="Workspace navigation">
      <a className="wordmark" href="#workspace">Personal workspace<span>LOCAL</span></a>
      <button className="new-note-button" onClick={() => setEditorNote(null)} disabled={locked}><span>＋</span> New note <kbd>N</kbd></button>
      <form className="sidebar-search" onSubmit={submitSearch}><span aria-hidden="true">⌕</span><input ref={searchRef} value={query} onChange={event => setQuery(event.target.value)} placeholder="Search notes" aria-label="Search notes" /><kbd>/</kbd></form>
      <nav className="main-nav">{views.map(item => <button key={item.id} className={view === item.id ? "nav-item active" : "nav-item"} onClick={() => { setView(item.id); setActiveCollection(null); setActiveTag(null); }}><span className="nav-icon">{item.icon}</span>{item.label}{item.id === "reminders" && reminders.filter(reminder => !reminder.completed_at).length > 0 && <small>{reminders.filter(reminder => !reminder.completed_at).length}</small>}</button>)}</nav>
      <div className="sidebar-section"><div className="section-label">COLLECTIONS <button aria-label="Create collection" onClick={() => { const name = window.prompt("Collection name"); if (name?.trim()) void runAction(null, () => localCaptureAPI.createCollection(name.trim()), "Collection created."); }}>＋</button></div>
        {collections.map(item => <button key={item.id} className={view === "collections" && activeCollection === item.id ? "nav-item subnav active" : "nav-item subnav"} onClick={() => { setView("collections"); setActiveCollection(item.id); setActiveTag(null); }}><span className="collection-dot" />{item.name}</button>)}
      </div>
      <div className="sidebar-section"><div className="section-label">TAGS</div>{tags.slice(0, 12).map(tag => <button key={tag.id} className={view === "tags" && activeTag === tag.id ? "nav-item subnav active" : "nav-item subnav"} onClick={() => { setView("tags"); setActiveTag(tag.id); setActiveCollection(null); }}><span className="tag-hash">#</span>{tag.name}</button>)}</div>
      <div className="theme-control"><label htmlFor="theme-picker">Appearance</label><select id="theme-picker" value={theme} onChange={event => setTheme(event.target.value as ThemeId)}>{themes.map(id => <option key={id} value={id}>{themeLabels[id]}</option>)}</select></div>
      <CompanionDock />
      <CompanionSettings />
      <div className="sidebar-foot">Local-first · stored on this device</div>
    </aside>

    <main className="main-area" id="workspace">
      <header className="topbar"><div className="breadcrumbs"><span>Workspace</span><b>/</b><strong>{currentTitle}</strong></div><button className="refresh-button" onClick={() => { void loadResources(true); void loadMetadata(); }} disabled={loading}>↻ <span>Refresh</span></button></header>
      {(errorMessage || message) && <div className={errorMessage ? locked ? "workspace-alert danger" : "workspace-alert" : "workspace-message"} role={errorMessage ? "alert" : "status"}>{errorMessage ? <><strong>{errorMessage.title}</strong><span>{errorMessage.message}</span><button onClick={() => { void loadResources(true); void loadMetadata(); }}>Retry</button></> : message}</div>}

      {view === "collections" && !activeCollection ? <CollectionDirectory collections={collections} onSelect={id => setActiveCollection(id)} onRename={(item, name) => void runAction(null, () => localCaptureAPI.updateCollection(item.id, name), "Collection renamed.")} onDelete={item => { if (window.confirm(`Delete collection “${item.name}”?`)) void runAction(null, () => localCaptureAPI.deleteCollection(item.id), "Collection deleted."); }} onCreate={name => void runAction(null, () => localCaptureAPI.createCollection(name), "Collection created.")} />
        : view === "tags" && !activeTag ? <TagDirectory tags={tags} onSelect={id => setActiveTag(id)} onCreate={name => void runAction(null, () => localCaptureAPI.createTag(name), "Tag created.")} />
          : view === "reminders" ? <ReminderView reminders={reminders} notes={notes} onOpenNote={openNoteByID} onSnooze={id => void runAction(id, () => localCaptureAPI.snoozeReminder(id, { preset: "two_hours" }), "Reminder snoozed.")} onComplete={id => void runAction(id, () => localCaptureAPI.completeReminder(id), "Reminder completed.")} />
            : <>
              <div className="page-heading"><div><p className="eyebrow">YOUR LIBRARY</p><h1>{currentTitle}</h1><p>{view === "search" ? `Results for “${appliedQuery}”` : view === "inbox" ? "A quick view of active notes from across your workspace." : view === "archived" ? "Notes you’ve put away." : view === "all" ? "Browse every active note; archived notes have their own view." : "Everything you’ve captured, all in one place."}</p></div><button className="primary-button" onClick={() => setEditorNote(null)} disabled={locked}>＋ <span>New note</span></button></div>
              {view === "search" && !appliedQuery && <div className="empty-state"><strong>Search your workspace</strong><p>Enter a word or phrase above to find matching notes.</p></div>}
              {loading ? <div className="loading-state" role="status"><span className="loading-mark">◌</span><span>Loading your notes…</span></div>
                : notes.length === 0 && !(view === "search" && !appliedQuery) ? <div className="empty-state"><div className="empty-icon">{view === "archived" ? "□" : view === "pinned" ? "◆" : "✳"}</div><strong>{emptyTitle(view)}</strong><p>{emptyDescription(view)}</p>{view !== "archived" && <button className="primary-button" onClick={() => setEditorNote(null)}>＋ Create a note</button>}</div>
                  : notes.length > 0 && <ul className="note-grid" ref={noteListRef} aria-label={`${currentTitle} notes`}>{notes.map(note => <NoteCard key={note.id} note={note} collectionName={note.collection_id ? collectionByID.get(note.collection_id) : undefined} tagNames={note.tag_ids.map(id => tagByID.get(id) ?? id)} reminder={reminderByNote.get(note.id)} busy={busyID === note.id} onOpen={() => void openEditor(note)} onPin={() => void changePin(note)} onArchive={() => runCardAction(note, () => localCaptureAPI.archiveNote(note.id), "Note archived.")} onRestore={() => runCardAction(note, () => localCaptureAPI.restoreNote(note.id), "Note restored.")} onDuplicate={() => void duplicate(note)} onDelete={() => { if (window.confirm(`Permanently delete “${note.title || "Untitled"}”?`)) runCardAction(note, () => localCaptureAPI.deleteNote(note.id), "Note deleted."); }} onMove={id => void moveNote(note, id)} onTag={id => void updateTags(note, id, !note.tag_ids.includes(id))} collections={collections} tags={tags} />)}</ul>}
              {hasMore && <div className="load-more"><button onClick={() => void loadResources(false)} disabled={loadingMore}>{loadingMore ? "Loading…" : "Load more notes"}</button><span>Showing {notes.length} notes</span></div>}
            </>}
    </main>
    {editorNote !== undefined && <NoteEditor note={editorNote} collections={collections} tags={tags} reminders={reminders} busy={busyID === (editorNote?.id ?? "new")} onClose={() => setEditorNote(undefined)} onSave={saveNote} />}
  </div>;
}

function emptyTitle(view: View) { return ({ inbox: "Your inbox is clear", all: "Your workspace is ready", pinned: "Nothing pinned yet", archived: "No archived notes", search: "No matching notes", collections: "This collection is empty", tags: "No notes with this tag" } as Partial<Record<View, string>>)[view] ?? "Nothing here yet"; }
function emptyDescription(view: View) { return ({ inbox: "Your recent notes will appear here.", all: "Capture an idea, task, link, or anything worth keeping.", pinned: "Pin notes you want to find quickly.", archived: "Archived notes will appear here.", search: "Try another search phrase.", collections: "Add a note to this collection to keep related ideas together.", tags: "Choose a different tag or add it to a note." } as Partial<Record<View, string>>)[view] ?? "Create your first note to get started."; }
function localDateTimeValue(value: string) { const date = new Date(value); return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16); }

function NoteCard({ note, collectionName, tagNames, reminder, busy, collections, tags, onOpen, onPin, onArchive, onRestore, onDuplicate, onDelete, onMove, onTag }: { note: Note; collectionName?: string; tagNames: string[]; reminder?: Reminder; busy: boolean; collections: Collection[]; tags: Tag[]; onOpen: () => void; onPin: () => void; onArchive: () => void; onRestore: () => void; onDuplicate: () => void; onDelete: () => void; onMove: (id: string) => void; onTag: (id: string) => void }) {
  const [menuOpen, setMenuOpen] = useState(false);
  return <li><article className="note-card">
    <div className="card-topline"><span className={`type-label type-${note.type}`}>{note.type.replaceAll("_", " ")}</span><div className="card-tools">{note.pinned && <span title="Pinned" aria-label="Pinned">◆</span>}<button className="icon-button" aria-label={`Actions for ${note.title}`} aria-expanded={menuOpen} onClick={() => setMenuOpen(!menuOpen)}>•••</button></div></div>
    {menuOpen && <div className="card-menu"><button onClick={onOpen}>Edit</button><button onClick={onPin}>{note.pinned ? "Unpin" : "Pin"}</button>{note.archived ? <button onClick={onRestore}>Restore</button> : <button onClick={onArchive}>Archive</button>}<button onClick={onDuplicate}>Duplicate</button><label>Move to<select value={note.collection_id ?? ""} onChange={event => onMove(event.target.value)}><option value="">No collection</option>{collections.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label><label>Tags<div className="menu-tags">{tags.map(tag => <button key={tag.id} onClick={() => onTag(tag.id)}>{note.tag_ids.includes(tag.id) ? "✓ " : "+ "}{tag.name}</button>)}</div></label><button className="danger-action" onClick={onDelete}>Delete…</button></div>}
    <button className="card-main" data-note-title="true" onClick={onOpen}><strong>{note.title || "Untitled"}</strong><span className="card-preview">{note.content.slice(0, 180)}{note.content.length > 180 ? "…" : ""}</span></button>
    <div className="card-meta">{collectionName && <span className="collection-label">▱ {collectionName}</span>}{reminder && <span title={`Reminder ${new Date(reminder.due_at).toLocaleString()}`}>◷ Reminder</span>}{note.attachment_ids?.length > 0 && <span title={`${note.attachment_ids.length} attachment(s)`}>▧ {note.attachment_ids.length}</span>}</div>
    <div className="card-footer"><div className="tag-list">{tagNames.slice(0, 3).map((tag, index) => <span className="tag-chip" key={`${note.tag_ids[index]}-${tag}`}>#{tag}</span>)}{tagNames.length > 3 && <span className="tag-chip">+{tagNames.length - 3}</span>}</div><time dateTime={note.created_at}>{new Date(note.created_at).toLocaleDateString(undefined, { month: "short", day: "numeric" })}</time></div>
    {busy && <span className="card-busy">Updating…</span>}
  </article></li>;
}

function NoteEditor({ note, collections, tags, reminders, busy, onClose, onSave }: { note: Note | null; collections: Collection[]; tags: Tag[]; reminders: Reminder[]; busy: boolean; onClose: () => void; onSave: (input: NoteInput, id?: string, reminderDue?: string) => Promise<void> }) {
  const existingReminder = note ? reminders.find(item => item.note_id === note.id) : undefined;
  const [title, setTitle] = useState(note?.title ?? ""); const [content, setContent] = useState(note?.content ?? ""); const [type, setType] = useState<NoteType>(note?.type ?? "thought"); const [collectionID, setCollectionID] = useState(note?.collection_id ?? ""); const [tagIDs, setTagIDs] = useState<string[]>(note?.tag_ids ?? []); const [reminderDue, setReminderDue] = useState(existingReminder ? localDateTimeValue(existingReminder.due_at) : "");
  const types: NoteType[] = ["thought", "idea", "task", "link", "image", "quote", "code_snippet", "reference"];
  function submit(event: FormEvent) { event.preventDefault(); void onSave({ title: title.trim() || content.trim().slice(0, 60), content, type, collection_id: collectionID, tag_ids: tagIDs, pinned: note?.pinned ?? false, source_url: note?.metadata?.source_url ?? "", attachment_ids: note?.attachment_ids ?? [] }, note?.id, reminderDue); }
  return <div className="editor-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) onClose(); }}><section className="editor-panel" role="dialog" aria-modal="true" aria-labelledby="editor-title"><header className="editor-header"><div><p className="eyebrow">{note ? "YOUR LIBRARY" : "QUICK CAPTURE"}</p><h2 id="editor-title">{note ? "Edit note" : "New note"}</h2></div><button className="icon-button" onClick={onClose} aria-label="Close editor">×</button></header><form onSubmit={submit} className="editor-form"><label>Title <span>Optional</span><input autoFocus value={title} onChange={event => setTitle(event.target.value)} maxLength={500} placeholder="Give this note a title" /></label><label>Content<textarea required autoFocus={!note} value={content} onChange={event => setContent(event.target.value)} rows={9} placeholder="Write something worth keeping…" /></label><div className="editor-fields"><label>Type<select value={type} onChange={event => setType(event.target.value as NoteType)}>{types.map(item => <option key={item} value={item}>{item.replaceAll("_", " ")}</option>)}</select></label><label>Collection<select value={collectionID} onChange={event => setCollectionID(event.target.value)}><option value="">No collection</option>{collections.map(item => <option key={item.id} value={item.id}>{item.name}</option>)}</select></label></div><label className="reminder-field">Reminder <span>Optional · local time</span><input type="datetime-local" value={reminderDue} onChange={event => setReminderDue(event.target.value)} /></label><fieldset><legend>Tags</legend><div className="editor-tags">{tags.length ? tags.map(tag => <label key={tag.id}><input type="checkbox" checked={tagIDs.includes(tag.id)} onChange={event => setTagIDs(current => event.target.checked ? [...current, tag.id] : current.filter(id => id !== tag.id))} />#{tag.name}</label>) : <span className="muted">No tags available</span>}</div></fieldset><footer><button type="button" onClick={onClose}>Cancel</button><button className="primary-button" type="submit" disabled={busy || !content.trim()}>{busy ? "Saving…" : note ? "Save changes" : "Save note"}</button></footer></form></section></div>;
}

function CollectionDirectory({ collections, onSelect, onRename, onDelete, onCreate }: { collections: Collection[]; onSelect: (id: string) => void; onRename: (item: Collection, name: string) => void; onDelete: (item: Collection) => void; onCreate: (name: string) => void }) {
  const [name, setName] = useState(""); return <section className="directory-page"><div className="page-heading"><div><p className="eyebrow">ORGANIZE</p><h1>Collections</h1><p>Group related notes around the things you care about.</p></div></div><form className="create-row" onSubmit={event => { event.preventDefault(); if (name.trim()) { onCreate(name.trim()); setName(""); } }}><input value={name} onChange={event => setName(event.target.value)} placeholder="Create a collection" aria-label="Collection name" /><button className="primary-button">＋ Create</button></form>{collections.length ? <div className="directory-grid">{collections.map(item => <article className="directory-card" key={item.id}><button className="directory-open" onClick={() => onSelect(item.id)}><span className="collection-large">▱</span><strong>{item.name}</strong><small>Open collection →</small></button><div className="row"><button onClick={() => { const next = window.prompt("Rename collection", item.name); if (next?.trim()) onRename(item, next.trim()); }}>Rename</button><button onClick={() => onDelete(item)}>Delete</button></div></article>)}</div> : <div className="empty-state"><strong>No collections yet</strong><p>Create collections to group related notes.</p></div>}</section>;
}

function TagDirectory({ tags, onSelect, onCreate }: { tags: Tag[]; onSelect: (id: string) => void; onCreate: (name: string) => void }) {
  const [name, setName] = useState(""); return <section className="directory-page"><div className="page-heading"><div><p className="eyebrow">ORGANIZE</p><h1>Tags</h1><p>Use tags to connect notes across collections.</p></div></div><form className="create-row" onSubmit={event => { event.preventDefault(); if (name.trim()) { onCreate(name.trim()); setName(""); } }}><input value={name} onChange={event => setName(event.target.value)} placeholder="Create a tag" aria-label="Tag name" /><button className="primary-button">＋ Create</button></form>{tags.length ? <div className="tag-directory">{tags.map(tag => <button key={tag.id} onClick={() => onSelect(tag.id)}><span>#</span>{tag.name}<b>→</b></button>)}</div> : <div className="empty-state"><strong>No tags yet</strong><p>Create tags to make notes easier to find.</p></div>}</section>;
}

function ReminderView({ reminders, notes, onOpenNote, onSnooze, onComplete }: { reminders: Reminder[]; notes: Note[]; onOpenNote: (id: string) => void; onSnooze: (id: string) => void; onComplete: (id: string) => void }) {
  const titleFor = new Map(notes.map(note => [note.id, note.title])); return <section className="directory-page"><div className="page-heading"><div><p className="eyebrow">STAY ON TRACK</p><h1>Reminders</h1><p>Things you asked yourself to come back to.</p></div></div>{reminders.length ? <div className="reminder-list">{reminders.map(item => <article key={item.id} className={item.completed_at ? "reminder-row completed" : "reminder-row"}><span className="reminder-glyph">◷</span><div><strong>{titleFor.get(item.note_id) ?? "Note reminder"}</strong><time>{new Date(item.due_at).toLocaleString()}</time></div><div className="row"><button onClick={() => onOpenNote(item.note_id)}>Open note</button>{!item.completed_at && <><button onClick={() => onSnooze(item.id)}>Snooze 2h</button><button onClick={() => onComplete(item.id)}>Complete</button></>}</div>{item.completed_at && <span className="completed-label">Completed</span>}</article>)}</div> : <div className="empty-state"><span className="empty-icon">◷</span><strong>No reminders</strong><p>Add a reminder while editing a note to see it here.</p></div>}</section>;
}
