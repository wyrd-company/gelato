import { decode, encode } from "@msgpack/msgpack";

export const mediaTypeMessagePack = "application/vnd.msgpack" as const;
export const headerContentType = "Content-Type" as const;

export const actionRepoBranch = "repo.branch" as const;
export const actionRepoCreate = "repo.create" as const;
export const actionRepoRename = "repo.rename" as const;
export const actionRepoTag = "repo.tag" as const;

export const eventRepositoryAdded = "repository.added" as const;
export const eventBranchCreated = "branch.created" as const;
export const eventRepositoryRenamed = "repository.renamed" as const;
export const eventRepositoryTagged = "repository.tagged" as const;
export const eventRepositoryRemotePushed = "repository.remote.pushed" as const;
export const eventRepositoryRemotePulled = "repository.remote.pulled" as const;

export type AdminAction =
  | typeof actionRepoBranch
  | typeof actionRepoCreate
  | typeof actionRepoRename
  | typeof actionRepoTag;

export type RepositoryEventType =
  | typeof eventRepositoryAdded
  | typeof eventBranchCreated
  | typeof eventRepositoryRenamed
  | typeof eventRepositoryTagged
  | typeof eventRepositoryRemotePushed
  | typeof eventRepositoryRemotePulled;

export interface SshSignature {
  format: string;
  blob: Uint8Array;
  rest?: Uint8Array;
}

export interface AdminEnvelope {
  payload: Uint8Array;
  signature: SshSignature;
  certificate: string;
}

export interface AdminRequestBase {
  requestId?: string;
  action: AdminAction;
  target?: string;
  timestamp: string;
}

export interface RepoCreateRequest extends AdminRequestBase {
  action: typeof actionRepoCreate;
  repository: string;
  projectName?: string;
  description?: string;
  private?: boolean;
  hidden?: boolean;
  remote?: string;
  mirror?: boolean;
  lfs?: boolean;
  lfsEndpoint?: string;
}

export interface RepoRenameRequest extends AdminRequestBase {
  action: typeof actionRepoRename;
  repository: string;
  newName: string;
}

export interface RepoBranchRequest extends AdminRequestBase {
  action: typeof actionRepoBranch;
  repository: string;
  operation: "list" | "default" | "create" | "delete" | "remove" | "rm";
  branch?: string;
  fromRef?: string;
}

export interface RepoTagRequest extends AdminRequestBase {
  action: typeof actionRepoTag;
  repository: string;
  operation: "list" | "create" | "delete" | "remove" | "rm";
  tag?: string;
  ref?: string;
  message?: string;
}

export type AdminRequest =
  | RepoCreateRequest
  | RepoRenameRequest
  | RepoBranchRequest
  | RepoTagRequest;

export interface AdminResponse<T = unknown> {
  requestId?: string;
  ok: boolean;
  error?: string;
  result?: T;
}

export interface RepositoryEvent {
  id: string;
  type: RepositoryEventType;
  repository: string;
  projectName?: string;
  remote?: string;
  mirror?: boolean;
  oldName?: string;
  newName?: string;
  ref?: string;
  branch?: string;
  tag?: string;
  oldSha?: string;
  newSha?: string;
  actor?: string;
  timestamp: string;
}

export function encodeAdminEnvelope(envelope: AdminEnvelope): Uint8Array {
  return encode(envelope);
}

export function decodeAdminEnvelope(data: Uint8Array): AdminEnvelope {
  return decode(data) as AdminEnvelope;
}

export function encodeAdminResponse(response: AdminResponse): Uint8Array {
  return encode(response);
}

export function decodeAdminResponse<T = unknown>(data: Uint8Array): AdminResponse<T> {
  return decode(data) as AdminResponse<T>;
}

export function encodeRepositoryEvent(event: RepositoryEvent): Uint8Array {
  return encode(event);
}

export function decodeRepositoryEvent(data: Uint8Array): RepositoryEvent {
  return decode(data) as RepositoryEvent;
}

export function adminSubject(prefix: string, action: AdminAction): string {
  return joinSubject(prefix, "admin", action);
}

export function adminWildcardSubject(prefix: string): string {
  return joinSubject(prefix, "admin", "repo", "*");
}

export function eventSubjects(prefix: string, eventType: RepositoryEventType, repository: string): string[] {
  const repoPart = repoSubjectTokens(repository).join(".") || "_";
  return [
    joinSubject(prefix, "events", "repo", repoPart, eventType),
    joinSubject(prefix, "events", "type", eventType, repoPart),
  ];
}

export function repoSubjectTokens(repository: string): string[] {
  return repository
    .replace(/^\/+|\/+$/g, "")
    .split("/")
    .filter((part) => part.length > 0)
    .map(encodeSubjectToken);
}

function joinSubject(...parts: string[]): string {
  return parts
    .map((part) => part.replace(/^\.+|\.+$/g, ""))
    .filter((part) => part.length > 0)
    .join(".");
}

function encodeSubjectToken(token: string): string {
  const bytes = new TextEncoder().encode(token);
  let out = "";
  for (const byte of bytes) {
    const isDigit = byte >= 0x30 && byte <= 0x39;
    const isUpper = byte >= 0x41 && byte <= 0x5a;
    const isLower = byte >= 0x61 && byte <= 0x7a;
    const isSafe = isDigit || isUpper || isLower || byte === 0x2d || byte === 0x5f;
    out += isSafe ? String.fromCharCode(byte) : `%${byte.toString(16).toUpperCase().padStart(2, "0")}`;
  }
  return out || "_";
}
