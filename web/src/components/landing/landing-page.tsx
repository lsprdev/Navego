import Link from "next/link";
import {
  ArrowRightIcon,
  BotIcon,
  CheckIcon,
  FingerprintIcon,
  Globe2Icon,
  KeyRoundIcon,
  LockKeyholeIcon,
  MonitorUpIcon,
  MousePointer2Icon,
  ShieldCheckIcon,
  SparklesIcon,
  WorkflowIcon,
} from "lucide-react";

import { NavegoLogo } from "@/components/brand/navego-logo";
import { Navy } from "@/components/brand/navy";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { cn } from "@/lib/utils";

type LandingPageProps = {
  authenticated: boolean;
  userName?: string;
};

const commands = [
  "Entre no X e prepare um post",
  "Abra o portal da faculdade",
  "Mostre meu carrinho da Amazon",
  "Tire um print desta página",
  "Pare e me entregue o login",
];

const features = [
  {
    icon: MonitorUpIcon,
    eyebrow: "01 · Sessões",
    title: "Um Chromium para cada contexto",
    description:
      "Separe trabalho, faculdade e contas pessoais em perfis persistentes, visíveis e independentes.",
  },
  {
    icon: MousePointer2Icon,
    eyebrow: "02 · Controle",
    title: "Assuma a tela quando quiser",
    description:
      "Quando surgir um login, CAPTCHA ou decisão delicada, o Navy devolve a tela para você continuar.",
  },
  {
    icon: BotIcon,
    eyebrow: "03 · MCP",
    title: "Converse. O navegador executa.",
    description:
      "Conecte o Navego ao ChatGPT e transforme pedidos em navegação, capturas e ações confirmadas.",
  },
];

