import type { ControlBrowser } from "@/components/dashboard/types";
import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";

export async function POST(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const pocketBase = await authenticatedPocketBase();
    const browser = await pocketBase.send<ControlBrowser>(
      `/api/navego/browsers/${encodeURIComponent(id)}/default`,
      { method: "POST", requestKey: null },
    );
    return Response.json(browser);
  } catch (error) {
    return apiErrorResponse(error);
  }
}
