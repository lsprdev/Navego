"use client";

import { FormEvent, useState } from "react";
import {
  CircleUserRoundIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  LockKeyholeIcon,
  MailIcon,
  ShieldCheckIcon,
} from "lucide-react";
import { toast } from "sonner";

import type { SessionUser } from "@/components/dashboard/types";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@/components/ui/tabs";

export type AccountView = "profile" | "security";

type AccountDialogProps = {
  user: SessionUser;
  open: boolean;
  initialView: AccountView;
  onOpenChange: (open: boolean) => void;
  onUserChange: (user: SessionUser) => void;
  onSessionEnded: () => void;
};

export function AccountDialog({
  user,
  open,
  initialView,
  onOpenChange,
  onUserChange,
  onSessionEnded,
}: AccountDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="gap-0 overflow-hidden p-0 sm:max-w-2xl">
        <DialogHeader className="border-b px-6 py-5">
          <div className="flex items-center gap-4 pr-8">
            <Avatar className="size-11 rounded-xl">
              <AvatarFallback className="rounded-xl bg-primary/12 font-mono text-sm text-primary">
                {userInitials(user.name)}
              </AvatarFallback>
            </Avatar>
            <div className="min-w-0">
              <DialogTitle className="truncate text-lg">Sua conta</DialogTitle>
              <DialogDescription className="mt-1 truncate">
                {user.email}
              </DialogDescription>
            </div>
          </div>
        </DialogHeader>

        <Tabs defaultValue={initialView} className="gap-0 sm:flex-row">
          <TabsList
            variant="line"
            className="h-auto w-full justify-start gap-5 border-b px-6 py-0 sm:w-44 sm:shrink-0 sm:flex-col sm:items-stretch sm:justify-start sm:gap-1 sm:border-r sm:border-b-0 sm:px-3 sm:py-5"
          >
            <TabsTrigger
              value="profile"
              className="h-11 justify-start px-2.5 sm:w-full"
            >
              <CircleUserRoundIcon />
              Perfil
            </TabsTrigger>
            <TabsTrigger
              value="security"
              className="h-11 justify-start px-2.5 sm:w-full"
            >
              <ShieldCheckIcon />
              Segurança
            </TabsTrigger>
          </TabsList>

          <div className="min-w-0 flex-1 px-6 py-6">
            <TabsContent value="profile">
              <ProfileForm user={user} onUserChange={onUserChange} />
            </TabsContent>
            <TabsContent value="security">
              <PasswordForm onSessionEnded={onSessionEnded} />
            </TabsContent>
          </div>
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}

type ProfileFormProps = {
  user: SessionUser;
  onUserChange: (user: SessionUser) => void;
};

function ProfileForm({ user, onUserChange }: ProfileFormProps) {
  const [name, setName] = useState(user.name);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalizedName = name.trim();
    if (normalizedName.length < 2 || normalizedName.length > 80) {
      setError("O nome deve ter entre 2 e 80 caracteres.");
      return;
    }

    setError("");
    setPending(true);
    try {
      const response = await fetch("/api/account", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ name: normalizedName }),
      });
      const body = (await response.json().catch(() => ({}))) as {
        user?: SessionUser;
        error?: string;
      };
      if (!response.ok || !body.user) {
        throw new Error(body.error || "Não foi possível atualizar seu perfil.");
      }

      setName(body.user.name);
      onUserChange(body.user);
      toast.success("Perfil atualizado.");
    } catch (cause) {
      setError(errorMessage(cause));
    } finally {
      setPending(false);
    }
  }

  return (
    <form className="flex flex-col gap-6" onSubmit={submit}>
      <div>
        <h2 className="text-base font-medium">Informações pessoais</h2>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">
          Esses dados identificam sua conta dentro do Navego.
        </p>
      </div>

      <FieldGroup>
        <Field data-invalid={Boolean(error)}>
          <FieldLabel htmlFor="account-name">Nome</FieldLabel>
          <Input
            id="account-name"
            name="name"
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoComplete="name"
            maxLength={80}
            aria-invalid={Boolean(error)}
            disabled={pending}
          />
          <FieldError>{error}</FieldError>
        </Field>

        <Field>
          <FieldLabel htmlFor="account-email">Email</FieldLabel>
          <div className="relative">
            <MailIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="account-email"
              value={user.email}
              className="pl-8"
              disabled
              readOnly
            />
          </div>
          <FieldDescription>
            A alteração de email será liberada junto ao fluxo de verificação.
          </FieldDescription>
        </Field>
      </FieldGroup>

      <DialogFooter className="mx-0 mb-0 rounded-xl px-4 py-3">
        <Button
          type="submit"
          disabled={pending || name.trim() === user.name}
        >
          {pending && <LoaderCircleIcon className="animate-spin" />}
          Salvar perfil
        </Button>
      </DialogFooter>
    </form>
  );
}

