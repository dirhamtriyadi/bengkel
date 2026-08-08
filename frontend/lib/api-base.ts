import "server-only";

const API_PATH = "/api/v1";

export function apiBaseURL(): string {
  const configured = process.env.API_ORIGIN ?? "http://localhost:8080";
  let parsed: URL;

  try {
    parsed = new URL(configured);
  } catch {
    throw new Error("API_ORIGIN harus berupa origin http(s) yang valid");
  }

  if (
    !["http:", "https:"].includes(parsed.protocol)
    || parsed.pathname !== "/"
    || parsed.search
    || parsed.hash
    || parsed.username
    || parsed.password
  ) {
    throw new Error("API_ORIGIN hanya boleh berisi skema, host, dan port tanpa path, query, fragment, atau kredensial");
  }

  return `${parsed.origin}${API_PATH}`;
}
