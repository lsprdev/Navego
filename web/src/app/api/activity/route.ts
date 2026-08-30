import type { ActivityEvent } from "@/components/dashboard/types";
import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";

export async function GET() {
  try {
    const pocketBase = await authenticatedPocketBase();
    const events = await pocketBase.send<ActivityEvent[]>(
      "/api/navego/activity",
      { method: "GET", requestKey: null },
    );
    return Response.json(events);
  } catch (error) {
    return apiErrorResponse(error);
  }
}
