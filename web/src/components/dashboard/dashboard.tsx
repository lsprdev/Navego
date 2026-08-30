"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter } from "next/navigation";
import {
  ActivityIcon,
  BotIcon,
  CheckIcon,
  ChevronDownIcon,
  CircleUserRoundIcon,
  Clock3Icon,
  CopyIcon,
  KeyRoundIcon,
  LoaderCircleIcon,
  LogOutIcon,
  PanelsTopLeftIcon,
  PencilLineIcon,
  PlusIcon,
  PowerIcon,
  RefreshCwIcon,
  SettingsIcon,
  SparklesIcon,
  Trash2Icon,
} from "lucide-react";
import { toast } from "sonner";

import { Navy } from "@/components/brand/navy";
import {
  AccountDialog,
  type AccountView,
} from "@/components/dashboard/account-dialog";
import { BrowserCard } from "@/components/dashboard/browser-card";
import {
  BrowserFormDialog,
  DeleteBrowserDialog,
} from "@/components/dashboard/browser-dialogs";
import type {
  ActivityEvent,
  BrowserInstance,
  BrowserState,
  ControlBrowser,
  SavedCredential,
  SavedCredentialInput,
  SessionUser,
} from "@/components/dashboard/types";
import { CredentialsSection } from "@/components/dashboard/credentials-section";
import { ViewerDialog } from "@/components/dashboard/viewer-dialog";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";

type Section = "browsers" | "credentials" | "activity";

const navItems = [
  { id: "browsers" as const, label: "Navegadores", icon: PanelsTopLeftIcon },
  { id: "credentials" as const, label: "Acessos salvos", icon: KeyRoundIcon },
  { id: "activity" as const, label: "Atividade", icon: ActivityIcon },
];

type DashboardProps = {
  initialUser: SessionUser;
  initialBrowsers: ControlBrowser[];
  initialViewerBrowserID?: string;
  mcpURL: string;
};

