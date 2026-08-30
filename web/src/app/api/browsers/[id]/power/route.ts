import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";
import type { ControlBrowser } from "@/components/dashboard/types";

export async function POST(
  request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const body = (await request.json()) as { running?: boolean };
    const pocketBase = await authenticatedPocketBase();
    const browser = await pocketBase.send<ControlBrowser>(
      `/api/navego/browsers/${encodeURIComponent(id)}/power`,
      {
        method: "POST",
        body: { running: Boolean(body.running) },
        requestKey: null,
      },
    );
    return Response.json(browser, { status: 202 });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