export function LandingPage({ authenticated, userName }: LandingPageProps) {
  const primaryHref = authenticated ? "/dashboard" : "/register";
  const primaryLabel = authenticated ? "Abrir dashboard" : "Criar meu navegador";

  return (
    <main className="landing-page min-h-svh overflow-hidden bg-background text-foreground">
      <header className="landing-nav fixed inset-x-0 top-0 z-40 border-b bg-background/80 backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-[1480px] items-center justify-between gap-3 px-5 sm:gap-6 md:px-8">
          <NavegoLogo />
          <nav className="hidden items-center gap-7 text-sm text-muted-foreground md:flex" aria-label="Navegação principal">
            <a href="#produto" className="transition-colors hover:text-foreground">
              Produto
            </a>
            <a href="#como-funciona" className="transition-colors hover:text-foreground">
              Como funciona
            </a>
            <a href="#seguranca" className="transition-colors hover:text-foreground">
              Segurança
            </a>
          </nav>
          <div className="flex items-center gap-2">
            {authenticated ? (
              <span className="hidden max-w-40 truncate text-sm text-muted-foreground sm:block">
                Olá, {userName?.split(" ")[0] || "explorador"}
              </span>
            ) : (
              <Button variant="ghost" render={<Link href="/login" />}>
                Entrar
              </Button>
            )}
            <Button render={<Link href={primaryHref} />}>
              <span className="sm:hidden">
                {authenticated ? "Dashboard" : "Começar"}
              </span>
              <span className="hidden sm:inline">{primaryLabel}</span>
              <ArrowRightIcon data-icon="inline-end" />
            </Button>
          </div>
        </div>
      </header>

      <section className="landing-hero relative flex min-h-[920px] items-center px-5 pb-24 pt-32 md:px-8 md:pt-36">
        <div className="landing-grid absolute inset-0" />
        <div className="landing-glow pointer-events-none absolute left-1/2 top-[34%] size-[680px] -translate-x-1/2 rounded-full" />
        <div className="relative mx-auto grid w-full max-w-[1480px] items-center gap-16 lg:grid-cols-[minmax(0,0.96fr)_minmax(520px,1.04fr)]">
          <div className="max-w-3xl">
            <Badge variant="outline" className="landing-reveal landing-delay-1 border-primary/25 bg-primary/5 text-primary">
              <SparklesIcon data-icon="inline-start" />
              Navy está pronto para navegar
            </Badge>
            <h1 className="landing-reveal landing-delay-2 mt-7 text-5xl font-medium leading-[0.98] tracking-[-0.065em] sm:text-6xl lg:text-7xl xl:text-[88px]">
              A web trabalha.
              <span className="block text-muted-foreground">Você mantém o controle.</span>
            </h1>
            <p className="landing-reveal landing-delay-3 mt-7 max-w-2xl text-lg leading-8 text-muted-foreground md:text-xl">
              Dê ao ChatGPT um navegador real, persistente e visível. Quando for
              hora de entrar, aprovar ou decidir, o Navego chama você de volta.
            </p>
            <div className="landing-reveal landing-delay-4 mt-9 flex flex-col gap-3 sm:flex-row">
              <Button size="xl" render={<Link href={primaryHref} />}>
                {primaryLabel}
                <ArrowRightIcon data-icon="inline-end" />
              </Button>
              <Button
                size="xl"
                variant="outline"
                render={<a href="#produto" />}
              >
                Ver como funciona
              </Button>
            </div>
            <div className="landing-reveal landing-delay-5 mt-9 flex flex-wrap items-center gap-x-6 gap-y-3 text-xs text-muted-foreground">
              <span className="flex items-center gap-2">
                <CheckIcon className="text-primary" /> Perfis persistentes
              </span>
              <span className="flex items-center gap-2">
                <CheckIcon className="text-primary" /> Login humano
              </span>
              <span className="flex items-center gap-2">
                <CheckIcon className="text-primary" /> Ações confirmadas
              </span>
            </div>
          </div>

          <div className="landing-reveal landing-delay-3 relative mx-auto flex min-h-[560px] w-full max-w-[680px] items-center justify-center">
            <div className="landing-orbit landing-orbit-outer absolute size-[500px] rounded-full border sm:size-[590px]" />
            <div className="landing-orbit landing-orbit-inner absolute size-[350px] rounded-full border sm:size-[430px]" />
            <div className="landing-navy-stage landing-float relative flex size-[310px] items-center justify-center rounded-[42%] border bg-card/75 shadow-2xl backdrop-blur-xl sm:size-[390px]">
              <div className="absolute inset-5 rounded-[38%] border border-dashed border-border/70" />
              <Navy className="relative w-[82%] drop-shadow-[0_28px_42px_rgba(0,0,0,.48)]" />
              <span className="absolute bottom-7 font-mono text-[9px] uppercase tracking-[0.28em] text-muted-foreground">
                Ponteiro detectado
              </span>
            </div>
            <StatusNote className="left-0 top-[18%]" icon={Globe2Icon} label="2 Chromiums online" />
            <StatusNote className="bottom-[14%] right-0" icon={FingerprintIcon} label="Controle humano pronto" />
          </div>
        </div>
      </section>

      <div className="landing-command-rail border-y bg-card/45 py-3" aria-hidden="true">
        <div className="landing-command-track flex w-max items-center gap-3">
          {[...commands, ...commands].map((command, index) => (
            <span key={`${command}-${index}`} className="flex items-center gap-3 rounded-full border bg-background px-4 py-2 font-mono text-[10px] uppercase tracking-[0.14em] text-muted-foreground">
              <span className="size-1.5 rounded-full bg-primary" />
              {command}
            </span>
          ))}
        </div>
      </div>

      <section id="produto" className="landing-section px-5 py-28 md:px-8 md:py-36">
        <div className="mx-auto max-w-[1480px]">
          <SectionHeading
            eyebrow="Seu control plane"
            title="Todos os navegadores. Uma única visão."
            description="Veja cada sessão ao vivo, abra o controle em tela cheia e saiba exatamente onde a automação está trabalhando."
          />
          <BrowserControlMockup />
        </div>
      </section>

      <section id="como-funciona" className="landing-section border-y bg-card/35 px-5 py-28 md:px-8 md:py-36">
        <div className="mx-auto max-w-[1480px]">
          <SectionHeading
            eyebrow="Trabalho em três tempos"
            title="Você pede. O Navy navega. Você decide."
            description="A automação continua simples porque o controle humano não é uma exceção: ele faz parte do fluxo."
            centered
          />
          <div className="mt-16 grid gap-4 lg:grid-cols-3">
            {features.map((feature) => (
              <Card key={feature.title} className="landing-feature-card min-h-80 border bg-card/80 py-0 ring-0">
                <CardHeader className="gap-5 px-6 pt-7">
                  <span className="flex size-11 items-center justify-center rounded-xl border bg-secondary text-primary">
                    <feature.icon />
                  </span>
                  <div className="flex flex-col gap-2">
                    <span className="font-mono text-[9px] uppercase tracking-[0.22em] text-primary">
                      {feature.eyebrow}
                    </span>
                    <CardTitle className="text-2xl tracking-[-0.035em]">
                      {feature.title}
                    </CardTitle>
                  </div>
                  <CardDescription className="max-w-md text-base leading-7">
                    {feature.description}
                  </CardDescription>
                </CardHeader>
                <CardContent className="mt-auto px-6 pb-7">
                  <div className="h-px bg-border transition-colors group-hover/card:bg-primary/40" />
                </CardContent>
              </Card>
            ))}
          </div>
        </div>
      </section>

      <section id="seguranca" className="landing-section px-5 py-28 md:px-8 md:py-36">
        <div className="mx-auto grid max-w-[1480px] gap-8 rounded-[2rem] border bg-card p-7 md:p-12 lg:grid-cols-[0.85fr_1.15fr] lg:p-16">
          <div className="flex flex-col justify-between gap-12">
            <div>
              <span className="flex size-12 items-center justify-center rounded-2xl bg-primary text-primary-foreground">
                <ShieldCheckIcon />
              </span>
              <p className="mt-7 font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
                Controle antes de conveniência
              </p>
              <h2 className="mt-3 text-4xl font-medium tracking-[-0.05em] md:text-5xl">
                A senha não precisa virar uma mensagem.
              </h2>
              <p className="mt-5 max-w-xl text-base leading-7 text-muted-foreground">
                Faça login diretamente no Chromium ou guarde credenciais no cofre
                cifrado. O modelo recebe capacidade de agir, não acesso irrestrito
                aos seus segredos.
              </p>
            </div>
            <Navy className="hidden w-44 opacity-90 lg:block" />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <SecurityItem icon={KeyRoundIcon} title="Cofre cifrado" text="Segredos protegidos em repouso, separados por usuário e origem." />
            <SecurityItem icon={FingerprintIcon} title="Login humano" text="Você assume a tela sem enviar senha, token ou código para o chat." />
            <SecurityItem icon={WorkflowIcon} title="Confirmação explícita" text="Publicações, compras e exclusões param antes do clique final." />
            <SecurityItem icon={LockKeyholeIcon} title="Sessões isoladas" text="Cada Chromium mantém perfil, cookies e armazenamento próprios." />
          </div>
        </div>
      </section>

      <section className="landing-section px-5 pb-20 pt-12 md:px-8 md:pb-28">
        <div className="landing-final relative mx-auto flex max-w-[1480px] flex-col items-center overflow-hidden rounded-[2rem] border px-6 py-24 text-center md:py-32">
          <div className="landing-grid absolute inset-0 opacity-55" />
          <Navy className="landing-float relative w-28" />
          <h2 className="relative mt-8 max-w-4xl text-4xl font-medium tracking-[-0.055em] sm:text-5xl md:text-6xl">
            Dê um navegador ao seu próximo pedido.
          </h2>
          <p className="relative mt-5 max-w-xl text-base leading-7 text-muted-foreground">
            Comece localmente, conecte ao ChatGPT e mantenha a tela ao alcance de um clique.
          </p>
          <Button size="xl" className="relative mt-8" render={<Link href={primaryHref} />}>
            {primaryLabel}
            <ArrowRightIcon data-icon="inline-end" />
          </Button>
        </div>
      </section>

      <footer className="border-t px-5 py-8 md:px-8">
        <div className="mx-auto flex max-w-[1480px] flex-col items-start justify-between gap-5 sm:flex-row sm:items-center">
          <NavegoLogo />
          <p className="font-mono text-[9px] uppercase tracking-[0.18em] text-muted-foreground">
            Navegação assistida. Controle humano.
          </p>
        </div>
      </footer>
    </main>
  );
}

