import {
  apiErrorResponse,
  authenticatedPocketBase,
  clearSessionCookie,
  mapUser,
  setSessionCookie,
} from "@/lib/navego-server";

export async function GET() {
  try {
    const pocketBase = await authenticatedPocketBase();
    const auth = await pocketBase.collection("users").authRefresh();
    await setSessionCookie(auth.token);
    return Response.json({ user: mapUser(auth.record) });
  } catch (error) {
    await clearSessionCookie();
    return apiErrorResponse(error);
  }
}