export function Dashboard({
  initialUser,
  initialBrowsers,
  initialViewerBrowserID,
  mcpURL,
}: DashboardProps) {
  const router = useRouter();
  const [user, setUser] = useState(initialUser);
  const [section, setSection] = useState<Section>("browsers");
  const [browsers, setBrowsers] = useState<BrowserInstance[]>(() =>
    initialBrowsers.map(mapControlBrowser),
  );
  const [previewRevision, setPreviewRevision] = useState(0);
  const [viewerBrowser, setViewerBrowser] = useState<BrowserInstance | null>(
    () => {
      const browser = initialBrowsers.find(
        (candidate) => candidate.id === initialViewerBrowserID,
      );
      return browser ? mapControlBrowser(browser) : null;
    },
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [renameBrowser, setRenameBrowser] = useState<BrowserInstance | null>(
    null,
  );
  const [deleteBrowser, setDeleteBrowser] = useState<BrowserInstance | null>(
    null,
  );
  const [setupOpen, setSetupOpen] = useState(false);
  const [accountView, setAccountView] = useState<AccountView | null>(null);
  const [activityEvents, setActivityEvents] = useState<ActivityEvent[] | null>(
    null,
  );
  const [activityError, setActivityError] = useState("");
  const [activityRefreshing, setActivityRefreshing] = useState(false);
  const [savedCredentials, setSavedCredentials] = useState<
    SavedCredential[] | null
  >(null);
  const [credentialsError, setCredentialsError] = useState("");
  const [credentialsRefreshing, setCredentialsRefreshing] = useState(false);

  const runningCount = useMemo(
    () => browsers.filter((browser) => browser.state === "running").length,
    [browsers],
  );
	const hasPendingBrowser = browsers.some(
		(browser) => browser.state === "starting",
	);

	useEffect(() => {
		if (browsers.length === 0) return;
		let active = true;
		async function refreshBrowsers() {
			if (document.visibilityState !== "visible") return;
			try {
				const response = await fetch("/api/browsers", { cache: "no-store" });
				if (response.status === 401) {
					router.replace("/login");
					return;
				}
				if (!response.ok) return;
				const current = (await response.json()) as ControlBrowser[];
				if (active) setBrowsers(current.map(mapControlBrowser));
			} catch {
				// A later polling cycle can recover from a transient control-plane error.
			}
		}

		void refreshBrowsers();
		const interval = window.setInterval(
			refreshBrowsers,
			hasPendingBrowser ? 2500 : 10_000,
		);
		return () => {
			active = false;
			window.clearInterval(interval);
		};
	}, [browsers.length, hasPendingBrowser, router]);

  useEffect(() => {
    if (section !== "browsers" || runningCount === 0) return;

    const interval = window.setInterval(() => {
      if (document.visibilityState === "visible") {
        setPreviewRevision((current) => current + 1);
      }
    }, 10_000);

    return () => window.clearInterval(interval);
  }, [runningCount, section]);

  async function apiRequest<T = unknown>(
    url: string,
    init?: RequestInit,
  ): Promise<T> {
    const response = await fetch(url, {
      ...init,
      headers: { "content-type": "application/json", ...init?.headers },
    });
    const body = (await response.json().catch(() => ({}))) as T & {
      error?: string;
    };
    if (response.status === 401) {
      router.replace("/login");
      router.refresh();
      throw new Error("Sua sessão expirou.");
    }
    if (!response.ok) {
      throw new Error(body.error || "Não foi possível concluir a operação.");
    }
    return body;
  }

  async function createBrowser(name: string) {
    try {
      const browser = await apiRequest<ControlBrowser>("/api/browsers", {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      setBrowsers((current) => [mapControlBrowser(browser), ...current]);
      toast.success(`${name} entrou na fila de criação.`);
    } catch (error) {
      toast.error(errorMessage(error));
    }
  }

  async function renameSelectedBrowser(name: string) {
    if (!renameBrowser) return;
    try {
      const updated = await apiRequest<ControlBrowser>(
        `/api/browsers/${encodeURIComponent(renameBrowser.id)}`,
        { method: "PATCH", body: JSON.stringify({ name }) },
      );
      setBrowsers((current) =>
        current.map((browser) =>
          browser.id === updated.id ? mapControlBrowser(updated) : browser,
        ),
      );
      toast.success("Nome atualizado.");
      setRenameBrowser(null);
    } catch (error) {
      toast.error(errorMessage(error));
    }
  }

  async function deleteSelectedBrowser() {
    if (!deleteBrowser) return;
    try {
      await apiRequest(`/api/browsers/${encodeURIComponent(deleteBrowser.id)}`, {
        method: "DELETE",
      });
      setBrowsers((current) =>
        current.filter((browser) => browser.id !== deleteBrowser.id),
      );
      toast.success(`${deleteBrowser.name} entrou na fila de remoção.`);
      setDeleteBrowser(null);
    } catch (error) {
      toast.error(errorMessage(error));
    }
  }

  async function togglePower(browser: BrowserInstance) {
    const running = browser.state !== "running";
    try {
      const updated = await apiRequest<ControlBrowser>(
        `/api/browsers/${encodeURIComponent(browser.id)}/power`,
        { method: "POST", body: JSON.stringify({ running }) },
      );
      setBrowsers((current) =>
        current.map((candidate) =>
          candidate.id === updated.id ? mapControlBrowser(updated) : candidate,
        ),
      );
      toast.info(
        running
          ? `${browser.name} será iniciado.`
          : `${browser.name} será desligado.`,
      );
    } catch (error) {
      toast.error(errorMessage(error));
    }
  }

  async function setDefaultBrowser(browser: BrowserInstance) {
    try {
      const updated = await apiRequest<ControlBrowser>(
        `/api/browsers/${encodeURIComponent(browser.id)}/default`,
        { method: "POST" },
      );
      setBrowsers((current) =>
        current.map((candidate) => ({
          ...(candidate.id === updated.id
            ? mapControlBrowser(updated)
            : candidate),
          isDefault: candidate.id === updated.id,
        })),
      );
      toast.success(`${browser.name} agora é o Chromium padrão.`);
    } catch (error) {
      toast.error(errorMessage(error));
    }
  }

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    router.replace("/login");
    router.refresh();
  }

  function openAccount(view: AccountView) {
    setAccountView(view);
  }

  function selectSection(nextSection: Section) {
    setSection(nextSection);
    if (nextSection === "activity") {
      void refreshActivity(activityEvents !== null);
    }
    if (nextSection === "credentials") {
      void refreshCredentials(savedCredentials !== null);
    }
  }

  async function refreshActivity(background = false) {
    if (!background) setActivityRefreshing(true);
    setActivityError("");
    try {
      const events = await apiRequest<ActivityEvent[]>("/api/activity");
      setActivityEvents(events);
    } catch (cause) {
      setActivityError(errorMessage(cause));
    } finally {
      setActivityRefreshing(false);
    }
  }

  async function refreshCredentials(background = false) {
    if (!background) setCredentialsRefreshing(true);
    setCredentialsError("");
    try {
      const credentials = await apiRequest<SavedCredential[]>("/api/credentials");
      setSavedCredentials(credentials);
    } catch (cause) {
      setCredentialsError(errorMessage(cause));
    } finally {
      setCredentialsRefreshing(false);
    }
  }

  async function createCredential(input: SavedCredentialInput) {
    try {
      const credential = await apiRequest<SavedCredential>("/api/credentials", {
        method: "POST",
        body: JSON.stringify(input),
      });
      setSavedCredentials((current) =>
        sortCredentials([...(current ?? []), credential]),
      );
      toast.success(`${credential.label} foi salvo no cofre.`);
      return true;
    } catch (cause) {
      toast.error(errorMessage(cause));
      return false;
    }
  }

  async function updateCredential(
    currentCredential: SavedCredential,
    input: SavedCredentialInput,
  ) {
    try {
      const credential = await apiRequest<SavedCredential>(
        `/api/credentials/${encodeURIComponent(currentCredential.id)}`,
        { method: "PATCH", body: JSON.stringify(input) },
      );
      setSavedCredentials((current) =>
        sortCredentials(
          (current ?? []).map((item) =>
            item.id === credential.id ? credential : item,
          ),
        ),
      );
      toast.success("Acesso atualizado.");
      return true;
    } catch (cause) {
      toast.error(errorMessage(cause));
      return false;
    }
  }

  async function deleteCredential(credential: SavedCredential) {
    try {
      await apiRequest(
        `/api/credentials/${encodeURIComponent(credential.id)}`,
        { method: "DELETE" },
      );
      setSavedCredentials((current) =>
        (current ?? []).filter((item) => item.id !== credential.id),
      );
      toast.success(`${credential.label} foi removido do cofre.`);
      return true;
    } catch (cause) {
      toast.error(errorMessage(cause));
      return false;
    }
  }

  return (
    <SidebarProvider>
      <Sidebar variant="inset" collapsible="icon">
        <SidebarHeader className="p-3">
          <div className="flex items-center gap-3 px-1 py-1">
            <div className="brand-mark relative flex size-9 shrink-0 items-center justify-center overflow-hidden rounded-xl border bg-card">
              <Navy className="size-8" interactive={false} decorative />
            </div>
            <div className="grid min-w-0 flex-1 leading-none group-data-[collapsible=icon]:hidden">
              <span className="text-[17px] font-semibold tracking-[-0.03em]">
                Navego
              </span>
              <span className="mt-1 font-mono text-[9px] uppercase tracking-[0.22em] text-muted-foreground">
                Browser control
              </span>
            </div>
          </div>
        </SidebarHeader>

        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Central</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {navItems.map((item) => (
                  <SidebarMenuItem key={item.id}>
                    <SidebarMenuButton
                      isActive={section === item.id}
                      tooltip={item.label}
                      onClick={() => selectSection(item.id)}
                    >
                      <item.icon />
                      <span>{item.label}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>

          <SidebarGroup>
            <SidebarGroupLabel>Integrações</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    tooltip="Conectar ao ChatGPT"
                    onClick={() => setSetupOpen(true)}
                    className="border border-primary/20 bg-primary/10 text-primary hover:bg-primary/15 hover:text-primary"
                  >
                    <SparklesIcon />
                    <span>Conectar ao ChatGPT</span>
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>

        <SidebarFooter className="p-3">
          <SidebarMenu>
            <SidebarMenuItem>
              <DropdownMenu>
                <DropdownMenuTrigger className="flex h-auto w-full items-center gap-2 overflow-hidden rounded-md p-2 text-left text-sm outline-hidden transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground focus-visible:ring-2 focus-visible:ring-sidebar-ring data-popup-open:bg-sidebar-accent data-popup-open:text-sidebar-accent-foreground group-data-[collapsible=icon]:size-8 group-data-[collapsible=icon]:p-0">
                  <Avatar className="size-8 rounded-lg">
                    <AvatarFallback className="rounded-lg bg-muted font-mono text-xs">
                      {userInitials(user.name)}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid min-w-0 flex-1 text-left text-sm leading-tight group-data-[collapsible=icon]:hidden">
                    <span className="truncate font-medium">{user.name}</span>
                    <span className="truncate text-xs text-muted-foreground">
                      {user.email}
                    </span>
                  </div>
                  <ChevronDownIcon className="ml-auto size-4 group-data-[collapsible=icon]:hidden" />
                </DropdownMenuTrigger>
                <DropdownMenuContent side="top" align="end" className="w-52">
                  <DropdownMenuGroup>
                    <DropdownMenuLabel>Conta</DropdownMenuLabel>
                    <DropdownMenuItem onClick={() => openAccount("profile")}>
                      <CircleUserRoundIcon />
                      Perfil
                    </DropdownMenuItem>
                    <DropdownMenuItem onClick={() => openAccount("security")}>
                      <SettingsIcon />
                      Configurações
                    </DropdownMenuItem>
                  </DropdownMenuGroup>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem onClick={logout}>
                    <LogOutIcon />
                    Sair
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>

      <SidebarInset className="min-w-0 bg-background/75 backdrop-blur-[2px]">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center justify-between gap-4 border-b bg-background/85 px-4 backdrop-blur-xl md:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <SidebarTrigger />
            <Separator orientation="vertical" className="h-5" />
            <div className="flex min-w-0 items-center gap-2 font-mono text-[10px] uppercase tracking-[0.16em] text-muted-foreground">
              <span className="signal-pulse size-1.5 rounded-full bg-chart-2" />
              <span className="truncate">Control plane conectado</span>
            </div>
          </div>
          <Badge variant="outline" className="hidden font-mono text-[10px] sm:inline-flex">
            {runningCount}/{browsers.length} online
          </Badge>
        </header>

        <main className="flex flex-1 flex-col px-4 py-6 md:px-8 md:py-8">
          {section === "browsers" && (
            <BrowserSection
              browsers={browsers}
              previewRevision={previewRevision}
              onOpen={setViewerBrowser}
              onRename={setRenameBrowser}
              onDelete={setDeleteBrowser}
              onTogglePower={togglePower}
              onSetDefault={setDefaultBrowser}
              onCreate={() => setCreateOpen(true)}
            />
          )}
          {section === "credentials" && (
            <CredentialsSection
              credentials={savedCredentials}
              error={credentialsError}
              refreshing={credentialsRefreshing}
              onRefresh={() => refreshCredentials()}
              onCreate={createCredential}
              onUpdate={updateCredential}
              onDelete={deleteCredential}
            />
          )}
          {section === "activity" && (
            <ActivitySection
              events={activityEvents}
              error={activityError}
              refreshing={activityRefreshing}
              onRefresh={() => refreshActivity()}
            />
          )}
        </main>
      </SidebarInset>

      <BrowserFormDialog
        key={`create-${createOpen}`}
        mode="create"
        open={createOpen}
        onOpenChange={setCreateOpen}
        onSubmit={createBrowser}
      />
      <BrowserFormDialog
        key={`rename-${renameBrowser?.id ?? "closed"}`}
        mode="rename"
        browser={renameBrowser}
        open={renameBrowser !== null}
        onOpenChange={(open) => !open && setRenameBrowser(null)}
        onSubmit={renameSelectedBrowser}
      />
      <DeleteBrowserDialog
        browser={deleteBrowser}
        open={deleteBrowser !== null}
        onOpenChange={(open) => !open && setDeleteBrowser(null)}
        onConfirm={deleteSelectedBrowser}
      />
      <ViewerDialog
        browser={viewerBrowser}
        open={viewerBrowser !== null}
        onOpenChange={(open) => {
          if (open) return;
          setViewerBrowser(null);
          if (initialViewerBrowserID) {
            router.replace("/dashboard", { scroll: false });
          }
        }}
      />
      <ChatGPTSetupDialog
        open={setupOpen}
        onOpenChange={setSetupOpen}
        mcpURL={mcpURL}
      />
      <AccountDialog
        key={`${accountView ?? "closed"}-${user.id}-${user.name}`}
        user={user}
        open={accountView !== null}
        initialView={accountView ?? "profile"}
        onOpenChange={(open) => !open && setAccountView(null)}
        onUserChange={setUser}
        onSessionEnded={logout}
      />
    </SidebarProvider>
  );
}

function mapControlBrowser(browser: ControlBrowser): BrowserInstance {
  const stateMap: Record<string, BrowserState> = {
    running: "running",
    stopped: "stopped",
    error: "error",
  };
  return {
    id: browser.id,
    name: browser.name,
    state: stateMap[browser.state] ?? "starting",
    title: browser.title || "Aguardando o Navego Agent",
    url: browser.url || "O container ainda não informou uma página",
    updatedAt: browser.updated_at,
    isDefault: browser.is_default,
  };
}

function errorMessage(error: unknown) {
  return error instanceof Error
    ? error.message
    : "Não foi possível concluir a operação.";
}

function sortCredentials(credentials: SavedCredential[]) {
  return [...credentials].sort((left, right) =>
    left.label.localeCompare(right.label, "pt-BR", { sensitivity: "base" }),
  );
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

type BrowserSectionProps = {
  browsers: BrowserInstance[];
  previewRevision: number;
  onOpen: (browser: BrowserInstance) => void;
  onRename: (browser: BrowserInstance) => void;
  onDelete: (browser: BrowserInstance) => void;
  onTogglePower: (browser: BrowserInstance) => void;
  onSetDefault: (browser: BrowserInstance) => void;
  onCreate: () => void;
};

function BrowserSection({
  browsers,
  previewRevision,
  onOpen,
  onRename,
  onDelete,
  onTogglePower,
  onSetDefault,
  onCreate,
}: BrowserSectionProps) {
  return (
    <div className="mx-auto flex w-full max-w-[1500px] flex-col gap-8">
      <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
        <div className="flex max-w-2xl flex-col gap-2">
          <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
            Sessões isoladas
          </p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] md:text-4xl">
            Seus navegadores
          </h1>
          <p className="max-w-xl text-sm leading-6 text-muted-foreground">
            Cada cartão é um Chromium independente. As prévias são atualizadas
            automaticamente; clique em qualquer uma para assumir o controle.
          </p>
        </div>
        <Button size="lg" onClick={onCreate}>
          <PlusIcon data-icon="inline-start" />
          Novo navegador
        </Button>
      </div>

      {browsers.length === 0 ? (
        <Card>
          <CardContent>
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <PanelsTopLeftIcon />
                </EmptyMedia>
                <EmptyTitle>Nenhum navegador criado</EmptyTitle>
                <EmptyDescription>
                  Crie seu primeiro Chromium isolado para começar a navegar com o
                  ChatGPT.
                </EmptyDescription>
              </EmptyHeader>
              <EmptyContent>
                <Button onClick={onCreate}>
                  <PlusIcon data-icon="inline-start" />
                  Criar primeiro navegador
                </Button>
              </EmptyContent>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <div className="grid grid-cols-1 gap-5 lg:grid-cols-2 2xl:grid-cols-3">
          {browsers.map((browser) => (
            <BrowserCard
              key={browser.id}
              browser={browser}
              previewRevision={previewRevision}
              onOpen={() => onOpen(browser)}
              onRename={() => onRename(browser)}
              onDelete={() => onDelete(browser)}
              onTogglePower={() => onTogglePower(browser)}
              onSetDefault={() => onSetDefault(browser)}
            />
          ))}
        </div>
      )}
    </div>
  );
}

type ActivitySectionProps = {
  events: ActivityEvent[] | null;
  error: string;
  refreshing: boolean;
  onRefresh: () => void;
};

function ActivitySection({
  events,
  error,
  refreshing,
  onRefresh,
}: ActivitySectionProps) {

  return (
    <div className="mx-auto flex w-full max-w-[1100px] flex-col gap-8">
      <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
        <div className="flex max-w-2xl flex-col gap-2">
          <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
            Auditoria
          </p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] md:text-4xl">
            Atividade recente
          </h1>
          <p className="text-sm leading-6 text-muted-foreground">
            Operações realizadas nos seus navegadores, em ordem cronológica.
          </p>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={onRefresh}
          disabled={refreshing}
        >
          <RefreshCwIcon className={refreshing ? "animate-spin" : undefined} />
          Atualizar
        </Button>
      </div>

      {events === null && !error ? (
        <Card>
          <CardContent className="flex min-h-56 items-center justify-center">
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <LoaderCircleIcon className="size-4 animate-spin" />
              Carregando eventos
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
      ) : events?.length === 0 ? (
        <Card>
          <CardContent>
            <Empty>
              <EmptyHeader>
                <EmptyMedia variant="icon">
                  <ActivityIcon />
                </EmptyMedia>
                <EmptyTitle>Nenhum evento registrado</EmptyTitle>
                <EmptyDescription>
                  Crie ou altere um navegador para iniciar seu histórico.
                </EmptyDescription>
              </EmptyHeader>
            </Empty>
          </CardContent>
        </Card>
      ) : (
        <Card className="gap-0 py-0">
          <div className="divide-y">
            {events?.map((event, index) => {
              const presentation = activityPresentation(event.event);
              const EventIcon = presentation.icon;
              return (
                <div
                  key={event.id}
                  className="grid grid-cols-[auto_1fr] gap-3 px-4 py-4 sm:grid-cols-[auto_1fr_auto] sm:items-center sm:px-5"
                >
                  <div className="relative flex size-9 items-center justify-center rounded-lg border bg-muted/40 text-muted-foreground">
                    <EventIcon className="size-4" />
                    {index < (events?.length ?? 0) - 1 && (
                      <span className="absolute top-9 left-1/2 h-4 w-px -translate-x-1/2 bg-border" />
                    )}
                  </div>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">
                      {presentation.label}
                    </p>
                    <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
                      {event.browser_name || event.browser_id || "Navegador removido"}
                    </p>
                  </div>
                  <div className="col-start-2 flex items-center gap-2 sm:col-start-auto sm:justify-end">
                    <Badge
                      variant="outline"
                      className={
                        event.result === "success"
                          ? "border-chart-2/25 text-chart-2"
                          : "border-destructive/25 text-destructive"
                      }
                    >
                      {event.result === "success" ? "Concluído" : "Falhou"}
                    </Badge>
                    <span className="flex items-center gap-1 font-mono text-[10px] text-muted-foreground">
                      <Clock3Icon className="size-3" />
                      {formatActivityDate(event.created_at)}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </Card>
      )}
    </div>
  );
}

function activityPresentation(event: string) {
  const events = {
    "browser.create": { label: "Navegador criado", icon: PlusIcon },
    "browser.rename": { label: "Navegador renomeado", icon: PencilLineIcon },
    "browser.start": { label: "Navegador iniciado", icon: PowerIcon },
    "browser.stop": { label: "Navegador desligado", icon: PowerIcon },
    "browser.delete": { label: "Navegador excluído", icon: Trash2Icon },
    "credential.create": { label: "Acesso salvo", icon: KeyRoundIcon },
    "credential.update": { label: "Acesso atualizado", icon: KeyRoundIcon },
    "credential.delete": { label: "Acesso excluído", icon: Trash2Icon },
  } satisfies Record<string, { label: string; icon: typeof ActivityIcon }>;
  return events[event as keyof typeof events] ?? {
    label: event,
    icon: ActivityIcon,
  };
}

function formatActivityDate(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "Data indisponível";
  return new Intl.DateTimeFormat("pt-BR", {
    dateStyle: "short",
    timeStyle: "short",
  }).format(date);
}

type ChatGPTSetupDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
	mcpURL: string;
};

function ChatGPTSetupDialog({ open, onOpenChange, mcpURL }: ChatGPTSetupDialogProps) {
  async function copyMcpURL() {
    await navigator.clipboard.writeText(mcpURL);
    toast.success("URL do MCP copiada.");
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <div className="mb-1 flex size-10 items-center justify-center rounded-lg bg-primary/15 text-primary">
            <BotIcon />
          </div>
          <DialogTitle>Conectar o Navego ao ChatGPT</DialogTitle>
          <DialogDescription>
            Adicione o servidor MCP uma única vez. O OAuth conecta sua conta
            Navego, sem prender o ChatGPT a um único Chromium.
          </DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <div className="rounded-xl border bg-muted/35 p-4">
            <div className="flex gap-3">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary font-mono text-xs font-semibold text-primary-foreground">
                1
              </span>
              <div className="flex flex-col gap-1">
                <p className="text-sm font-medium">Abra as configurações do ChatGPT</p>
                <p className="text-xs leading-5 text-muted-foreground">
                  Em Apps e conectores, habilite o modo de desenvolvedor e crie um
                  novo servidor MCP.
                </p>
              </div>
            </div>
          </div>
          <div className="rounded-xl border bg-muted/35 p-4">
            <div className="flex gap-3">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-primary font-mono text-xs font-semibold text-primary-foreground">
                2
              </span>
              <div className="min-w-0 flex-1">
                <p className="text-sm font-medium">Cole a URL do servidor</p>
                <div className="mt-2 flex items-center gap-2 rounded-lg border bg-background p-1 pl-3">
                  <code className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">
                    {mcpURL}
                  </code>
                  <Button
                    type="button"
                    variant="ghost"
                    size="icon-sm"
                    onClick={copyMcpURL}
                    aria-label="Copiar URL do MCP"
                  >
                    <CopyIcon />
                  </Button>
                </div>
              </div>
            </div>
          </div>
          <div className="rounded-xl border bg-muted/35 p-4">
            <div className="flex gap-3">
              <span className="flex size-6 shrink-0 items-center justify-center rounded-full bg-chart-2 font-mono text-xs font-semibold text-primary-foreground">
                <CheckIcon />
              </span>
              <div className="flex flex-col gap-1">
                <p className="text-sm font-medium">Autorize o navegador</p>
                <p className="text-xs leading-5 text-muted-foreground">
                  Entre com sua conta Navego e autorize o acesso. Depois você pode
                  pedir “use o Trabalho”; sem um nome, o Chromium padrão é usado.
                </p>
              </div>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            Fechar
          </Button>
          <Button type="button" onClick={copyMcpURL}>
            <CopyIcon data-icon="inline-start" />
            Copiar URL
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
