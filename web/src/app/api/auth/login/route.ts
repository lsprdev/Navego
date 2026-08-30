import {
  apiErrorResponse,
  mapUser,
  newPocketBase,
  setSessionCookie,
} from "@/lib/navego-server";

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as { email?: string; password?: string };
    const email = body.email?.trim().toLowerCase() ?? "";
    const password = body.password ?? "";
    if (!email || !password) {
      return Response.json(
        { error: "Informe seu email e sua senha." },
        { status: 400 },
      );
    }

    const pocketBase = newPocketBase();
    const auth = await pocketBase
      .collection("users")
      .authWithPassword(email, password);
    await setSessionCookie(auth.token);
    return Response.json({ user: mapUser(auth.record) });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
