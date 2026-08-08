import "server-only";

import { NextResponse } from "next/server";

type DecodedUpstream<T> =
  | { valid: true; payload: T; requestId: string }
  | { valid: false; response: NextResponse };

function responseHeaders(requestId: string, extraHeaders?: HeadersInit) {
  const headers = new Headers(extraHeaders);
  headers.set("Cache-Control", "no-store");
  headers.set("X-Request-ID", requestId);
  return headers;
}

export function apiErrorResponse(status: number, code: string, message: string, extraHeaders?: HeadersInit, requestId = crypto.randomUUID()) {
  return NextResponse.json(
    { meta: { request_id: requestId }, error: { code, message } },
    { status, headers: responseHeaders(requestId, extraHeaders) },
  );
}

export function apiUnavailableResponse(message = "Backend sedang tidak tersedia", extraHeaders?: HeadersInit) {
  return apiErrorResponse(503, "API_UNAVAILABLE", message, extraHeaders);
}

function dependencyHeaders(upstreamStatus: number, extraHeaders?: HeadersInit) {
  const headers = new Headers(extraHeaders);
  headers.set("X-Upstream-Status", String(upstreamStatus));
  return headers;
}

export async function decodeUpstreamJSON<T>(upstream: Response, extraHeaders?: HeadersInit): Promise<DecodedUpstream<T>> {
  const requestId = upstream.headers.get("x-request-id") || crypto.randomUUID();
  const raw = await upstream.text();
  let payload: unknown;

  try {
    payload = JSON.parse(raw);
  } catch {
    const routeMissing = upstream.status === 404;
    return {
      valid: false,
      response: apiErrorResponse(
        routeMissing ? 404 : 424,
        routeMissing ? "UPSTREAM_ROUTE_NOT_FOUND" : "UPSTREAM_INVALID_RESPONSE",
        routeMissing
          ? "Endpoint backend tidak ditemukan. Pastikan backend dan frontend menggunakan versi deployment yang sama."
          : "Backend mengembalikan respons yang tidak valid.",
        routeMissing ? extraHeaders : dependencyHeaders(upstream.status, extraHeaders),
        requestId,
      ),
    };
  }

  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    return {
      valid: false,
      response: apiErrorResponse(
        424,
        "UPSTREAM_INVALID_RESPONSE",
        "Format respons backend tidak valid.",
        dependencyHeaders(upstream.status, extraHeaders),
        requestId,
      ),
    };
  }
  return { valid: true, payload: payload as T, requestId };
}

export function apiEnvelopeResponse(payload: unknown, status: number, requestId: string, extraHeaders?: HeadersInit) {
  return NextResponse.json(payload, { status, headers: responseHeaders(requestId, extraHeaders) });
}

export async function forwardUpstreamJSON(upstream: Response, extraHeaders?: HeadersInit) {
  const decoded = await decodeUpstreamJSON<unknown>(upstream, extraHeaders);
  if (!decoded.valid) return decoded.response;
  if (upstream.status === 502) {
    // Cloudflare replaces an origin 502 body with its own HTML error page.
    // Preserve the backend status in a header and use Failed Dependency at
    // the browser-facing BFF boundary so the JSON API envelope stays intact.
    return apiEnvelopeResponse(decoded.payload, 424, decoded.requestId, dependencyHeaders(upstream.status, extraHeaders));
  }
  return apiEnvelopeResponse(decoded.payload, upstream.status, decoded.requestId, extraHeaders);
}
