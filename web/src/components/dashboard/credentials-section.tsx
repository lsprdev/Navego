"use client";

import { FormEvent, useState } from "react";
import {
  AlertTriangleIcon,
  EyeIcon,
  EyeOffIcon,
  GlobeLockIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  LockKeyholeIcon,
  MoreHorizontalIcon,
  PencilLineIcon,
  PlusIcon,
  RefreshCwIcon,
  ShieldCheckIcon,
  Trash2Icon,
  UserRoundIcon,
} from "lucide-react";

import type {
  SavedCredential,
  SavedCredentialInput,
} from "@/components/dashboard/types";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import {
  Field,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

type CredentialsSectionProps = {
  credentials: SavedCredential[] | null;
  error: string;
  refreshing: boolean;
  onRefresh: () => void;
  onCreate: (input: SavedCredentialInput) => Promise<boolean>;
  onUpdate: (
    credential: SavedCredential,
    input: SavedCredentialInput,
  ) => Promise<boolean>;
  onDelete: (credential: SavedCredential) => Promise<boolean>;
};

export function CredentialsSection({
  credentials,
  error,
  refreshing,
  onRefresh,
  onCreate,
  onUpdate,
  onDelete,
}: CredentialsSectionProps) {
  const [formCredential, setFormCredential] = useState<
    SavedCredential | null | undefined
  >(undefined);
  const [deleteCredential, setDeleteCredential] =
    useState<SavedCredential | null>(null);

  return (
    <div className="mx-auto flex w-full max-w-[1200px] flex-col gap-8">
      <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
        <div className="flex max-w-2xl flex-col gap-2">
          <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
            Cofre de acessos
          </p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] md:text-4xl">
            Logins sem revelar senhas
          </h1>
          <p className="max-w-2xl text-sm leading-6 text-muted-foreground">
            As credenciais ficam cifradas no control plane. O dashboard nunca
            devolve a senha salva e o ChatGPT não recebe o segredo.
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="lg"
            onClick={onRefresh}
            disabled={refreshing}
            aria-label="Atualizar acessos"
          >
            <RefreshCwIcon className={refreshing ? "animate-spin" : undefined} />
          </Button>
          <Button type="button" size="lg" onClick={() => setFormCredential(null)}>
            <PlusIcon />
            Salvar acesso
          </Button>
        </div>
      </div>

      <Card className="border-primary/20 bg-primary/5">
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ShieldCheckIcon className="text-primary" />
            Proteção em duas camadas
          </CardTitle>
          <CardDescription>
            AES-256-GCM protege o conteúdo no banco. A chave mestra fica somente
            no servidor e cada payload é autenticado com o identificador do dono.
          </CardDescription>
        </CardHeader>
      </Card>

      {credentials === null && !error ? (
        <Card>
          <CardContent className="flex min-h-56 items-center justify-center">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" />
              Abrindo o cofre
            </div>
          </CardContent>
        </Card>
      ) : error ? (
        <Card className="border-destructive/20 bg-destructive/5">
          <CardContent className="flex items-center justify-between gap-4">
            <p className="text-sm text-destructive">{error}</p>
            <Button type="button" variant="outline" onClick={onRefresh}>
              Tentar novamente
            </Button>
          </CardContent>
        </Card>
      ) : credentials?.length === 0 ? (
        <Card>
          <CardContent>
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <KeyRoundIcon />
                </EmptyMedia>
                <EmptyTitle>Nenhum acesso salvo</EmptyTitle>
                <EmptyDescription>
                  Cadastre um login para preparar o preenchimento protegido no
                  navegador.
                </EmptyDescription>
              </EmptyHeader>
              <EmptyContent>
                <Button type="button" onClick={() => setFormCredential(null)}>
                  <PlusIcon />
                  Salvar primeiro acesso
                </Button>
              </EmptyContent>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-3">
          {credentials?.map((credential) => (
            <Card key={credential.id} className="gap-4">
              <CardHeader>
                <div className="mb-2 flex size-10 items-center justify-center rounded-xl border bg-muted/45 text-primary">
                  <GlobeLockIcon className="size-5" />
                </div>
                <CardTitle className="truncate">{credential.label}</CardTitle>
                <CardDescription className="truncate font-mono text-[11px]">
                  {credential.origin}
                </CardDescription>
                <CardAction>
                  <DropdownMenu>
                    <DropdownMenuTrigger
                      render={
                        <Button
                          variant="ghost"
                          size="icon-sm"
                          aria-label={`Ações de ${credential.label}`}
                        />
                      }
                    >
                      <MoreHorizontalIcon />
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-44">
                      <DropdownMenuItem
                        onClick={() => setFormCredential(credential)}
                      >
                        <PencilLineIcon />
                        Editar
                      </DropdownMenuItem>
                      <DropdownMenuSeparator />
                      <DropdownMenuItem
                        variant="destructive"
                        onClick={() => setDeleteCredential(credential)}
                      >
                        <Trash2Icon />
                        Excluir
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </CardAction>
              </CardHeader>
              <CardContent className="flex flex-col gap-3">
                <div className="flex items-center gap-2 rounded-lg border bg-muted/25 px-3 py-2.5">
                  <UserRoundIcon className="size-4 shrink-0 text-muted-foreground" />
                  <span className="min-w-0 flex-1 truncate text-sm">
                    {credential.username}
                  </span>
                </div>
                <div className="flex items-center justify-between gap-3">
                  <div className="flex items-center gap-2 font-mono text-xs text-muted-foreground">
                    <LockKeyholeIcon className="size-3.5" />
                    ••••••••••••
                  </div>
                  <Badge variant="outline" className="font-mono text-[9px] uppercase">
                    cifrada
                  </Badge>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      <CredentialFormDialog
        key={formCredential === undefined ? "closed" : formCredential?.id ?? "new"}
        credential={formCredential ?? null}
        open={formCredential !== undefined}
        onOpenChange={(open) => !open && setFormCredential(undefined)}
        onSubmit={async (input) => {
          const saved = formCredential
            ? await onUpdate(formCredential, input)
            : await onCreate(input);
          if (saved) setFormCredential(undefined);
          return saved;
        }}
      />
      <DeleteCredentialDialog
        credential={deleteCredential}
        open={deleteCredential !== null}
        onOpenChange={(open) => !open && setDeleteCredential(null)}
        onConfirm={async () => {
          if (!deleteCredential) return;
          if (await onDelete(deleteCredential)) setDeleteCredential(null);
        }}
      />
    </div>
  );
}

type CredentialFormDialogProps = {
  credential: SavedCredential | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: SavedCredentialInput) => Promise<boolean>;
};

function CredentialFormDialog({
  credential,
  open,
  onOpenChange,
  onSubmit,
}: CredentialFormDialogProps) {
  const [label, setLabel] = useState(credential?.label ?? "");
  const [origin, setOrigin] = useState(credential?.origin ?? "");
  const [username, setUsername] = useState(credential?.username ?? "");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    let normalizedOrigin = "";
    try {
      const parsed = new URL(origin.trim());
      if (parsed.protocol !== "https:") {
        throw new Error("protocol");
      }
      normalizedOrigin = parsed.origin;
    } catch {
      setError("Informe um site HTTPS válido, como https://exemplo.com.");
      return;
    }
    if (label.trim().length < 2) {
      setError("Informe um nome com pelo menos 2 caracteres.");
      return;
    }
    if (!username.trim()) {
      setError("Informe o usuário ou email.");
      return;
    }
    if (!credential && !password) {
      setError("Informe a senha.");
      return;
    }

    setPending(true);
    const saved = await onSubmit({
      label: label.trim(),
      origin: normalizedOrigin,
      username: username.trim(),
      password,
    });
    if (!saved) {
      setPending(false);
      return;
    }
    setPassword("");
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={submit} className="flex flex-col gap-5">
          <DialogHeader>
            <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <GlobeLockIcon />
            </div>
            <DialogTitle>
              {credential ? "Editar acesso" : "Salvar novo acesso"}
            </DialogTitle>
            <DialogDescription>
              A senha é cifrada antes de chegar ao banco e não poderá ser
              visualizada depois de salva.
            </DialogDescription>
          </DialogHeader>

          <FieldGroup className="gap-4">
            <Field>
              <FieldLabel htmlFor="credential-label">Nome</FieldLabel>
              <Input
                id="credential-label"
                value={label}
                onChange={(event) => setLabel(event.target.value)}
                placeholder="Ex.: Faculdade, Amazon pessoal"
                maxLength={80}
                disabled={pending}
                autoFocus
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="credential-origin">Site</FieldLabel>
              <Input
                id="credential-origin"
                type="url"
                value={origin}
                onChange={(event) => setOrigin(event.target.value)}
                placeholder="https://exemplo.com"
                disabled={pending}
              />
              <FieldDescription>
                Você pode colar uma página completa; salvaremos apenas a origem.
              </FieldDescription>
            </Field>
            <Field>
              <FieldLabel htmlFor="credential-username">Usuário ou email</FieldLabel>
              <Input
                id="credential-username"
                value={username}
                onChange={(event) => setUsername(event.target.value)}
                autoComplete="username"
                maxLength={500}
                disabled={pending}
              />
            </Field>
            <Field data-invalid={Boolean(error)}>
              <FieldLabel htmlFor="credential-password">
                {credential ? "Nova senha" : "Senha"}
              </FieldLabel>
              <div className="relative">
                <Input
                  id="credential-password"
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  className="pr-9"
                  autoComplete="new-password"
                  aria-invalid={Boolean(error)}
                  disabled={pending}
                />
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  className="absolute top-1/2 right-0.5 -translate-y-1/2"
                  onClick={() => setShowPassword((current) => !current)}
                  aria-label={showPassword ? "Ocultar senha" : "Mostrar senha"}
                  disabled={pending}
                >
                  {showPassword ? <EyeOffIcon /> : <EyeIcon />}
                </Button>
              </div>
              {credential && (
                <FieldDescription>
                  Deixe em branco para manter a senha atual.
                </FieldDescription>
              )}
              <FieldError>{error}</FieldError>
            </Field>
          </FieldGroup>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
              disabled={pending}
            >
              Cancelar
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? (
                <LoaderCircleIcon className="animate-spin" />
              ) : (
                <ShieldCheckIcon />
              )}
              {credential ? "Salvar alterações" : "Cifrar e salvar"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

type DeleteCredentialDialogProps = {
  credential: SavedCredential | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => Promise<void>;
};

function DeleteCredentialDialog({
  credential,
  open,
  onOpenChange,
  onConfirm,
}: DeleteCredentialDialogProps) {
  const [pending, setPending] = useState(false);

  async function confirm() {
    setPending(true);
    await onConfirm();
    setPending(false);
  }

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia className="bg-destructive/10 text-destructive">
            <AlertTriangleIcon />
          </AlertDialogMedia>
          <AlertDialogTitle>
            Excluir {credential?.label ?? "este acesso"}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            O payload cifrado será removido permanentemente. Essa ação não altera
            a sessão que já existe no Chromium.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>Cancelar</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            onClick={confirm}
            disabled={pending}
          >
            {pending && <LoaderCircleIcon className="animate-spin" />}
            Excluir acesso
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
