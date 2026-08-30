"use client";

import { FormEvent, useState } from "react";
import { AlertTriangleIcon, MonitorUpIcon } from "lucide-react";

import type { BrowserInstance } from "@/components/dashboard/types";
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
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field";
import { Input } from "@/components/ui/input";

type BrowserFormDialogProps = {
  mode: "create" | "rename";
  browser?: BrowserInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (name: string) => void;
};

export function BrowserFormDialog({
  mode,
  browser,
  open,
  onOpenChange,
  onSubmit,
}: BrowserFormDialogProps) {
  const [name, setName] = useState(browser?.name ?? "");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = name.trim();
    if (!normalized) return;
    onSubmit(normalized);
    onOpenChange(false);
  }

  const creating = mode === "create";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={handleSubmit} className="flex flex-col gap-5">
          <DialogHeader>
            <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-primary/15 text-primary">
              <MonitorUpIcon />
            </div>
            <DialogTitle>
              {creating ? "Novo navegador" : "Renomear navegador"}
            </DialogTitle>
            <DialogDescription>
              {creating
                ? "Criaremos um Chromium isolado, com perfil e armazenamento próprios."
                : "O nome muda apenas no Navego; sua sessão continua intacta."}
            </DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="browser-name">Nome</FieldLabel>
              <Input
                id="browser-name"
                name="name"
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="Ex.: Pessoal, Faculdade, Trabalho"
                autoComplete="off"
                autoFocus
                maxLength={48}
              />
              <FieldDescription>
                Use um nome que deixe claro qual identidade será usada nesse perfil.
              </FieldDescription>
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              Cancelar
            </Button>
            <Button type="submit" disabled={!name.trim()}>
              {creating ? "Criar navegador" : "Salvar nome"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

type DeleteBrowserDialogProps = {
  browser: BrowserInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
};

export function DeleteBrowserDialog({
  browser,
  open,
  onOpenChange,
  onConfirm,
}: DeleteBrowserDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogMedia className="bg-destructive/10 text-destructive">
            <AlertTriangleIcon />
          </AlertDialogMedia>
          <AlertDialogTitle>
            Excluir {browser?.name ?? "este navegador"}?
          </AlertDialogTitle>
          <AlertDialogDescription>
            O container e o perfil persistente serão removidos. Sessões, cookies e
            histórico desse navegador não poderão ser recuperados.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancelar</AlertDialogCancel>
          <AlertDialogAction variant="destructive" onClick={onConfirm}>
            Excluir definitivamente
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
