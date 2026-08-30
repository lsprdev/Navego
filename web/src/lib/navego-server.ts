import "server-only";

import { cookies } from "next/headers";
import PocketBase, { ClientResponseError, type RecordModel } from "pocketbase";

import type {
  ControlBrowser,
  SessionUser,
} from "@/components/dashboard/types";

export const SESSION_COOKIE = "navego_session";

const controlURL =
  process.env.NAVEGO_CONTROL_URL?.trim() || "http://127.0.0.1:8090";

export function newPocketBase(token?: string) {
  const pocketBase = new PocketBase(controlURL);
  pocketBase.autoCancellation(false);
  if (token) pocketBase.authStore.save(token);
  return pocketBase;
}

export function mapUser(record: RecordModel): SessionUser {
  return {
    id: record.id,
    name: String(record.name || record.email || "Usuário"),
    email: String(record.email || ""),
  };
}

export async function setSessionCookie(token: string) {
  const store = await cookies();
  store.set(SESSION_COOKIE, token, {
    httpOnly: true,
    secure: process.env.NODE_ENV === "production",
    sameSite: "strict",
    path: "/",
    maxAge: 60 * 60 * 24 * 14,
    priority: "high",
  });
}

export async function clearSessionCookie() {
  (await cookies()).delete(SESSION_COOKIE);
}

export async function authenticatedPocketBase() {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  if (!token) throw new UnauthorizedError();
  return newPocketBase(token);
}

export async function authenticatedControlFetch(
  path: string,
  init?: RequestInit,
) {
  const pocketBase = await authenticatedPocketBase();
  return fetch(`${controlURL}${path}`, {
    ...init,
    cache: "no-store",
    headers: {
      Authorization: pocketBase.authStore.token,
      ...init?.headers,
    },
  });
}

export async function getSessionUser(): Promise<SessionUser | null> {
  try {
    const pocketBase = await authenticatedPocketBase();
    const auth = await pocketBase.collection("users").authRefresh();
    return mapUser(auth.record);
  } catch {
    return null;
  }
}

export async function getBrowsers(): Promise<ControlBrowser[]> {
  const pocketBase = await authenticatedPocketBase();
  return pocketBase.send<ControlBrowser[]>("/api/navego/browsers", {
    method: "GET",
    requestKey: null,
  });
}

export class UnauthorizedError extends Error {
  constructor() {
    super("Sessão ausente ou expirada.");
    this.name = "UnauthorizedError";
  }
}

export function apiErrorResponse(error: unknown) {
  if (error instanceof UnauthorizedError) {
    return Response.json({ error: error.message }, { status: 401 });
  }
  if (error instanceof ClientResponseError) {
    const message =
      typeof error.response?.message === "string" && error.response.message
        ? error.response.message
        : "Não foi possível concluir a operação.";
    return Response.json(
      { error: message },
      { status: error.status >= 400 ? error.status : 502 },
    );
  }
  return Response.json(
    { error: "O control plane está indisponível." },
    { status: 502 },
  );
}
