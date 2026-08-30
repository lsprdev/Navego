import { redirect } from "next/navigation";

import { AuthForm } from "@/components/auth/auth-form";
import { getSessionUser } from "@/lib/navego-server";
import { safeReturnTo } from "@/lib/safe-return-to";

export default async function LoginPage({
  searchParams,
}: {
  searchParams: Promise<{ returnTo?: string | string[] }>;
}) {
  const returnTo = safeReturnTo((await searchParams).returnTo);
  if (await getSessionUser()) redirect(returnTo);
  return <AuthForm mode="login" returnTo={returnTo} />;
}