function StatusNote({
  className,
  icon: Icon,
  label,
}: {
  className: string;
  icon: typeof Globe2Icon;
  label: string;
}) {
  return (
    <span className={cn("landing-status-note absolute hidden items-center gap-2 rounded-full border bg-background/85 px-3 py-2 text-xs shadow-xl backdrop-blur-xl sm:flex", className)}>
      <Icon className="text-primary" />
      {label}
    </span>
  );
}

function SectionHeading({
  eyebrow,
  title,
  description,
  centered = false,
}: {
  eyebrow: string;
  title: string;
  description: string;
  centered?: boolean;
}) {
  return (
    <div className={cn("landing-scroll-reveal flex max-w-3xl flex-col gap-3", centered && "mx-auto items-center text-center")}>
      <p className="font-mono text-[10px] uppercase tracking-[0.22em] text-primary">
        {eyebrow}
      </p>
      <h2 className="text-4xl font-medium tracking-[-0.055em] md:text-6xl">
        {title}
      </h2>
      <p className="max-w-2xl text-base leading-7 text-muted-foreground md:text-lg">
        {description}
      </p>
    </div>
  );
}

function BrowserControlMockup() {
  return (
    <div className="landing-product-window landing-scroll-reveal mt-14 overflow-hidden rounded-[1.7rem] border bg-card shadow-2xl">
      <div className="flex h-12 items-center justify-between border-b px-4">
        <div className="flex items-center gap-2">
          <span className="size-2.5 rounded-full bg-destructive/80" />
          <span className="size-2.5 rounded-full bg-chart-4/80" />
          <span className="size-2.5 rounded-full bg-primary/80" />
        </div>
        <span className="font-mono text-[9px] uppercase tracking-[0.2em] text-muted-foreground">
          Navego control plane
        </span>
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <span className="signal-pulse size-1.5 rounded-full bg-primary" /> online
        </span>
      </div>
      <div className="grid min-h-[610px] lg:grid-cols-[250px_1fr]">
        <aside className="hidden border-r bg-background/45 p-5 lg:flex lg:flex-col">
          <NavegoLogo />
          <div className="mt-10 flex flex-col gap-2">
            <MockNav active label="Navegadores" />
            <MockNav label="Acessos salvos" />
            <MockNav label="Atividade" />
          </div>
          <div className="mt-auto rounded-xl border bg-card p-3">
            <p className="text-xs font-medium">MCP conectado</p>
            <p className="mt-1 font-mono text-[8px] uppercase tracking-[0.16em] text-primary">
              ChatGPT · online
            </p>
          </div>
        </aside>
        <div className="p-4 sm:p-7 lg:p-9">
          <div className="flex flex-col justify-between gap-5 sm:flex-row sm:items-end">
            <div>
              <p className="font-mono text-[9px] uppercase tracking-[0.2em] text-primary">Sessões isoladas</p>
              <h3 className="mt-2 text-2xl font-medium tracking-[-0.04em] sm:text-3xl">Seus navegadores</h3>
            </div>
            <span className="w-fit rounded-lg bg-primary px-3 py-2 text-xs font-medium text-primary-foreground">+ Novo navegador</span>
          </div>
          <div className="mt-8 grid gap-4 xl:grid-cols-2">
            <MockBrowser name="Trabalho" page="X · Página inicial" accent />
            <MockBrowser name="Faculdade" page="Portal do aluno · Notas" />
          </div>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-4 rounded-xl border bg-background/50 px-4 py-3 text-xs text-muted-foreground">
            <span className="flex items-center gap-2"><ShieldCheckIcon className="text-primary" /> 2 perfis protegidos</span>
            <span className="font-mono text-[9px] uppercase tracking-[0.16em]">Última atividade agora</span>
          </div>
        </div>
      </div>
    </div>
  );
}

