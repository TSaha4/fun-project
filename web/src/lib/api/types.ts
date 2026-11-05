export type NoteType =
  | "thought"
  | "idea"
  | "task"
  | "link"
  | "image"
  | "quote"
  | "code_snippet"
  | "reference";

export interface Note {
  id: string;
  title: string;
  content: string;
  type: NoteType;
  created_at: string;
  updated_at: string;
  collection_id?: string;
  tag_ids: string[];
  pinned: boolean;
  archived: boolean;
  reminder?: Reminder;
  metadata: { source_url?: string };
  attachment_ids: string[];
}

export interface NoteInput {
  title: string;
  content: string;
  type: NoteType;
  collection_id?: string;
  tag_ids?: string[];
  pinned?: boolean;
  source_url?: string;
  attachment_ids?: string[];
}

export interface NoteQuery {
  collection_id?: string;
  tag_id?: string;
  type?: NoteType;
  pinned?: boolean;
  archived?: boolean;
  created_after?: string;
  created_before?: string;
  limit?: number;
  offset?: number;
}

export interface SearchQuery extends Omit<NoteQuery, "tag_id"> {
  text: string;
  tag_ids?: string[];
}

export interface Collection {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface Tag {
  id: string;
  name: string;
  created_at: string;
}

export interface Attachment {
  id: string;
  file_name: string;
  media_type: string;
  size: number;
  checksum: string;
  created_at: string;
}

export interface Reminder {
  id: string;
  note_id: string;
  due_at: string;
  snoozed_until?: string | null;
  completed_at?: string | null;
}

export interface ReminderEvent {
  id: string;
  reminder_id: string;
  kind: "created" | "updated" | "snoozed" | "completed" | "notification_claimed";
  occurred_at: string;
  due_at: string;
}

export type SnoozeInput =
  | { until: string }
  | { preset: "two_hours" }
  | { preset: "tomorrow" | "next_monday"; timezone?: string }
  | { preset: "custom"; custom_at: string; timezone?: string };
