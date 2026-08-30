import { ExpandIcon, ImageOffIcon, LockKeyholeIcon } from "lucide-react";
import Image from "next/image";
import { useState } from "react";

import type { BrowserInstance } from "@/components/dashboard/types";
import { Button } from "@/components/ui/button";

type BrowserPreviewProps = {
  browser: BrowserInstance;
  refreshVersion: string;
  onRefresh: () => void;
  onOpen: () => void;
};

export function BrowserPreview({
  browser,
  refreshVersion,
  onRefresh,
  onOpen,
}: BrowserPreviewProps) {
  const [failedVersion, setFailedVersion] = useState<string | null>(null);
  const imageFailed = failedVersion === refreshVersion;

  if (browser.state !== "running") {
    return (
      <button
        type="button"
        onClick={onOpen}
        className="flex aspect-video w-full flex-col items-center justify-center gap-3 bg-muted/35 outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <span className="signal-pulse size-2 rounded-full bg-muted-foreground" />
        <span className="font-mono text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
          {browser.state === "starting" ? "Inicializando" : "Sem sinal"}
        </span>
      </button>
    );
  }

  if (imageFailed) {
    return (
      <button
        type="button"
        onClick={onRefresh}
        className="flex aspect-video w-full flex-col items-center justify-center gap-3 bg-muted/35 outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
      >
        <ImageOffIcon className="text-muted-foreground" />
        <span className="font-mono text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
          Recarregar captura
        </span>
      </button>
    );
  }

  return (
    <div className="relative aspect-video w-full overflow-hidden bg-muted/35">
      {/* A static capture keeps cards cheap and avoids competing Selkies sessions. */}
      <Image
        src={`/api/browsers/${encodeURIComponent(browser.id)}/preview?v=${encodeURIComponent(refreshVersion)}`}
        alt={`Captura atual de ${browser.name}`}
        fill
        sizes="(max-width: 768px) 100vw, 40vw"
        unoptimized
        className="object-contain object-center"
        onError={() => setFailedVersion(refreshVersion)}
      />
      <button
        type="button"
        onClick={onOpen}
        className="group/open absolute inset-0 flex items-end justify-end bg-transparent p-3 outline-none transition-colors hover:bg-foreground/5 focus-visible:ring-3 focus-visible:ring-inset focus-visible:ring-ring/60"
        aria-label={`Abrir ${browser.name} em tela grande`}
      >
        <span className="flex items-center gap-2 rounded-lg border bg-background/90 px-2.5 py-1.5 text-xs font-medium shadow-lg backdrop-blur-sm transition-transform group-hover/open:-translate-y-0.5">
          <LockKeyholeIcon data-icon="inline-start" />
          Somente leitura
          <ExpandIcon data-icon="inline-end" />
        </span>
      </button>
      <Button
        type="button"
        variant="secondary"
        size="icon-sm"
        className="pointer-events-none absolute right-3 top-3 shadow-lg"
        aria-hidden
        tabIndex={-1}
      >
        <ExpandIcon />
      </Button>
    </div>
  );
}
