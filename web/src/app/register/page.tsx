import { redirect } from "next/navigation";

import { AuthForm } from "@/components/auth/auth-form";
import { getSessionUser } from "@/lib/navego-server";

export default async function RegisterPage() {
  if (await getSessionUser()) redirect("/dashboard");
  return <AuthForm mode="register" />;
}
