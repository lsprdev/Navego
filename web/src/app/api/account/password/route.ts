import { ClientResponseError } from "pocketbase";

import {
  apiErrorResponse,
  authenticatedPocketBase,
  clearSessionCookie,
} from "@/lib/navego-server";

export async function PATCH(request: Request) {
  try {
    const body = (await request.json()) as {
      currentPassword?: string;
      newPassword?: string;
      confirmPassword?: string;
    };
    const currentPassword = body.currentPassword ?? "";
    const newPassword = body.newPassword ?? "";
    const confirmPassword = body.confirmPassword ?? "";

    if (!currentPassword) {
      return Response.json(
        { error: "Informe sua senha atual." },
        { status: 400 },
      );
    }
    if (newPassword.length < 12) {
      return Response.json(
        { error: "A nova senha deve ter pelo menos 12 caracteres." },
        { status: 400 },
      );
    }
    if (newPassword !== confirmPassword) {
      return Response.json(
        { error: "A confirmação não corresponde à nova senha." },
        { status: 400 },
      );
    }
    if (newPassword === currentPassword) {
      return Response.json(
        { error: "Escolha uma senha diferente da atual." },
        { status: 400 },
      );
    }

    const pocketBase = await authenticatedPocketBase();
    const auth = await pocketBase.collection("users").authRefresh();
    await pocketBase.collection("users").update(auth.record.id, {
      oldPassword: currentPassword,
      password: newPassword,
      passwordConfirm: confirmPassword,
    });
    await clearSessionCookie();
    return Response.json({ status: "ok" });
  } catch (error) {
    if (error instanceof ClientResponseError && error.status === 400) {
      return Response.json(
        { error: "A senha atual está incorreta." },
        { status: 400 },
      );
    }
    return apiErrorResponse(error);
  }
}
