"use client";

import { FormEvent, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  ArrowLeftIcon,
  ArrowRightIcon,
  EyeIcon,
  EyeOffIcon,
  LoaderCircleIcon,
  LockKeyholeIcon,
  ShieldCheckIcon,
} from "lucide-react";

import { NavegoLogo } from "@/components/brand/navego-logo";
import { Navy } from "@/components/brand/navy";
import { Button } from "@/components/ui/button";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";

type AuthFormProps = {
  mode: "login" | "register";
  returnTo?: string;
};

export function AuthForm({ mode, returnTo = "/dashboard" }: AuthFormProps) {
  const router = useRouter();
  const [pending, setPending] = useState(false);
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const registering = mode === "register";

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setPending(true);
    setError("");
    const data = new FormData(event.currentTarget);
    try {
      const response = await fetch(`/api/auth/${mode}`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({
          name: data.get("name"),
          email: data.get("email"),
          password: data.get("password"),
        }),
      });
      const body = (await response.json()) as { error?: string };
      if (!response.ok) {
        throw new Error(body.error || "Não foi possível entrar no Navego.");
      }
      router.replace(returnTo);
      router.refresh();
    } catch (caught) {
      setError(
        caught instanceof Error
          ? caught.message
          : "Não foi possível entrar no Navego.",
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <main className="auth-page grid min-h-svh bg-background lg:grid-cols-[minmax(0,1.05fr)_minmax(440px,0.95fr)]">
      <section className="relative hidden overflow-hidden border-r lg:flex lg:flex-col lg:justify-between lg:p-10 xl:p-14">
        <div className="landing-grid absolute inset-0 opacity-60" />
        <div className="auth-glow pointer-events-none absolute left-1/2 top-1/2 size-[620px] -translate-x-1/2 -translate-y-1/2 rounded-full" />
        <NavegoLogo className="relative" />

        <div className="relative flex items-center justify-center py-10">
          <div className="auth-navy-stage landing-float relative flex size-[360px] items-center justify-center rounded-[38%] border bg-card/70 xl:size-[430px]">
            <div className="absolute inset-5 rounded-[34%] border border-dashed border-border/70" />
            <Navy className="relative w-[78%] drop-shadow-[0_28px_40px_rgba(0,0,0,.5)]" />
            <span className="absolute bottom-7 rounded-full border bg-background/80 px-3 py-1.5 font-mono text-[8px] uppercase tracking-[0.2em] text-muted-foreground backdrop-blur">
              Navy está olhando por você
            </span>
          </div>
        </div>

        <div className="relative max-w-xl">
          <p className="font-mono text-[10px] uppercase tracking-[0.24em] text-primary">
            Seu browser. Suas regras.
          </p>
          <h1 className="mt-3 text-4xl font-medium leading-[1.03] tracking-[-0.055em] xl:text-5xl">
            Entre na web sem entregar o volante.
          </h1>
          <div className="mt-6 flex items-center gap-3 text-xs text-muted-foreground">
            <ShieldCheckIcon className="text-primary" />
            <span>Perfis isolados · cofre cifrado · auditoria de ações</span>
          </div>
        </div>
      </section>

      <section className="relative flex min-h-svh items-center justify-center px-5 py-20 sm:px-10">
        <Button
          variant="ghost"
          render={<Link href="/" />}
          className="absolute left-5 top-5 sm:left-8 sm:top-8"
        >
          <ArrowLeftIcon data-icon="inline-start" />
          Voltar
        </Button>
        <div className="w-full max-w-md">
          <NavegoLogo className="mb-12 lg:hidden" />

          <div className="mb-7 flex flex-col gap-2">
            <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
              {registering ? "Novo control plane" : "Acesso seguro"}
            </p>
            <h2 className="text-4xl font-medium tracking-[-0.05em]">
              {registering ? "Conheça seu Navy" : "Bem-vindo de volta"}
            </h2>
            <p className="text-sm leading-6 text-muted-foreground">
              {registering
                ? "Crie sua conta e abra o primeiro Chromium em poucos instantes."
                : "Entre para continuar de onde seus navegadores pararam."}
            </p>
          </div>

          <form onSubmit={handleSubmit} className="flex flex-col gap-6 rounded-2xl border bg-card p-5 shadow-2xl sm:p-6">
            <FieldGroup>
              {registering && (
                <Field>
                  <FieldLabel htmlFor="name">Nome</FieldLabel>
                  <Input
                    id="name"
                    name="name"
                    placeholder="Como devemos chamar você?"
                    autoComplete="name"
                    minLength={2}
                    maxLength={80}
                    required
                  />
                </Field>
              )}
              <Field>
                <FieldLabel htmlFor="email">Email</FieldLabel>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  placeholder="voce@exemplo.com"
                  autoComplete="email"
                  required
                />
              </Field>
              <Field>
                <div className="flex items-center justify-between gap-3">
                  <FieldLabel htmlFor="password">Senha</FieldLabel>
                  {!registering && (
                    <span className="text-xs text-muted-foreground">
                      Recuperação em breve
                    </span>
                  )}
                </div>
                <div className="relative">
                  <Input
                    id="password"
                    name="password"
                    type={showPassword ? "text" : "password"}
                    autoComplete={registering ? "new-password" : "current-password"}
                    minLength={registering ? 12 : 1}
                    className="pr-10"
                    required
                  />
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    className="absolute right-1 top-1/2 -translate-y-1/2"
                    onClick={() => setShowPassword((current) => !current)}
                    aria-label={showPassword ? "Ocultar senha" : "Mostrar senha"}
                  >
                    {showPassword ? <EyeOffIcon /> : <EyeIcon />}
                  </Button>
                </div>
                {registering && (
                  <FieldDescription>
                    Use pelo menos 12 caracteres. A senha é processada pelo PocketBase.
                  </FieldDescription>
                )}
              </Field>
              {error ? (
                <Field data-invalid>
                  <FieldError>{error}</FieldError>
                </Field>
              ) : null}
            </FieldGroup>

            <Button type="submit" size="xl" disabled={pending} className="w-full">
              {pending ? (
                <LoaderCircleIcon className="animate-spin" data-icon="inline-start" />
              ) : (
                <LockKeyholeIcon data-icon="inline-start" />
              )}
              {pending
                ? "Conectando..."
                : registering
                  ? "Criar minha conta"
                  : "Entrar no Navego"}
              {!pending && <ArrowRightIcon data-icon="inline-end" />}
            </Button>
          </form>

          <p className="mt-6 text-center text-sm text-muted-foreground">
            {registering ? "Já tem uma conta?" : "Ainda não tem uma conta?"}{" "}
            <Link
              href={`${registering ? "/login" : "/register"}?returnTo=${encodeURIComponent(returnTo)}`}
              className="font-medium text-foreground underline decoration-border underline-offset-4 hover:decoration-primary"
            >
              {registering ? "Entrar" : "Criar agora"}
            </Link>
          </p>
        </div>
      </section>
    </main>
  );
}
