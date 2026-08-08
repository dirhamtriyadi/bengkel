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

export async function apiClient<T>(path: string, init?: RequestInit): Promise<ApiEnvelope<T>> {
  const response = await fetch(`/api/backend${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
    cache: "no-store",
  });
  const payload = (await response.json()) as ApiEnvelope<T>;
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
  const payload = (await response.json()) as ApiEnvelope<T>;
  if (!response.ok || payload.error) {
    throw new ApiException(response.status, payload.error ?? { code: "UNKNOWN_ERROR", message: "Terjadi kesalahan" }, payload.meta?.request_id);
  }
  return payload;
}
