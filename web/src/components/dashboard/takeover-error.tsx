"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowLeftIcon,
  LoaderCircleIcon,
  ShieldAlertIcon,
  UserRoundCogIcon,
} from "lucide-react";
import { useState } from "react";

import { NavegoLogo } from "@/components/brand/navego-logo";
import { Navy } from "@/components/brand/navy";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { TakeoverAccessFailure } from "@/lib/navego-server";

type TakeoverErrorProps = {
  kind: TakeoverAccessFailure;
  message: string;
  returnTo: string;
};

const headings: Record<TakeoverAccessFailure, string> = {
  account_mismatch: "Este acesso pertence a outra conta",
  expired: "O link de acesso expirou",
  browser_unavailable: "O Chromium não está disponível",
  control_unavailable: "O Navego está temporariamente indisponível",
};

export function TakeoverError({ kind, message, returnTo }: TakeoverErrorProps) {
  const router = useRouter();
  const [switching, setSwitching] = useState(false);
  const accountMismatch = kind === "account_mismatch";

  async function switchAccount() {
    setSwitching(true);
    try {
      await fetch("/api/auth/logout", { method: "POST" });
      router.replace(`/login?returnTo=${encodeURIComponent(returnTo)}`);
      router.refresh();
    } finally {
      setSwitching(false);
    }
  }

  return (
    <main className="relative flex min-h-svh items-center justify-center overflow-hidden bg-background px-5 py-16">
      <div className="landing-grid absolute inset-0 opacity-40" />
      <div className="auth-glow pointer-events-none absolute left-1/2 top-1/2 size-[520px] -translate-x-1/2 -translate-y-1/2 rounded-full" />

      <Card className="relative w-full max-w-xl shadow-2xl">
        <CardHeader className="items-start">
          <div className="mb-5 flex w-full items-center justify-between gap-4">
            <NavegoLogo />
            <Badge variant="outline">Acesso protegido</Badge>
          </div>
          <div className="mb-3 flex size-12 items-center justify-center rounded-xl border bg-muted text-muted-foreground">
            {accountMismatch ? (
              <UserRoundCogIcon className="size-6" />
            ) : (
              <ShieldAlertIcon className="size-6" />
            )}
          </div>
          <CardTitle className="text-2xl tracking-[-0.035em]">
            {headings[kind]}
          </CardTitle>
          <CardDescription className="max-w-md leading-6">
            {message}
          </CardDescription>
        </CardHeader>

        <CardContent>
          <div className="flex items-center gap-4 rounded-xl border bg-muted/35 p-4">
            <Navy className="w-20 shrink-0" />
            <p className="text-sm leading-6 text-muted-foreground">
              {accountMismatch
                ? "Saia desta conta e entre com o mesmo email usado ao autorizar o Navego no ChatGPT."
                : "Volte ao ChatGPT e solicite um novo acesso ao Chromium para continuar exatamente da mesma página."}
            </p>
          </div>
        </CardContent>

        <CardFooter className="justify-between gap-3">
          <Button variant="ghost" render={<Link href="/dashboard" />}>
            <ArrowLeftIcon data-icon="inline-start" />
            Ir ao dashboard
          </Button>
          {accountMismatch ? (
            <Button onClick={switchAccount} disabled={switching}>
              {switching ? (
                <LoaderCircleIcon className="animate-spin" data-icon="inline-start" />
              ) : (
                <UserRoundCogIcon data-icon="inline-start" />
              )}
              Trocar de conta
            </Button>
          ) : null}
        </CardFooter>
      </Card>
    </main>
  );
}
