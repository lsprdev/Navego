import {
  apiErrorResponse,
  authenticatedPocketBase,
  mapUser,
} from "@/lib/navego-server";

export async function PATCH(request: Request) {
  try {
    const body = (await request.json()) as { name?: string };
    const name = body.name?.trim() ?? "";
    if (name.length < 2 || name.length > 80) {
      return Response.json(
        { error: "O nome deve ter entre 2 e 80 caracteres." },
        { status: 400 },
      );
    }

    const pocketBase = await authenticatedPocketBase();
    const auth = await pocketBase.collection("users").authRefresh();
    const record = await pocketBase
      .collection("users")
      .update(auth.record.id, { name });
    return Response.json({ user: mapUser(record) });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
