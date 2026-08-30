import { authenticatedControlFetch, UnauthorizedError } from "@/lib/navego-server";

export async function GET(
  _request: Request,
  context: { params: Promise<{ id: string }> },
) {
  try {
    const { id } = await context.params;
    const response = await authenticatedControlFetch(
      `/api/navego/browsers/${encodeURIComponent(id)}/preview`,
    );
    if (!response.ok) {
      const body = await response.text();
      return new Response(body, {
        status: response.status,
        headers: { "content-type": response.headers.get("content-type") ?? "application/json" },
      });
    }
    return new Response(response.body, {
      status: 200,
      headers: {
        "cache-control": "private, no-store",
        "content-type": response.headers.get("content-type") ?? "image/png",
        "x-content-type-options": "nosniff",
      },
    });
  } catch (error) {
    if (error instanceof UnauthorizedError) {
      return Response.json({ error: error.message }, { status: 401 });
    }
    return Response.json(
      { error: "Não foi possível carregar a prévia." },
      { status: 502 },
    );
  }
}