type PasswordFormProps = {
  onSessionEnded: () => void;
};

function PasswordForm({ onSessionEnded }: PasswordFormProps) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!currentPassword) {
      setError("Informe sua senha atual.");
      return;
    }
    if (newPassword.length < 12) {
      setError("A nova senha deve ter pelo menos 12 caracteres.");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("A confirmação não corresponde à nova senha.");
      return;
    }

    setError("");
    setPending(true);
    try {
      const response = await fetch("/api/account/password", {
        method: "PATCH",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ currentPassword, newPassword, confirmPassword }),
      });
      const body = (await response.json().catch(() => ({}))) as {
        error?: string;
      };
      if (!response.ok) {
        throw new Error(body.error || "Não foi possível trocar sua senha.");
      }

      toast.success("Senha alterada. Entre novamente para continuar.");
      onSessionEnded();
    } catch (cause) {
      setError(errorMessage(cause));
      setPending(false);
    }
  }

  return (
    <form className="flex flex-col gap-6" onSubmit={submit}>
      <div>
        <div className="flex items-center gap-2">
          <h2 className="text-base font-medium">Trocar senha</h2>
          <Badge variant="outline" className="font-mono text-[9px] uppercase">
            sessão protegida
          </Badge>
        </div>
        <p className="mt-1 text-sm leading-5 text-muted-foreground">
          Depois da alteração, sua sessão atual será encerrada.
        </p>
      </div>

      <FieldGroup className="gap-4">
        <Field>
          <FieldLabel htmlFor="current-password">Senha atual</FieldLabel>
          <div className="relative">
            <LockKeyholeIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              id="current-password"
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              className="pl-8"
              autoComplete="current-password"
              disabled={pending}
            />
          </div>
        </Field>
        <Field>
          <FieldLabel htmlFor="new-password">Nova senha</FieldLabel>
          <Input
            id="new-password"
            type="password"
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            autoComplete="new-password"
            minLength={12}
            disabled={pending}
          />
          <FieldDescription>Mínimo de 12 caracteres.</FieldDescription>
        </Field>
        <Field data-invalid={Boolean(error)}>
          <FieldLabel htmlFor="confirm-password">Confirmar nova senha</FieldLabel>
          <Input
            id="confirm-password"
            type="password"
            value={confirmPassword}
            onChange={(event) => setConfirmPassword(event.target.value)}
            autoComplete="new-password"
            minLength={12}
            aria-invalid={Boolean(error)}
            disabled={pending}
          />
          <FieldError>{error}</FieldError>
        </Field>
      </FieldGroup>

      <DialogFooter className="mx-0 mb-0 rounded-xl px-4 py-3">
        <Button type="submit" disabled={pending}>
          {pending ? (
            <LoaderCircleIcon className="animate-spin" />
          ) : (
            <KeyRoundIcon />
          )}
          Alterar senha
        </Button>
      </DialogFooter>
    </form>
  );
}

function errorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : "Não foi possível concluir a operação.";
}

function userInitials(name: string) {
  return (
    name
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toLocaleUpperCase())
      .join("") || "NV"
  );
}
