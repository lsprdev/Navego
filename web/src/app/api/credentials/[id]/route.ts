import type {
  SavedCredential,
  SavedCredentialInput,
} from "@/components/dashboard/types";
import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";

type RouteContext = { params: Promise<{ id: string }> };

export async function PATCH(request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const input = (await request.json()) as SavedCredentialInput;
    const pocketBase = await authenticatedPocketBase();
    const credential = await pocketBase.send<SavedCredential>(
      `/api/navego/credentials/${encodeURIComponent(id)}`,
      { method: "PATCH", body: input, requestKey: null },
    );
    return Response.json(credential);
  } catch (error) {
    return apiErrorResponse(error);
  }
}

export async function DELETE(_request: Request, context: RouteContext) {
  try {
    const { id } = await context.params;
    const pocketBase = await authenticatedPocketBase();
    await pocketBase.send(
      `/api/navego/credentials/${encodeURIComponent(id)}`,
      { method: "DELETE", requestKey: null },
    );
    return Response.json({ status: "deleted" });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
