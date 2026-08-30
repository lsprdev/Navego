import type { Metadata } from "next";
import { redirect } from "next/navigation";

import { TakeoverError } from "@/components/dashboard/takeover-error";
import {
  getSessionUser,
  resolveTakeoverBrowser,
  TakeoverAccessError,
} from "@/lib/navego-server";

export const metadata: Metadata = {
  title: "Acesso ao Chromium · Navego",
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

export default async function TakeoverPage({
  params,
}: {
  params: Promise<{ ticket: string }>;
}) {
  const { ticket } = await params;
  const returnTo = `/takeover/${encodeURIComponent(ticket)}`;

  if (!(await getSessionUser())) {
    redirect(`/login?returnTo=${encodeURIComponent(returnTo)}`);
  }

  let browserID = "";
  try {
    browserID = (await resolveTakeoverBrowser(ticket)).id;
  } catch (error) {
    const failure =
      error instanceof TakeoverAccessError
        ? error
        : new TakeoverAccessError(
            "control_unavailable",
            "O control plane não respondeu. Tente novamente em instantes.",
          );
    return (
      <TakeoverError
        kind={failure.kind}
        message={failure.message}
        returnTo={returnTo}
      />
    );
  }

  redirect(`/dashboard?viewer=${encodeURIComponent(browserID)}`);
}
