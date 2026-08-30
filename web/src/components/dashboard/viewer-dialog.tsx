import {
  ExternalLinkIcon,
  LoaderCircleIcon,
  LockKeyholeIcon,
  TriangleAlertIcon,
} from "lucide-react";
import { useEffect, useState } from "react";

import type { BrowserInstance } from "@/components/dashboard/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

type ViewerDialogProps = {
  browser: BrowserInstance | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function ViewerDialog({ browser, open, onOpenChange }: ViewerDialogProps) {
	const [access, setAccess] = useState<{
		browserID: string;
		viewerURL: string | null;
		sessionURL: string | null;
		error: string | null;
	} | null>(null);
	const browserID = browser?.id;
	const browserState = browser?.state;

	useEffect(() => {
		if (!open || !browserID || browserState !== "running") return;
		const selectedBrowserID = browserID;
		const controller = new AbortController();

		async function issueTicket() {
			try {
				const response = await fetch(
					`/api/browsers/${encodeURIComponent(selectedBrowserID)}/viewer-ticket`,
					{ method: "POST", signal: controller.signal },
				);
				const body = (await response.json().catch(() => ({}))) as {
					url?: string;
					session_url?: string;
					error?: string;
				};
				if (!response.ok || !body.url) {
					throw new Error(body.error || "Não foi possível abrir o navegador.");
				}
				setAccess({
					browserID: selectedBrowserID,
					viewerURL: body.url,
					sessionURL: body.session_url ?? null,
					error: null,
				});
			} catch (cause) {
				if (!controller.signal.aborted) {
					setAccess({
						browserID: selectedBrowserID,
						viewerURL: null,
						sessionURL: null,
						error:
							cause instanceof Error
								? cause.message
								: "Não foi possível abrir o navegador.",
					});
				}
			}
		}

		void issueTicket();
		return () => controller.abort();
	}, [browserID, browserState, open]);

  if (!browser) return null;
	const currentAccess = access?.browserID === browser.id ? access : null;
	const viewerURL = currentAccess?.viewerURL ?? null;
	const sessionURL = currentAccess?.sessionURL ?? null;
	const viewerError =
		currentAccess?.error ??
		(browser.state !== "running"
			? "Ligue o navegador antes de abrir o controle remoto."
			: null);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="grid h-[90svh] w-[90vw] max-w-none grid-rows-[auto_1fr] gap-0 overflow-hidden p-0 sm:max-w-none">
        <DialogHeader className="border-b px-4 py-3 pr-14">
          <div className="flex flex-wrap items-center gap-3">
            <DialogTitle className="flex items-center gap-2">
              <span className="size-2 rounded-full bg-chart-2" />
              {browser.name}
            </DialogTitle>
            <Badge variant="outline" className="font-mono text-[10px] uppercase">
              Controle ativo
            </Badge>
          </div>
          <DialogDescription className="flex items-center gap-2 truncate font-mono text-[11px]">
            <LockKeyholeIcon />
            {browser.url}
          </DialogDescription>
        </DialogHeader>
        <div className="relative min-h-0 bg-muted">
          {viewerURL ? (
            <iframe
              src={viewerURL}
              title={`Controle remoto de ${browser.name}`}
              className="size-full border-0"
              allow="clipboard-read; clipboard-write; fullscreen"
            />
          ) : (
            <div className="flex size-full min-h-80 flex-col items-center justify-center gap-3 px-6 text-center">
              {viewerError ? (
                <TriangleAlertIcon className="size-7 text-destructive" />
              ) : (
                <LoaderCircleIcon className="size-7 animate-spin text-primary" />
              )}
              <p className="text-sm font-medium">
                {viewerError ?? "Preparando uma sessão segura…"}
              </p>
              <p className="max-w-sm text-xs text-muted-foreground">
                O endereço interno do Chromium permanece isolado na rede Docker.
              </p>
            </div>
          )}
          {sessionURL ? (
            <Button
              render={
                <a
                  href={sessionURL}
                  target="_blank"
                  rel="noopener noreferrer"
                  aria-label="Abrir navegador em uma nova aba"
                />
              }
              variant="secondary"
              size="icon-sm"
              className="absolute bottom-3 right-3 shadow-xl"
            >
              <ExternalLinkIcon />
            </Button>
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}