function MockNav({ label, active = false }: { label: string; active?: boolean }) {
  return (
    <span className={cn("rounded-lg px-3 py-2 text-sm", active ? "bg-secondary text-foreground" : "text-muted-foreground")}>
      {label}
    </span>
  );
}

function MockBrowser({ name, page, accent = false }: { name: string; page: string; accent?: boolean }) {
  return (
    <div className={cn("overflow-hidden rounded-xl border bg-background", accent && "ring-1 ring-primary/30")}>
      <div className="flex items-center justify-between border-b px-4 py-3">
        <span className="flex items-center gap-2 text-sm font-medium"><span className="size-2 rounded-full bg-primary" />{name}</span>
        <span className="text-muted-foreground">•••</span>
      </div>
      <div className="relative flex aspect-video items-center justify-center overflow-hidden bg-secondary/40">
        <div className="absolute inset-x-8 top-7 h-7 rounded-full border bg-background/80" />
        <div className="grid w-[72%] gap-2 pt-8">
          <span className="h-3 w-1/2 rounded-full bg-muted" />
          <span className="h-2 rounded-full bg-muted/70" />
          <span className="h-2 w-4/5 rounded-full bg-muted/70" />
          <span className="mt-3 h-14 rounded-lg border bg-card" />
        </div>
        {accent ? <Navy className="absolute bottom-3 right-3 w-14" interactive={false} decorative /> : null}
      </div>
      <div className="px-4 py-3">
        <p className="text-sm font-medium">{page}</p>
        <p className="mt-1 font-mono text-[9px] text-muted-foreground">controle disponível</p>
      </div>
    </div>
  );
}

function SecurityItem({ icon: Icon, title, text }: { icon: typeof KeyRoundIcon; title: string; text: string }) {
  return (
    <div className="landing-security-item flex min-h-52 flex-col justify-between rounded-2xl border bg-background/55 p-5">
      <Icon className="text-primary" />
      <div>
        <h3 className="text-lg font-medium">{title}</h3>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">{text}</p>
      </div>
    </div>
  );
}
