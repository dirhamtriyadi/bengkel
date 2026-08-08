import { NextRequest } from "next/server";
import { apiBaseURL } from "@/lib/api-base";
import { apiUnavailableResponse, forwardUpstreamJSON } from "@/lib/api-proxy";

const base = apiBaseURL();

async function proxy(request: NextRequest, parts: string[]) {
  const path = parts.map(encodeURIComponent).join("/");
  const body = ["GET", "HEAD"].includes(request.method) ? undefined : await request.arrayBuffer();
  try {
    const upstream = await fetch(`${base}/public/${path}${request.nextUrl.search}`, {
      method: request.method,
      headers: {
        Accept: "application/json",
        "Content-Type": request.headers.get("content-type") ?? "application/json",
        "User-Agent": request.headers.get("user-agent") ?? "",
      },
      body,
      cache: "no-store",
      redirect: "manual",
    });
    return await forwardUpstreamJSON(upstream, {
      "Referrer-Policy": "no-referrer",
      "X-Robots-Tag": "noindex, nofollow, noarchive",
    });
  } catch {
    return apiUnavailableResponse("Layanan invoice sedang tidak tersedia", {
      "Referrer-Policy": "no-referrer",
      "X-Robots-Tag": "noindex, nofollow, noarchive",
    });
  }
}

type Context = { params: Promise<{ path: string[] }> };

export async function GET(request: NextRequest, context: Context) {
  return proxy(request, (await context.params).path);
}

export async function POST(request: NextRequest, context: Context) {
  return proxy(request, (await context.params).path);
}
