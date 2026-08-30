export function safeReturnTo(
  value: string | string[] | undefined,
  fallback = "/dashboard",
) {
  if (typeof value !== "string" || !value.startsWith("/") || value.startsWith("//")) {
    return fallback;
  }
  try {
    const parsed = new URL(value, "https://navego.local");
    if (parsed.origin !== "https://navego.local") return fallback;
    return `${parsed.pathname}${parsed.search}${parsed.hash}`;
  } catch {
    return fallback;
  }
}
