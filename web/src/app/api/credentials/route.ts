import type {
  SavedCredential,
  SavedCredentialInput,
} from "@/components/dashboard/types";
import {
  apiErrorResponse,
  authenticatedPocketBase,
} from "@/lib/navego-server";

export async function GET() {
  try {
    const pocketBase = await authenticatedPocketBase();
    const credentials = await pocketBase.send<SavedCredential[]>(
      "/api/navego/credentials",
      { method: "GET", requestKey: null },
    );
    return Response.json(credentials);
  } catch (error) {
    return apiErrorResponse(error);
  }
}

export async function POST(request: Request) {
  try {
    const input = (await request.json()) as SavedCredentialInput;
    const pocketBase = await authenticatedPocketBase();
    const credential = await pocketBase.send<SavedCredential>(
      "/api/navego/credentials",
      { method: "POST", body: input, requestKey: null },
    );
    return Response.json(credential, { status: 201 });
  } catch (error) {
    return apiErrorResponse(error);
  }
}
