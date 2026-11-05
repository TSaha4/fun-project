import { apiErrorFromResponse, LocalAPIError, type APIErrorBody } from "./errors.ts";
import type {
  Attachment,
  Collection,
  Note,
  NoteInput,
  NoteQuery,
  Reminder,
  ReminderEvent,
  SearchQuery,
  SnoozeInput,
  Tag,
} from "./types.ts";

export type Fetcher = typeof fetch;

export class LocalCaptureAPI {
  private readonly fetcher: Fetcher;
  private readonly basePath: string;

  constructor(fetcher: Fetcher = fetch, basePath = "/api/local") {
    this.fetcher = fetcher;
    this.basePath = basePath;
  }

  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const headers = new Headers(init.headers);
    headers.set("X-Local-Dashboard", "1");
    if (typeof init.body === "string") {
      headers.set("Content-Type", "application/json");
    }
    let response: Response;
    try {
      response = await this.fetcher(`${this.basePath}${path}`, {
        ...init,
        headers,
        credentials: "same-origin",
        cache: "no-store",
      });
    } catch {
      throw new LocalAPIError({
        kind: "network",
        code: "network_error",
        message: "The local API could not be reached.",
      });
    }
    if (!response.ok) {
      let body: APIErrorBody = {};
      try {
        body = (await response.json()) as APIErrorBody;
      } catch {
        // Keep the error generic when an intermediary returns non-JSON content.
      }
      throw apiErrorFromResponse(response.status, body);
    }
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }

  listNotes(query: NoteQuery = {}): Promise<Note[]> {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined && value !== "") params.set(key, String(value));
    }
    const suffix = params.size ? `?${params.toString()}` : "";
    return this.request(`/notes${suffix}`);
  }

  searchNotes(query: SearchQuery): Promise<Note[]> {
    return this.request("/notes/search", { method: "POST", body: JSON.stringify(query) });
  }

  getNote(id: string): Promise<Note> {
    return this.request(`/notes/${encodeURIComponent(id)}`);
  }

  createNote(input: NoteInput): Promise<Note> {
    return this.request("/notes", { method: "POST", body: JSON.stringify(input) });
  }

  updateNote(id: string, input: NoteInput): Promise<Note> {
    return this.request(`/notes/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify(input) });
  }

  archiveNote(id: string): Promise<Note> {
    return this.request(`/notes/${encodeURIComponent(id)}/archive`, { method: "POST" });
  }

  restoreNote(id: string): Promise<Note> {
    return this.request(`/notes/${encodeURIComponent(id)}/restore`, { method: "POST" });
  }

  deleteNote(id: string): Promise<void> {
    return this.request(`/notes/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  listCollections(): Promise<Collection[]> {
    return this.request("/collections");
  }

  createCollection(name: string): Promise<Collection> {
    return this.request("/collections", { method: "POST", body: JSON.stringify({ name }) });
  }

  updateCollection(id: string, name: string): Promise<Collection> {
    return this.request(`/collections/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify({ name }) });
  }

  deleteCollection(id: string): Promise<void> {
    return this.request(`/collections/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  listTags(): Promise<Tag[]> {
    return this.request("/tags");
  }

  createTag(name: string): Promise<Tag> {
    return this.request("/tags", { method: "POST", body: JSON.stringify({ name }) });
  }

  assignTags(noteId: string, tagIds: string[]): Promise<Note> {
    return this.request("/tags/assign", { method: "POST", body: JSON.stringify({ note_id: noteId, tag_ids: tagIds }) });
  }

  removeTag(noteId: string, tagId: string): Promise<Note> {
    return this.request(`/notes/${encodeURIComponent(noteId)}/tags/${encodeURIComponent(tagId)}`, { method: "DELETE" });
  }

  uploadAttachment(noteId: string, file: File): Promise<Attachment> {
    const query = new URLSearchParams({ note_id: noteId });
    return this.request(`/attachments?${query}`, {
      method: "POST",
      headers: { "X-Filename": file.name, "Content-Type": file.type || "application/octet-stream" },
      body: file,
    });
  }

  async downloadAttachment(id: string): Promise<Blob> {
    const headers = new Headers({ "X-Local-Dashboard": "1" });
    let response: Response;
    try {
      response = await this.fetcher(`${this.basePath}/attachments/${encodeURIComponent(id)}`, {
        headers,
        credentials: "same-origin",
        cache: "no-store",
      });
    } catch {
      throw new LocalAPIError({ kind: "network", code: "network_error", message: "The local API could not be reached." });
    }
    if (!response.ok) {
      let body: APIErrorBody = {};
      try { body = (await response.json()) as APIErrorBody; } catch { /* use generic error */ }
      throw apiErrorFromResponse(response.status, body);
    }
    return response.blob();
  }

  deleteAttachment(id: string): Promise<void> {
    return this.request(`/attachments/${encodeURIComponent(id)}`, { method: "DELETE" });
  }

  createReminder(noteId: string, dueAt: string): Promise<Reminder> {
    return this.request("/reminders", { method: "POST", body: JSON.stringify({ note_id: noteId, due_at: dueAt }) });
  }

  updateReminder(id: string, dueAt: string): Promise<Reminder> {
    return this.request(`/reminders/${encodeURIComponent(id)}`, { method: "PUT", body: JSON.stringify({ due_at: dueAt }) });
  }

  listReminders(): Promise<Reminder[]> {
    return this.request("/reminders");
  }

  listDueReminders(): Promise<Reminder[]> {
    return this.request("/reminders/due");
  }

  snoozeReminder(id: string, input: SnoozeInput): Promise<Reminder> {
    return this.request(`/reminders/${encodeURIComponent(id)}/snooze`, { method: "POST", body: JSON.stringify(input) });
  }

  completeReminder(id: string): Promise<Reminder> {
    return this.request(`/reminders/${encodeURIComponent(id)}/complete`, { method: "POST" });
  }

  reminderHistory(id: string): Promise<ReminderEvent[]> {
    return this.request(`/reminders/${encodeURIComponent(id)}/history`);
  }
}

export const localCaptureAPI = new LocalCaptureAPI();
