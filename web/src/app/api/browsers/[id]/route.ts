import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";
import type { ControlBrowser } from "@/components/dashboard/types";

export async function PATCH(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const body = (await request.json()) as { name?: string };
    const pocketBase = await authenticatedPocketBase();
    const browser = await pocketBase.send<ControlBrowser>(
      `/api/navego/browsers/${encodeURIComponent(id)}`,
      {
        method: "PATCH",
        body: { name: body.name },
        requestKey: null,
      },
    );
    return Response.json(browser);
  } catch (error) {
    return apiErrorResponse(error);
  }
}

export async function DELETE(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const pocketBase = await authenticatedPocketBase();
    const result = await pocketBase.send<{ status: string }>(
      `/api/navego/browsers/${encodeURIComponent(id)}`,
      { method: "DELETE", requestKey: null },
    );
    return Response.json(result, { status: 202 });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
