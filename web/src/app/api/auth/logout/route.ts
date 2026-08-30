import { clearSessionCookie } from "@/lib/navego-server";

export async function POST() {
  await clearSessionCookie();
  return Response.json({ status: "ok" });
}
