# Status da implementação do Navego

Data da validação: 30 de agosto de 2026.

## Estado atual

O Navego agora possui um control plane multiusuário em construção, um dashboard
Next.js e provisionamento dinâmico de Chromiums. A arquitetura antiga de um
único Chromium continua documentada apenas como histórico. Em 29 de agosto de
2026, seus containers locais foram removidos após confirmação do usuário; os
volumes persistentes não foram apagados.

```text
Next.js BFF -> control Go + PocketBase -> agent Go -> Docker Engine
                                                 -> Chromium + worker por registro
```

Componentes ativos:

- `web/`: cadastro, login, sessão HttpOnly, perfil, troca de senha, dashboard e
  BFF;
- `cmd/navego-control`: PocketBase, API autenticada, estado desejado, previews e
  proxy do viewer;
- `cmd/navego-agent`: reconciliação e único acesso ao Docker socket;
- `cmd/navego`: worker MCP e automação CDP de cada Chromium;
- `docker/chromium/`: imagem do Chromium com Selkies e CDP restrito;
- `compose.yaml`: ambiente local consolidado;
- `compose.dokploy.yaml`: base do deploy com Traefik;
- `compose.ngrok.yaml`: overlay com domínio HTTPS estável para MCP/OAuth.

## Entregue neste milestone

- PocketBase 0.40.1 embutido no processo Go e migrations versionadas;
- collections de browsers, credenciais cifradas, OAuth e auditoria;
- isolamento de registros por usuário e limite inicial de cinco browsers;
- criar, renomear, ligar, desligar e excluir pelo dashboard;
- menu de conta com edição de nome e troca de senha que encerra a sessão atual;
- atividade recente ligada à trilha de auditoria do PocketBase, com atualização
  manual e identificação do navegador afetado;
- cofre de acessos com CRUD autenticado, origem HTTPS canônica e payload
  AES-256-GCM vinculado ao dono e à origem;
- senha nunca retornada pelas APIs de listagem, criação ou edição; uma edição
  com senha vazia preserva o valor já cifrado;
- agent idempotente com labels de ownership, volumes determinísticos, limites de
  CPU/memória/PIDs, logs rotacionados e reconciliação após restart;
- título, URL e heartbeat de cada Chromium reportados pelo worker ao control
  plane, com atualização automática dos cartões;
- somente o agent monta `/var/run/docker.sock`;
- worker no namespace de rede do Chromium, sem CDP ou portas no host;
- preview PNG autenticado e com `no-store`;
- cards com preview estático sob demanda, evitando várias sessões Selkies;
- viewer em diálogo de 90%, com ticket aleatório de uso único, cookie HttpOnly e
  proxy HTTP/WebSocket;
- handoff de autenticação do ChatGPT separado do ticket do iframe: o link de
  takeover dura 15 minutos, tolera prefetch/reload, exige sessão no dashboard,
  confere o mesmo owner do OAuth e abre o Chromium correto no diálogo existente;
- handoff humano cooperativo: a próxima chamada MCP retoma automaticamente a
  automação, inclusive para snapshots e screenshots, e `resume` é idempotente;
- autorização no pedido atual para posts, mensagens e formulários exatos:
  `prepare -> commit` continua vinculando página/campos e bloqueando replay, mas
  ocorre no mesmo turno quando conteúdo e destino já foram explicitamente
  ordenados; efeitos de alto impacto ainda pedem confirmação final;
- Dockerfiles separados para control, agent/worker e frontend standalone;
- rotas Traefik separadas para dashboard/viewer e MCP/OAuth;
- endpoint MCP público único, autenticado por OAuth 2.1 com PKCE S256, registro
  dinâmico de cliente, tokens opacos, refresh com rotação e revogação;
- ferramentas MCP para listar os Chromiums da conta e definir o padrão;
- seletor opcional `browser` por nome exato ou ID em todas as ferramentas do
  worker, com prioridade sobre o padrão configurado no dashboard.

## Validação executada

Passaram:

```text
go test ./...
go vet ./...
TypeScript sem emissão
ESLint
Next.js production build com Webpack
docker compose config
docker compose -f compose.dokploy.yaml config
git diff --check
```

O smoke local real também passou:

1. uma conta descartável criou um browser em estado `queued`;
2. o agent assumiu o registro e criou Chromium, worker e volume;
3. o browser chegou a `running`;
4. o preview retornou um PNG válido de 23 KB;
5. o viewer trocou um ticket descartável por uma sessão e retornou a interface
   Selkies pelo proxy com HTTP 200;
6. a exclusão removeu os dois containers e o volume, sempre filtrando pelas
   labels do browser de teste.

A telemetria também foi validada com dois navegadores simultâneos: os títulos,
URLs e `last_seen` reportados pelo agent foram persistidos no PocketBase sem
reiniciar os Chromiums existentes.

O cofre passou por um smoke isolado com conta descartável: criação, listagem sem
senha, atualização preservando a senha, exclusão da credencial e remoção da
conta de teste com HTTP 204.

O fluxo MCP/OAuth passou por E2E automatizado cobrindo registro dinâmico,
consentimento com login PocketBase, authorization code com PKCE, emissão e
rotação de tokens, listagem de dois Chromiums, troca do padrão, isolamento de um
terceiro Chromium pertencente a outra conta e rejeição após revogação do token.

## Pendências intencionais

1. Validar o endpoint MCP/OAuth no ChatGPT Developer mode usando ngrok ou o
   domínio de produção.
2. Ligar o cofre ao broker de login do worker através do proxy MCP. Até
   lá, os segredos podem ser gerenciados no dashboard, mas não são injetados no
   Chromium.
3. Adicionar convites, quotas configuráveis, recuperação de senha e hardening de
   cadastro antes de permitir usuários não confiáveis.
4. Executar E2E de WebSocket e controle humano no domínio final atrás do
   Traefik; o proxy HTTP e a emissão/consumo do ticket já foram validados localmente.
5. Configurar secrets, backup/restore e Cloudflare Access no deploy.

## Limites de segurança do MVP

- controlar o Docker socket equivale a poder administrativo no host; o agent
  precisa ser tratado como componente privilegiado;
- containers são isolamento operacional, não uma fronteira equivalente a VM;
- tickets do viewer ficam em memória e são perdidos ao reiniciar o control;
- links de takeover também ficam em memória e são perdidos ao reiniciar o
  control; eles não concedem acesso sem uma sessão do mesmo usuário;
- o token agent e a chave worker são globais no host neste estágio;
- perder `NAVEGO_VAULT_KEY` torna os payloads cifrados irrecuperáveis; rotação de
  chave ainda será implementada antes de produção.

O plano principal e as decisões detalhadas ficam em
[`dashboard-platform-plan.md`](dashboard-platform-plan.md).
