import { NextRequest } from "next/server";
import { apiBaseURL } from "@/lib/api-base";
import type { ApiEnvelope } from "@/lib/api";
import { apiEnvelopeResponse, apiErrorResponse, apiUnavailableResponse, decodeUpstreamJSON } from "@/lib/api-proxy";

const base = apiBaseURL();

type LoginData = { access_token: string; refresh_token: string; expires_at: string };

export async function POST(request: NextRequest) {
  const body = await request.text();
  let upstream: Response;
  try {
    upstream = await fetch(`${base}/auth/login`, {
      method: "POST",
      headers: { Accept: "application/json", "Content-Type": "application/json", "User-Agent": request.headers.get("user-agent") ?? "" },
      body,
      cache: "no-store",
      redirect: "manual",
    });
  } catch {
    return apiUnavailableResponse("Layanan autentikasi sedang tidak tersedia");
  }

  const decoded = await decodeUpstreamJSON<ApiEnvelope<LoginData>>(upstream);
  if (!decoded.valid) return decoded.response;
  const payload = decoded.payload;
  if (upstream.ok && (!payload.data?.access_token || !payload.data.refresh_token || !payload.data.expires_at)) {
    return apiErrorResponse(502, "UPSTREAM_INVALID_RESPONSE", "Respons login backend tidak lengkap.");
  }

  const response = apiEnvelopeResponse(payload, upstream.status, decoded.requestId);
  if (upstream.ok && payload.data) {
    response.cookies.set("access_token", payload.data.access_token, {
      httpOnly: true,
      sameSite: "lax",
      secure: process.env.NODE_ENV === "production",
      path: "/",
      expires: new Date(payload.data.expires_at),
    });
    response.cookies.set("refresh_token", payload.data.refresh_token, {
      httpOnly: true,
      sameSite: "strict",
      secure: process.env.NODE_ENV === "production",
      path: "/",
      maxAge: 60 * 60 * 24 * 7,
    });
  }
  return response;
}
