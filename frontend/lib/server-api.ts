import "server-only";

import { apiBaseURL } from "@/lib/api-base";
import type { ApiEnvelope } from "@/lib/api";

export async function serverApi<T>(path: string): Promise<ApiEnvelope<T> | null> {
  try {
    const response = await fetch(`${apiBaseURL()}${path}`, { next: { revalidate: 60 } });
    if (!response.ok) return null;
    return await response.json();
  } catch {
    return null;
  }
}
