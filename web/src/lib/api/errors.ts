export interface APIErrorBody {
  error?: {
    code?: string;
    message?: string;
    request_id?: string;
  };
}

export type APIErrorKind =
  | "authentication"
  | "vault_locked"
  | "network"
  | "permission"
  | "not_found"
  | "backend_unavailable"
  | "request"
  | "unknown";

export class LocalAPIError extends Error {
  readonly kind: APIErrorKind;
  readonly status: number;
  readonly code: string;
  readonly requestId?: string;

  constructor(input: {
    message: string;
    kind: APIErrorKind;
    status?: number;
    code?: string;
    requestId?: string;
  }) {
    super(input.message);
    this.name = "LocalAPIError";
    this.kind = input.kind;
    this.status = input.status ?? 0;
    this.code = input.code ?? "unknown";
    this.requestId = input.requestId;
  }
}

export function apiErrorFromResponse(status: number, body: APIErrorBody): LocalAPIError {
  const code = body.error?.code ?? "http_error";
  const kind: APIErrorKind =
    status === 401 || code === "unauthorized"
      ? "authentication"
      : status === 423 || code === "vault_locked"
        ? "vault_locked"
        : status === 403
          ? "permission"
          : status === 404
            ? "not_found"
            : status >= 500 || code === "api_not_configured" || code === "local_api_unavailable"
              ? "backend_unavailable"
              : status >= 400
                ? "request"
                : "unknown";
  return new LocalAPIError({
    kind,
    status,
    code,
    requestId: body.error?.request_id,
    message: body.error?.message ?? "The local API request failed.",
  });
}

export function describeAPIError(error: unknown): { title: string; message: string } {
  if (!(error instanceof LocalAPIError)) {
    return { title: "Something went wrong", message: "Please try again." };
  }
  switch (error.kind) {
    case "authentication":
      return { title: "Local API authentication failed", message: "Reconnect the dashboard using the trusted local launcher." };
    case "vault_locked":
      return { title: "Vault is locked", message: "Unlock the vault before viewing or changing captured data." };
    case "network":
      return { title: "Local API is unavailable", message: "Check that the local application is running, then retry." };
    case "permission":
      return { title: "Permission denied", message: "This local API token does not have permission for that action." };
    case "not_found":
      return { title: "Not found", message: error.message };
    case "backend_unavailable":
      return { title: "Backend capability unavailable", message: error.message };
    case "request":
      return { title: "Request could not be completed", message: error.message };
    default:
      return { title: "Something went wrong", message: "Please try again." };
  }
}
