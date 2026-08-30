import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";

type ViewerTicket = {
  url: string;
  session_url: string;
};

export async function POST(
	request: Request,
	context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const pocketBase = await authenticatedPocketBase();
    const ticket = await pocketBase.send<ViewerTicket>(
      `/api/navego/browsers/${encodeURIComponent(id)}/viewer-ticket`,
      { method: "POST", requestKey: null },
    );
		return Response.json(rewriteLocalViewerHost(ticket, request), {
      status: 201,
      headers: { "cache-control": "private, no-store" },
    });
  } catch (error) {
    return apiErrorResponse(error);
	}
}

function rewriteLocalViewerHost(
	ticket: ViewerTicket,
	request: Request,
): ViewerTicket {
	const dashboardHostname = requestHostname(request);
	return {
		url: rewriteLoopbackHostname(ticket.url, dashboardHostname),
		session_url: rewriteLoopbackHostname(
			ticket.session_url,
			dashboardHostname,
		),
	};
}

function requestHostname(request: Request): string {
	const forwardedHost = request.headers
		.get("x-forwarded-host")
		?.split(",", 1)[0]
		.trim();
	const host = forwardedHost || request.headers.get("host");
	if (host) {
		try {
			return new URL(`http://${host}`).hostname;
		} catch {
			// Fall back to the normalized request URL below.
		}
	}
	return new URL(request.url).hostname;
}

function rewriteLoopbackHostname(value: string, dashboardHostname: string): string {
	const loopbackHosts = new Set(["127.0.0.1", "localhost", "::1"]);
	const viewerURL = new URL(value);
	if (
		loopbackHosts.has(viewerURL.hostname) &&
		loopbackHosts.has(dashboardHostname)
	) {
		viewerURL.hostname = dashboardHostname;
	}
	return viewerURL.toString();
}
