export type FieldError = { field: string; rule: string; message: string };
export type ApiError = { code: string; message: string; details?: FieldError[] };
export type ApiMeta = { request_id: string; page?: number; per_page?: number; total?: number; last_page?: number };
export type ApiEnvelope<T> = { data?: T; meta?: ApiMeta; error?: ApiError };

export class ApiException extends Error {
  constructor(public status: number, public apiError: ApiError, public requestId?: string) {
    const details = apiError.details?.map((detail) => `${detail.field}: ${detail.message}`).join(", ");
    super(details ? `${apiError.message} (${details})` : apiError.message);
  }
}

async function parseEnvelope<T>(response: Response): Promise<ApiEnvelope<T>> {
  const raw = await response.text();
  let payload: unknown;
  try {
    payload = JSON.parse(raw);
  } catch {
    const requestId = response.headers.get("x-request-id") ?? undefined;
    const routeMissing = response.status === 404;
    throw new ApiException(
      response.status,
      {
        code: routeMissing ? "API_ROUTE_NOT_FOUND" : "INVALID_API_RESPONSE",
        message: routeMissing
          ? "Endpoint API tidak ditemukan. Pastikan backend sudah diperbarui ke versi yang sama dengan frontend."
          : "Server mengembalikan respons yang tidak valid.",
      },
      requestId,
    );
  }
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new ApiException(response.status, { code: "INVALID_API_RESPONSE", message: "Format respons API tidak valid." });
  }
  return payload as ApiEnvelope<T>;
}

export async function apiClient<T>(path: string, init?: RequestInit): Promise<ApiEnvelope<T>> {
  const response = await fetch(`/api/backend${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
    cache: "no-store",
  });
  const payload = await parseEnvelope<T>(response);
  if (!response.ok || payload.error) {
    throw new ApiException(response.status, payload.error ?? { code: "UNKNOWN_ERROR", message: "Terjadi kesalahan" }, payload.meta?.request_id);
  }
  return payload;
}

export async function publicApiClient<T>(path: string, init?: RequestInit): Promise<ApiEnvelope<T>> {
  const response = await fetch(`/api/public${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
    cache: "no-store",
  });
  const payload = await parseEnvelope<T>(response);
  if (!response.ok || payload.error) {
    throw new ApiException(response.status, payload.error ?? { code: "UNKNOWN_ERROR", message: "Terjadi kesalahan" }, payload.meta?.request_id);
  }
  return payload;
}
