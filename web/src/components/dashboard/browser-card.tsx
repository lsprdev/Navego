import {
  MoreHorizontalIcon,
  PencilLineIcon,
  PowerIcon,
  RefreshCwIcon,
  StarIcon,
  Trash2Icon,
} from "lucide-react";
import { useState } from "react";

import { BrowserPreview } from "@/components/dashboard/browser-preview";
import type { BrowserInstance } from "@/components/dashboard/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";

type BrowserCardProps = {
  browser: BrowserInstance;
  previewRevision: number;
  onOpen: () => void;
  onRename: () => void;
  onDelete: () => void;
  onTogglePower: () => void;
  onSetDefault: () => void;
};

const stateLabels: Record<BrowserInstance["state"], string> = {
  running: "Online",
  starting: "Iniciando",
  stopped: "Desligado",
  error: "Com erro",
};

export function BrowserCard({
  browser,
  previewRevision,
  onOpen,
  onRename,
  onDelete,
  onTogglePower,
  onSetDefault,
}: BrowserCardProps) {
  const [manualPreviewRevision, setManualPreviewRevision] = useState(0);

  function refreshPreview() {
    setManualPreviewRevision((current) => current + 1);
  }

  return (
    <Card
      className="gap-0 overflow-hidden py-0 transition-[box-shadow,transform] duration-200 hover:-translate-y-0.5 hover:shadow-xl hover:shadow-background/30"
    >
      <CardHeader className="border-b py-3">
        <CardTitle className="flex min-w-0 items-center gap-2">
          <span
            className={cn(
              "size-2 shrink-0 rounded-full",
              browser.state === "running" &&
                "bg-chart-2 shadow-[0_0_0_4px_color-mix(in_oklch,var(--chart-2),transparent_82%)]",
              browser.state === "starting" && "signal-pulse bg-primary",
              browser.state === "stopped" && "bg-muted-foreground/50",
              browser.state === "error" && "bg-destructive",
            )}
          />
          <span className="truncate">{browser.name}</span>
          {browser.isDefault ? (
            <Badge variant="outline" className="shrink-0">
              <StarIcon />
              Padrão
            </Badge>
          ) : null}
        </CardTitle>
        <CardDescription className="truncate font-mono text-[11px]">
          {browser.id}
        </CardDescription>
        <CardAction>
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button
                  variant="ghost"
                  size="icon-sm"
                  aria-label={`Ações de ${browser.name}`}
                />
              }
            >
              <MoreHorizontalIcon />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="end" className="w-44">
              <DropdownMenuGroup>
                <DropdownMenuItem onClick={onRename}>
                  <PencilLineIcon />
                  Renomear
                </DropdownMenuItem>
                <DropdownMenuItem onClick={onTogglePower}>
                  <PowerIcon />
                  {browser.state === "running" ? "Desligar" : "Ligar"}
                </DropdownMenuItem>
                {browser.isDefault ? null : (
                  <DropdownMenuItem onClick={onSetDefault}>
                    <StarIcon />
                    Tornar padrão
                  </DropdownMenuItem>
                )}
                <DropdownMenuItem onClick={refreshPreview}>
                  <RefreshCwIcon />
                  Atualizar prévia
                </DropdownMenuItem>
              </DropdownMenuGroup>
              <DropdownMenuSeparator />
              <DropdownMenuGroup>
                <DropdownMenuItem variant="destructive" onClick={onDelete}>
                  <Trash2Icon />
                  Excluir
                </DropdownMenuItem>
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </CardAction>
      </CardHeader>

      <BrowserPreview
        browser={browser}
        refreshVersion={`${browser.updatedAt}-${previewRevision}-${manualPreviewRevision}`}
        onRefresh={refreshPreview}
        onOpen={onOpen}
      />

      <CardFooter className="justify-between gap-3 bg-card px-4 py-3">
        <div className="min-w-0">
          <p className="truncate text-sm font-medium">{browser.title}</p>
          <p className="truncate font-mono text-[10px] text-muted-foreground">
            {browser.url}
          </p>
        </div>
        <Badge variant="outline" className="shrink-0 font-mono text-[10px] uppercase">
          {stateLabels[browser.state]}
        </Badge>
      </CardFooter>
    </Card>
  );
}
