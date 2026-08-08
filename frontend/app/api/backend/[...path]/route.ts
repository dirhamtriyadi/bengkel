import { cookies } from "next/headers";
import { NextRequest } from "next/server";
import { apiBaseURL } from "@/lib/api-base";
import { apiErrorResponse, apiUnavailableResponse, decodeUpstreamJSON, forwardUpstreamJSON } from "@/lib/api-proxy";

const base = apiBaseURL();

type RefreshEnvelope = {
  data?: { access_token?: string; refresh_token?: string; expires_at?: string };
};

async function proxy(request: NextRequest, parts: string[]) {
  const jar = await cookies();
  let access = jar.get("access_token")?.value;
  const refreshToken = jar.get("refresh_token")?.value;
  const rawBody = ["GET", "HEAD"].includes(request.method) ? undefined : await request.arrayBuffer();
  const path = parts.map(encodeURIComponent).join("/");
  const call = () => fetch(`${base}/${path}${request.nextUrl.search}`, {
    method: request.method,
    headers: {
      Accept: "application/json",
      "Content-Type": request.headers.get("content-type") ?? "application/json",
      Authorization: access ? `Bearer ${access}` : "",
      "X-Branch-ID": request.headers.get("x-branch-id") ?? "",
      "User-Agent": request.headers.get("user-agent") ?? "",
    },
    body: rawBody,
    cache: "no-store",
    redirect: "manual",
  });

  try {
    let upstream = await call();
    if (upstream.status === 401 && refreshToken) {
      const refreshed = await fetch(`${base}/auth/refresh`, {
        method: "POST",
        headers: { Accept: "application/json", "Content-Type": "application/json" },
        body: JSON.stringify({ refresh_token: refreshToken }),
        cache: "no-store",
        redirect: "manual",
      });
      if (refreshed.ok) {
        const decoded = await decodeUpstreamJSON<RefreshEnvelope>(refreshed);
        if (!decoded.valid) return decoded.response;
        const tokens = decoded.payload.data;
        if (!tokens?.access_token || !tokens.refresh_token || !tokens.expires_at) {
          return apiErrorResponse(502, "UPSTREAM_INVALID_RESPONSE", "Respons refresh token backend tidak lengkap.");
        }
        access = tokens.access_token;
        jar.set("access_token", access, {
          httpOnly: true,
          sameSite: "lax",
          secure: process.env.NODE_ENV === "production",
          path: "/",
          expires: new Date(tokens.expires_at),
        });
        jar.set("refresh_token", tokens.refresh_token, {
          httpOnly: true,
          sameSite: "strict",
          secure: process.env.NODE_ENV === "production",
          path: "/",
          maxAge: 60 * 60 * 24 * 7,
        });
        upstream = await call();
      }
    }
    return await forwardUpstreamJSON(upstream);
  } catch {
    return apiUnavailableResponse();
  }
}

type Context = { params: Promise<{ path: string[] }> };
export async function GET(request: NextRequest, context: Context) { return proxy(request, (await context.params).path); }
export async function POST(request: NextRequest, context: Context) { return proxy(request, (await context.params).path); }
export async function PUT(request: NextRequest, context: Context) { return proxy(request, (await context.params).path); }
export async function PATCH(request: NextRequest, context: Context) { return proxy(request, (await context.params).path); }
export async function DELETE(request: NextRequest, context: Context) { return proxy(request, (await context.params).path); }
