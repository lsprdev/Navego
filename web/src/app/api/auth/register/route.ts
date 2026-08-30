import {
  apiErrorResponse,
  mapUser,
  newPocketBase,
  setSessionCookie,
} from "@/lib/navego-server";

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as {
      name?: string;
      email?: string;
      password?: string;
    };
    const name = body.name?.trim() ?? "";
    const email = body.email?.trim().toLowerCase() ?? "";
    const password = body.password ?? "";
    if (name.length < 2 || name.length > 80) {
      return Response.json(
        { error: "O nome deve ter entre 2 e 80 caracteres." },
        { status: 400 },
      );
    }
    if (!email.includes("@")) {
      return Response.json({ error: "Informe um email válido." }, { status: 400 });
    }
    if (password.length < 12) {
      return Response.json(
        { error: "A senha deve ter pelo menos 12 caracteres." },
        { status: 400 },
      );
    }

    const pocketBase = newPocketBase();
    await pocketBase.collection("users").create({
      name,
      email,
      password,
      passwordConfirm: password,
    });
    const auth = await pocketBase
      .collection("users")
      .authWithPassword(email, password);
    await setSessionCookie(auth.token);
    return Response.json({ user: mapUser(auth.record) }, { status: 201 });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
