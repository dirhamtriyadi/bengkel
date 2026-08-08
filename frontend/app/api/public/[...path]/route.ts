import { NextRequest, NextResponse } from "next/server";
import { apiBaseURL } from "@/lib/api-base";

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
    return new NextResponse(await upstream.arrayBuffer(), {
      status: upstream.status,
      headers: {
        "Cache-Control": "no-store",
        "Content-Type": upstream.headers.get("content-type") ?? "application/json",
        "Referrer-Policy": "no-referrer",
        "X-Request-ID": upstream.headers.get("x-request-id") ?? "",
        "X-Robots-Tag": "noindex, nofollow, noarchive",
      },
    });
  } catch {
    const requestId = crypto.randomUUID();
    return NextResponse.json(
      { meta: { request_id: requestId }, error: { code: "API_UNAVAILABLE", message: "Layanan invoice sedang tidak tersedia" } },
      { status: 503, headers: { "Cache-Control": "no-store", "Referrer-Policy": "no-referrer", "X-Request-ID": requestId } },
    );
  }
}

type Context = { params: Promise<{ path: string[] }> };

export async function GET(request: NextRequest, context: Context) {
  return proxy(request, (await context.params).path);
}

export async function POST(request: NextRequest, context: Context) {
  return proxy(request, (await context.params).path);
}
