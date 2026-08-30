import {
  apiErrorResponse,
  authenticatedPocketBase,
  getBrowsers,
} from "@/lib/navego-server";
import type { ControlBrowser } from "@/components/dashboard/types";

export async function GET() {
  try {
    return Response.json(await getBrowsers());
  } catch (error) {
    return apiErrorResponse(error);
  }
}

export async function POST(request: Request) {
  try {
    const body = (await request.json()) as { name?: string };
    const pocketBase = await authenticatedPocketBase();
    const browser = await pocketBase.send<ControlBrowser>(
      "/api/navego/browsers",
      {
        method: "POST",
        body: { name: body.name },
        requestKey: null,
      },
    );
    return Response.json(browser, { status: 202 });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
