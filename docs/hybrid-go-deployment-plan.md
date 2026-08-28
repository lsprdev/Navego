# Planejamento da arquitetura híbrida em Go e do deploy

Data de revisão: 28 de agosto de 2026.

> **Documento histórico/descontinuado.** O adapter Obscura e o routing híbrido
> foram removidos. O desenho vigente está em
> [Plano Chromium-only do Navego](./chromium-only-plan.md). Este arquivo foi
> preservado apenas para registrar as decisões e alternativas avaliadas.

Este documento complementa:

- [Planejamento de arquitetura e segurança](./architecture-plan.md);
- [Planejamento da reescrita em Go](./go-rewrite-plan.md).

O protótipo Node/Playwright foi removido em 27 de agosto de 2026, depois que o
gateway Go passou pelos testes unitários e pelo smoke real com Chromium. O
perfil persistente do navegador foi preservado. As etapas de paridade restantes
estão registradas em [implementation-status.md](./implementation-status.md).

Status atual: o gateway híbrido, o OAuth Resource Server com scopes e o Compose
do Dokploy/Traefik já estão implementados no repositório. A configuração dos
serviços externos e o E2E remoto seguem o runbook em
[production-deployment.md](./production-deployment.md).

## Resumo da decisão

A arquitetura híbrida é viável e é a melhor opção para os casos definidos:

```text
                                ChatGPT
                                   |
                    Streamable HTTP ou MCP Tunnel
                                   |
                                   v
                        +----------------------+
                        |  Gateway MCP em Go   |
                        | políticas, lease,    |
                        | takeover e aprovação |
                        +-----+-----------+----+
                              |           |
                leitura pública           conta/login/escrita
                              |           |
                              v           v
                         +---------+   +----------------+
                         | Obscura |   | Chromium       |
                         | privado |   | persistente    |
                         +---------+   | GUI + CDP      |
                                       +----------------+
```

Decisões para o primeiro release:

1. O plano de controle será escrito em Go.
2. Obscura e Chromium continuarão sendo engines externas, em containers
   separados do código da aplicação.
3. Obscura fará somente leitura de páginas públicas, extração, screenshot e PDF.
4. Chromium será usado para páginas autenticadas, takeover humano e qualquer
   ação com efeito externo.
5. O ChatGPT verá apenas as ferramentas seguras do gateway; nunca verá o MCP
   bruto do Obscura nem o CDP.
6. Não haverá PocketBase no primeiro release.
7. O perfil do Chromium ficará em um volume nomeado e será incluído nos backups
   do Dokploy.
8. Toda escrita terá fluxo `prepare -> confirmação -> commit`.
9. O deploy inicial terá apenas uma instância do gateway e uma do Chromium.

## Endereços públicos

Os endereços escolhidos para produção são:

| Rota | Destino | Proteção |
| --- | --- | --- |
| `https://browser.lspr.dev/` | GUI do Chromium | Cloudflare Access |
| `https://mcp.browser.lspr.dev/mcp` | Gateway MCP Go | OAuth 2.1 do MCP |
| `https://browser.lspr.dev/control/*` | retomada/status de takeover futuro | Cloudflare Access + validação do JWT no gateway |
| `https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource` | descoberta OAuth | pública |
| `https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource/mcp` | descoberta OAuth específica de `/mcp` | pública |

O Streamable HTTP usa um único endpoint que aceita `GET` e `POST`; não devemos
remover o prefixo `/mcp` no Traefik.

`/control/*` continua planejado e não é exposto pela versão atual. A retomada do
takeover é feita pela tool MCP `browser_resume_after_human`.

### Separação entre Cloudflare Access e OAuth do MCP

O login humano do Cloudflare Access não pode proteger `/mcp`: o ChatGPT não
consegue abrir a tela interativa do Access a cada chamada e também não consegue
enviar um Cloudflare Service Token personalizado. A documentação atual do
ChatGPT informa que servidores MCP autenticados devem implementar o fluxo de
OAuth 2.1 do MCP.

Portanto, no Cloudflare Access teremos:

- aplicação `browser.lspr.dev/*`, com `Allow` apenas para o e-mail do
  proprietário e MFA/OTP;
- `/control/*` sem bypass, herdando a proteção humana da aplicação raiz.

O host `mcp.browser.lspr.dev` não recebe Cloudflare Access. O gateway autentica
todas as chamadas MCP e valida token, audience, issuer, expiração e scopes. WAF
e rate limiting da zona continuam sendo camadas adicionais, mas não substituem
OAuth.

### Decisão: hostname separado para o MCP

O deploy de produção usa:

```text
https://browser.lspr.dev/       -> GUI + Cloudflare Access
https://mcp.browser.lspr.dev/mcp -> MCP + OAuth 2.1
```

Isso elimina exceções de Access no mesmo hostname, reduz o risco de uma regra de
path expor a GUI e simplifica os metadados OAuth.

## Formas de conectar o ChatGPT

Existem dois modos suportados e eles terão papéis diferentes no projeto.

### Modo A: Secure MCP Tunnel

Uso recomendado durante desenvolvimento e para uma instalação estritamente
pessoal:

```text
ChatGPT -> endpoint hospedado pela OpenAI -> tunnel-client -> gateway:8080/mcp
```

Vantagens:

- o gateway não precisa de listener público;
- a conexão parte do servidor para a OpenAI por HTTPS;
- reduz a superfície de ataque enquanto implementamos OAuth;
- funciona bem para desenvolvimento e uso privado no Developer Mode.

Limitação: o tunnel não serve para publicação pública do plugin. A
disponibilidade também depende das permissões da conta/workspace.

### Modo B: endpoint HTTPS público

É o modo necessário para manter `https://mcp.browser.lspr.dev/mcp` e para uma futura
distribuição pública. Requisitos:

- Streamable HTTP em `/mcp`;
- OAuth 2.1 compatível com a especificação MCP;
- Protected Resource Metadata;
- um authorization server com discovery, PKCE e CIMD ou DCR;
- validação de scopes por ferramenta;
- HTTPS estável e certificado válido.

Para evitar escrever um authorization server do zero, a primeira opção será um
provedor estabelecido compatível com MCP, como Auth0. O gateway Go será o
resource server e validará os JWTs via JWKS. PocketBase não substitui esse
authorization server e Cloudflare Access não substitui o OAuth do MCP.

Scopes iniciais:

| Scope | Permite |
| --- | --- |
| `browser:read` | navegar, ler, pesquisar e consultar tabs |
| `browser:capture` | screenshot e PDF |
| `browser:interact` | click, digitação não secreta e seleção |
| `browser:write` | preparar e confirmar efeitos externos |
| `browser:takeover` | criar e concluir takeover humano |

Como será um sistema single-user, o provedor só deve autorizar a identidade do
proprietário.

Referências:

- [Conectar e testar um plugin](https://developers.openai.com/plugins/deploy/connect-chatgpt)
- [Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
- [Autenticação de plugins MCP](https://developers.openai.com/plugins/build/auth)
- [Autorização MCP](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
- [Transporte Streamable HTTP](https://modelcontextprotocol.io/specification/2025-06-18/basic/transports)

## Componentes do runtime

### Gateway MCP em Go

Responsabilidades:

- servir MCP Streamable HTTP;
- validar OAuth, scopes e identidade;
- expor ferramentas pequenas e estáveis;
- decidir qual backend deve atender cada tarefa;
- manter sessões, lease e máquina de estados do takeover;
- bloquear secrets e navegação interna;
- classificar ações com efeito externo;
- implementar `prepare/commit` e proteção contra replay;
- converter screenshots em `ImageContent` MCP;
- registrar auditoria redigida, métricas e health checks;
- detectar falha do Obscura e fazer fallback seguro para Chromium.

Usaremos o [SDK Go oficial do MCP](https://github.com/modelcontextprotocol/go-sdk).
O SDK possui suporte para servidor, cliente e primitivas OAuth, o que também
permite que o gateway seja cliente do MCP interno do Obscura.

### Obscura

O Obscura será um worker privado para conteúdo público. O gateway se conectará
ao MCP HTTP interno dele, mas chamará somente uma allowlist fixa.

Capacidades permitidas pelo adaptador:

- abrir URL pública;
- obter texto/markdown e links;
- obter snapshot semântico quando disponível;
- screenshot;
- PDF;
- esperar o carregamento dentro de timeout limitado.

Capacidades que não serão repassadas:

- avaliação livre de JavaScript;
- leitura, importação ou exportação de cookies/storage;
- submissão de formulário;
- digitação de senha;
- upload de arquivo;
- chamadas HTTP arbitrárias;
- acesso direto ao CDP ou ao MCP interno.

O gateway não deve espelhar dinamicamente a lista de tools do Obscura. Ele terá
adapters com nomes e schemas próprios. Na inicialização, um contract test
confirmará que a versão fixada ainda possui as capacidades internas esperadas.
Se o contrato falhar, a readiness falha e nenhuma tool perigosa aparece por
acidente.

O projeto Obscura está evoluindo rapidamente e o conteúdo atual da branch
principal já difere da imagem que avaliamos localmente. Não usaremos `latest` em
produção: fixaremos versão e digest depois da suíte de compatibilidade.

Referência: [repositório do Obscura](https://github.com/h4ckf0r0day/obscura).

### Chromium persistente

Responsabilidades:

- manter sessões autenticadas no volume `/config`;
- apresentar uma GUI para login, CAPTCHA, passkey e 2FA;
- executar páginas com máxima compatibilidade;
- fazer leitura de dados privados;
- executar interações e ações confirmadas;
- fornecer CDP somente ao gateway.

O CDP continuará em `127.0.0.1:9222`. O gateway e o Chromium compartilharão o
namespace de rede com `network_mode: service:navego-browser`. Isso evita abrir o CDP
para o host ou para outros containers da rede Docker.

Esse arranjo precisa de um teste específico no Dokploy. Como o gateway passa a
compartilhar a rede do Chromium:

- os routers Traefik serão declarados no serviço `chromium`;
- Traefik alcançará a GUI em `3000` e o gateway em `8080` no mesmo IP;
- gateway e Chromium devem ser recriados juntos quando o namespace mudar;
- a rede compartilhada também deverá alcançar o serviço Obscura interno.

Se esse comportamento não for estável no ambiente Dokploy, o fallback é criar
uma imagem Chromium customizada que inclua apenas o binário Go como processo
sidecar supervisionado. Ainda manteremos Obscura separado. Não abriremos CDP em
`0.0.0.0` como atalho de produção.

### Search provider

Obscura é um navegador, não um índice de busca. Para “últimas notícias”, há três
fontes possíveis:

1. o próprio web search do ChatGPT, quando disponível;
2. URLs/fontes fornecidas pelo modelo ao gateway;
3. uma futura interface `SearchProvider` no gateway, usando uma API de busca ou
   uma instância SearXNG.

O primeiro release aceitará URLs e fará leitura/extração. Não devemos depender
de scraping do Google como base do produto. Depois dos fluxos de browser
estarem estáveis, podemos adicionar `web_search` com um provider explícito.

## Roteamento híbrido

O usuário pode escolher explicitamente a engine com um prefixo, e o gateway
mantém essa decisão entre chamadas:

| Prefixo | Modo | Comportamento |
| --- | --- | --- |
| `ob:` | Obscura | público, efêmero e read-only; nunca transfere cookies |
| `ch:` | Chromium | persistente, interativo e apto a login humano |
| nenhum | auto | aplica as regras híbridas abaixo |

`browser_select_backend` troca a engine da página atual. `browser_open` também
aceita `backend=obscura`, `backend=chromium` ou `backend=auto`. Após takeover de
login, os hosts inicial e final são fixados ao Chromium até o processo reiniciar
ou o usuário forçar Obscura explicitamente.

No modo automático, o gateway aplica estas regras:

| Situação | Backend |
| --- | --- |
| URL pública, somente leitura | Obscura |
| Screenshot/PDF de página pública | Obscura |
| Domínio configurado como incompatível com Obscura | Chromium |
| Login, sessão, conta ou conteúdo privado | Chromium |
| Campo de senha, OTP, passkey ou CAPTCHA | takeover humano no Chromium |
| Digitação, upload ou submissão | Chromium |
| Ação com efeito externo | Chromium + prepare/commit |
| Falha de renderização pública no Obscura | fallback para Chromium |

Domínios iniciais `always_chromium`:

- `x.com` e domínios de autenticação associados;
- Amazon e domínios de autenticação associados;
- portal da faculdade;
- qualquer domínio marcado manualmente pelo proprietário.

### Handoff Obscura para Chromium

O handoff não transporta cookies nem storage:

1. Obscura abre e tenta ler a página.
2. O adapter detecta login, conteúdo vazio, erro de módulo, desafio ou
   incompatibilidade.
3. O gateway encerra o contexto público.
4. Chromium abre a mesma URL.
5. Um novo snapshot é gerado, com novas refs.
6. Se houver autenticação necessária, a automação entra em takeover.

Nenhuma ref de elemento do Obscura será reutilizada no Chromium.

## Ferramentas MCP públicas

O gateway esconderá os backends e exporá um conjunto pequeno:

### Leitura e navegação

- `browser_status`
- `browser_select_backend`
- `browser_open`
- `browser_snapshot`
- `browser_find`
- `browser_list_tabs`
- `browser_new_tab`
- `browser_switch_tab`
- `browser_close_tab`
- `browser_wait`
- `browser_take_screenshot`
- `browser_export_pdf`

### Interação reversível

- `browser_click`
- `browser_type`
- `browser_select_option`
- `browser_press_key`

Essas tools são executadas apenas no Chromium. `browser_type` recusa campos de
senha e valores classificados como secrets.

### Takeover humano

- `browser_request_human_login`
- `browser_takeover_status`

### Efeitos externos

- `browser_prepare_action`
- `browser_commit_action`
- `browser_cancel_action`

Não haverá tool pública para:

- escolher `obscura` ou `chromium` arbitrariamente;
- executar JavaScript, shell ou Go;
- fornecer CSS/XPath livre;
- exportar cookies ou local storage;
- fazer request HTTP genérico;
- acessar arquivos do servidor;
- controlar CDP;
- chamar uma tool do Obscura pelo nome.

Todas as tools terão schema, output estruturado e annotations MCP de leitura,
escrita, destrutividade e acesso externo. Outputs textuais terão limite e
paginação para evitar desperdício de tokens.

## Login humano e takeover

Fluxo:

1. O Chromium encontra login, CAPTCHA, 2FA ou passkey.
2. O gateway muda de `AUTOMATION_ACTIVE` para `HUMAN_REQUIRED`.
3. O ChatGPT recebe `https://browser.lspr.dev` e um takeover ID opaco.
4. O usuário passa pelo Cloudflare Access.
5. O usuário digita secrets diretamente no site.
6. Durante `HUMAN_ACTIVE`, todas as tools de browser são bloqueadas.
7. O usuário responde “pronto” no chat ou usa
   `https://browser.lspr.dev/control/takeovers/<id>`.
8. O gateway valida o JWT do Cloudflare Access, retoma o lease e gera um novo
   snapshot.
9. Login concluído não equivale a aprovação de uma escrita.

Estados:

```text
IDLE
  -> AUTOMATION_ACTIVE
  -> HUMAN_REQUIRED
  -> HUMAN_ACTIVE
  -> RESUMING
  -> AUTOMATION_ACTIVE
  -> IDLE
```

Para o primeiro release, responder “pronto” no chat é suficiente. A rota
`/control` poderá entrar no segundo incremento para evitar modificar a GUI do
Chromium logo no início.

## Escritas e confirmações

Toda ação externa seguirá dois passos:

1. `browser_prepare_action` abre/preenche a tela e retorna o conteúdo final,
   destino, URL, elemento e um `approval_id` curto.
2. Depois da confirmação explícita, `browser_commit_action` verifica novamente
   todos os vínculos e executa exatamente uma vez.

O approval ID ficará ligado a:

- identidade OAuth;
- sessão MCP e lease;
- tab, origem e URL;
- tipo da ação;
- hash do conteúdo final;
- hash do elemento final;
- expiração curta;
- nonce de uso único.

Se a página, conteúdo ou destino mudar, a aprovação será invalidada.

Há um limite importante: não existe classificação universal e perfeita de todo
botão de todos os sites. Para o primeiro release, ações genéricas desconhecidas
que possam submeter formulário exigirão confirmação, e os casos de maior valor
terão políticas específicas, começando por publicar no X. Compras e pagamentos
continuarão bloqueados até termos uma política dedicada e testes próprios.

## Estrutura Go proposta

```text
cmd/
  navego/
    main.go

internal/
  app/
  config/
  mcpserver/
  auth/
  backend/
    backend.go
    router.go
  obscura/
    client.go
    allowlist.go
    adapter.go
    contract.go
  chromium/
    client.go
    targets.go
    snapshot.go
    actions.go
    screenshot.go
  policy/
    urls.go
    network.go
    domains.go
    secrets.go
    effects.go
  takeover/
  approval/
  artifacts/
  audit/
  observability/
  storage/
    memory.go

test/
  fixtures/
  integration/
  e2e/

docker/
  chromium/
  local/

docs/
  architecture-plan.md
  go-rewrite-plan.md
  hybrid-go-deployment-plan.md

Dockerfile
compose.yaml
compose.local.yaml
compose.dokploy.yaml
.env.example
go.mod
go.sum
```

Interfaces principais:

```go
type PublicBrowser interface {
    Open(ctx context.Context, url string) (Page, error)
    Snapshot(ctx context.Context) (Snapshot, error)
    Screenshot(ctx context.Context, opts ScreenshotOptions) ([]byte, error)
    PDF(ctx context.Context, opts PDFOptions) ([]byte, error)
}

type AccountBrowser interface {
    Open(ctx context.Context, url string) (Page, error)
    Snapshot(ctx context.Context) (Snapshot, error)
    Click(ctx context.Context, ref Ref) error
    Type(ctx context.Context, ref Ref, value string) error
    Screenshot(ctx context.Context, opts ScreenshotOptions) ([]byte, error)
}
```

Essas interfaces são limites de teste; não são uma tentativa de unificar todas
as capacidades das engines.

## Docker

### Dockerfile do gateway

O `Dockerfile` raiz terá build multi-stage:

1. imagem Go com versão fixada;
2. download de módulos usando cache do BuildKit;
3. `go test` e build com `-trimpath`;
4. `CGO_ENABLED=0` enquanto PocketBase não for necessário;
5. runtime distroless/nonroot com CA certificates e timezone;
6. filesystem read-only, `/tmp` em tmpfs e nenhuma shell no runtime.

O Dockerfile não incluirá Chromium nem Obscura. Cada engine usa sua imagem
fixada por versão e digest.

### Compose canônico

Serviços:

```text
navego-browser  GUI, perfil persistente e CDP loopback
navego-gateway  gateway Go; network_mode: service:navego-browser
obscura         MCP interno, sem porta pública
mcp-tunnel      profile opcional para desenvolvimento/uso privado
```

Regras:

- nenhum `container_name`, pois interfere em recursos do Dokploy;
- nenhuma porta do CDP ou Obscura publicada;
- apenas GUI e gateway serão alcançáveis pelo Traefik;
- `navego-browser-data` será volume nomeado;
- screenshots serão retornados diretamente pelo MCP;
- artefatos temporários usarão tmpfs e TTL;
- health checks em todos os serviços;
- versões e digests fixados;
- `restart: unless-stopped`;
- `shm_size` explícito para Chromium;
- CPU, memória, PIDs e tamanho de logs limitados;
- `cap_drop: ALL` e `no-new-privileges` onde forem compatíveis;
- nenhuma montagem de `/var/run/docker.sock`;
- nenhum secret em label, imagem ou repositório.

O Chromium pode exigir exceções relacionadas a sandbox/user namespaces. Isso
será validado em um spike; não usaremos `--no-sandbox` sem documentar e compensar
o risco.

### Arquivos Compose

- `compose.yaml`: topologia comum e redes internas;
- `compose.local.yaml`: publica GUI/MCP apenas em `127.0.0.1` e usa perfil de
  teste;
- `compose.ngrok.yaml`: mantém o teste HTTPS local já existente;
- `compose.dokploy.yaml`: labels Traefik, `dokploy-network`, limites e política de
  produção.

## Traefik no Dokploy

Usaremos Docker Compose, não Docker Stack. O browser possui estado e deve ter uma
única réplica; Stack não traria vantagem inicial e exigiria imagens previamente
publicadas.

O Dokploy recomenda configurar domínios pela interface, mas neste projeto as
labels manuais são preferíveis porque precisamos de múltiplas rotas e portas no
mesmo hostname, prioridade explícita e configuração versionada.

Routers implementados:

| Router | Regra | Porta |
| --- | --- | --- |
| `navego-mcp` | `Host(mcp.browser.lspr.dev) && (Path(/mcp) || Path(/.well-known/...))` | `8001` |
| `navego-gui` | `Host(browser.lspr.dev)` | `3000` |

As regras exatas usarão a sintaxe v3 do Traefik. Não haverá middleware
`StripPrefix` para `/mcp`. Os routers e services terão nomes únicos e todos os
routers HTTPS usarão o mesmo TLS resolver/opções para o hostname.

Somente o serviço `navego-browser`, cujo namespace contém também o listener do
gateway, será conectado à `dokploy-network`. Obscura ficará apenas na rede
interna do projeto.

Antes do deploy, o botão **Preview Compose** do Dokploy deve ser usado para
confirmar que ele não gerou routers duplicados. Não devemos configurar o mesmo
domínio simultaneamente pela UI e por labels.

Referências:

- [Docker Compose no Dokploy](https://docs.dokploy.com/docs/core/docker-compose)
- [Domínios de Compose no Dokploy](https://docs.dokploy.com/docs/core/docker-compose/domains)
- [Traefik com Docker](https://doc.traefik.io/traefik/reference/routing-configuration/other-providers/docker/)
- [Regras e prioridades do Traefik](https://doc.traefik.io/traefik/master/reference/routing-configuration/http/routing/rules-and-priority/)

## Cloudflare e origem

### Caminho principal

```text
Internet
   -> Cloudflare DNS proxy + WAF + Access
   -> Traefik do Dokploy em 443
   -> router por host/path
   -> Chromium GUI ou gateway Go
```

Configuração:

1. criar registro DNS proxied para `browser.lspr.dev`;
2. usar TLS em modo Full (strict);
3. criar a aplicação Access raiz para o e-mail autorizado;
4. criar somente os bypasses específicos necessários para MCP e metadata;
5. adicionar rate limit/WAF para `/mcp`;
6. bloquear acesso direto à origem, permitindo 80/443 somente pelas redes da
   Cloudflare, ou usar Authenticated Origin Pulls;
7. validar que requests ao IP do servidor com `Host: browser.lspr.dev` não
   contornam o Access.

### Hardening opcional: Cloudflare Tunnel

Como melhoria posterior, `cloudflared` pode criar uma conexão outbound-only e
eliminar portas públicas na origem. Traefik continua fazendo o roteamento, mas o
connector precisa apontar para o Traefik dentro da rede do Dokploy. Faremos esse
modo somente depois de confirmar o nome/endereço interno do Traefik da instalação
real; não vamos acoplar o Compose a um nome de container presumido.

Referências:

- [Caminhos de aplicações no Access](https://developers.cloudflare.com/cloudflare-one/access-controls/policies/app-paths/)
- [Políticas do Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/access-controls/policies/)
- [Aplicação web privada com Tunnel](https://developers.cloudflare.com/cloudflare-one/setup/secure-private-apps/private-web-app/)

## Persistência e backup

Persistente:

- `navego-browser-data:/config`;
- configuração externa/secrets no Dokploy;
- opcionalmente logs agregados fora dos containers.

Não persistente por padrão:

- sessão de transporte MCP;
- leases;
- aprovações pendentes;
- snapshots/DOM;
- screenshot/PDF já entregue;
- contexto do Obscura.

O volume do Chromium contém cookies e sessões equivalentes a credenciais. O
backup deve ser criptografado, com acesso restrito e retenção curta. Antes de
atualizar a imagem do Chromium:

1. interromper novas tarefas;
2. criar backup do volume pelo Dokploy;
3. testar a nova imagem em perfil separado;
4. fazer deploy em janela de manutenção;
5. executar smoke tests;
6. manter a versão anterior disponível para rollback.

O Dokploy só faz Volume Backup automático de volumes nomeados, por isso não
usaremos bind mount para `/config` em produção.

Referência: [Volume Backups do Dokploy](https://docs.dokploy.com/docs/core/volume-backups).

## Observabilidade

Endpoints internos:

- `/healthz`: processo vivo;
- `/readyz`: OAuth, Chromium e contrato do Obscura prontos;
- `/metrics`: métricas, nunca público;
- `/debug/*`: desabilitado em produção.

Logs JSON incluirão:

- request/session ID opaco;
- backend selecionado;
- domínio sem query string;
- nome da tool;
- duração, status e tipo de fallback;
- transições de takeover;
- prepare/commit sem conteúdo secreto.

Nunca registrar:

- senhas, OTPs, cookies, tokens ou headers de autorização;
- query strings potencialmente sensíveis;
- DOM completo;
- texto de campos secretos;
- screenshots;
- conteúdo privado integral.

Métricas mínimas:

- chamadas e erros por tool/backend;
- latência;
- fallbacks Obscura -> Chromium;
- takeovers iniciados/concluídos/expirados;
- aprovações preparadas, confirmadas e rejeitadas;
- reconexões CDP;
- uso de CPU/memória e espaço do volume.

## Segurança de rede e conteúdo

O gateway validará todas as URLs e redirecionamentos:

- apenas `http` e `https`;
- bloquear loopback, link-local, redes privadas, multicast e metadados cloud;
- resolver e verificar todos os IPs para reduzir DNS rebinding;
- bloquear acesso aos serviços internos Docker;
- limitar redirects, response size e tempo;
- aplicar o mesmo controle no Obscura e Chromium.

Páginas são conteúdo não confiável. Texto de um site nunca pode:

- alterar as políticas do gateway;
- liberar tools escondidas;
- fornecer uma aprovação;
- solicitar cookie/token;
- mudar o destino de uma ação já aprovada.

Para páginas com prompt injection, o snapshot deve separar claramente conteúdo
da página de instruções do sistema. Toda escrita permanece vinculada ao fluxo de
aprovação no servidor, independentemente do que a página disser.

## Estratégia de implementação

### Fase 0: preservar o baseline — encerrada

- o baseline foi mantido durante a fundação Go;
- tools e smoke tests foram registrados;
- o volume Chromium foi preservado;
- o legado Node foi removido após o smoke MCP Go.

Critério: o protótipo continua disponível para comparação e rollback.

### Fase 1: fundação Go

- criar `go.mod` com o SDK MCP oficial;
- implementar config, logging, health e shutdown;
- servir `browser_status` por Streamable HTTP;
- criar interfaces dos dois backends;
- adicionar testes unitários e `go test -race`.

Critério: MCP Go inicializa, lista tools e encerra corretamente.

### Fase 2: adapter Obscura

- fixar uma versão candidata do Obscura;
- executar MCP HTTP somente na rede interna;
- implementar cliente MCP Go e allowlist;
- mapear open, snapshot/markdown, links, screenshot e PDF;
- testar SSRF, limite de output e incompatibilidades;
- criar contract test de versão/capacidades.

Critério: leitura pública e screenshot funcionam sem expor tools perigosas.

### Fase 3: adapter Chromium

- conectar via CDP loopback;
- implementar targets, tabs, navegação, screenshot e reconexão;
- criar snapshot compacto com refs opacas;
- implementar click/type/select/wait sem selector livre;
- testar iframe, shadow DOM e páginas reativas prioritárias.

Critério: fluxos básicos funcionam e a desconexão do gateway não fecha o browser.

Status local: tabs, navegação, screenshot, snapshot, click/type e wait estão
implementados. Uma aba real manteve o mesmo target ID após a recriação somente
do gateway. `select`, teclas especiais e casos avançados de iframe/shadow DOM
continuam pendentes.

### Fase 4: roteador híbrido

- classificar tarefa, domínio e capability;
- implementar `always_chromium`;
- detectar auth/incompatibilidade;
- fazer handoff com novas refs;
- adicionar circuit breaker e fallback;
- registrar qual backend foi usado.

Critério: páginas públicas usam Obscura e casos autenticados nunca passam por ele.

Status local: `always_chromium`, handoff, fallback e circuit breaker em memória
estão implementados. O Obscura abre o circuito após três falhas, usa cooldown de
30 segundos e expõe `closed/open/half_open` em `browser_status`.

### Fase 5: takeover e lease

- máquina de estados;
- um único lease de escrita;
- bloqueio total durante controle humano;
- URL estável da GUI;
- retomada por chat e depois `/control`;
- expiração e recuperação de lease órfão.

Critério: automação não consegue interagir enquanto o usuário controla a GUI.

### Fase 6: aprovação e políticas

- prepare/commit com HMAC e nonce;
- scopes OAuth por tool;
- bloqueio de secrets;
- classificação de submit e ações conhecidas;
- política específica inicial para publicação no X;
- auditoria redigida.

Critério: nenhuma postagem, envio ou alteração ocorre sem confirmação vinculada.

Status local: `prepare/commit`, bloqueio de secrets, classificação sensível e
scopes OAuth por tool estão implementados. HMAC ligado à identidade e auditoria
persistente continuam pendentes.

### Fase 7: Docker local

- criar Dockerfile Go multi-stage;
- separar serviços no Compose;
- validar `network_mode: service:navego-browser`;
- manter CDP e Obscura inacessíveis pelo host;
- usar `compose.local.yaml` e perfis descartáveis;
- medir memória, CPU e tamanho de outputs.

Critério: todos os testes rodam com um único `docker compose up`.

Status local: Dockerfile Go, sidecar de namespace, redes privadas, filesystem
read-only e Compose local passaram por build, config e smoke. O Chromium foi
fixado em X11 porque a versão testada bloqueou screenshots CDP sob Wayland.

### Fase 8: conexão ChatGPT

- validar primeiro com MCP Inspector;
- usar Secure MCP Tunnel ou ngrok no desenvolvimento;
- testar tools, imagens, annotations e confirmações no ChatGPT;
- implementar e testar OAuth 2.1 antes de abrir `/mcp` permanente;
- comparar custo de tokens com o protótipo.

A redução de tokens será feita nesta fase, não durante a integração inicial
dos backends. O trabalho inclui eliminar texto/JSON duplicados, retornar somente
status ou delta após `click`/`type`, paginar elementos e permitir snapshots
focados. O alvo inicial é reduzir em 80% ou mais o volume observado no E2E do X,
sem enfraquecer refs, verificação de campos ou aprovações.

Critério: ChatGPT executa os cenários principais sem receber CDP ou tools brutas.

Status: o OAuth Resource Server, discovery RFC 9728, JWT/JWKS, scopes e desafios
de step-up estão implementados e cobertos por testes. Falta configurar o Auth0 e
executar o fluxo real pelo endpoint HTTPS do ChatGPT.

### Fase 9: Dokploy e Traefik

- criar Compose application a partir do Git;
- configurar secrets no Dokploy;
- anexar à `dokploy-network`;
- adicionar labels com routers e prioridades;
- revisar Preview Compose;
- configurar volume nomeado e backup S3;
- executar smoke tests internos antes de DNS público.

Critério: rotas chegam aos ports corretos e nenhuma porta sensível está publicada.

Status: `compose.dokploy.yaml`, pins de imagens, volume nomeado e routers Traefik
com paths exatos estão implementados e validados pelo Compose. Preview e deploy
no servidor real continuam pendentes.

### Fase 10: Cloudflare e produção

- DNS proxied e TLS strict;
- Access para GUI e control;
- bypass mínimo para MCP/metadata;
- OAuth obrigatório no gateway;
- firewall da origem;
- rate limiting e alertas;
- testes de bypass por IP e Host header;
- rollback documentado.

Critério: GUI exige login, MCP exige OAuth e a origem não é acessível por fora da
Cloudflare.

### Fase 11: corte e limpeza

- executar toda a matriz E2E;
- migrar o perfil real somente após backup;
- manter uma release anterior funcional;
- validar o corte já realizado de Node/Playwright;
- manter o projeto sem dependências JavaScript de runtime.

Critério: a versão Go cobre os casos aceitos e existe rollback testado.

## Matriz de testes de aceitação

| Cenário | Resultado esperado |
| --- | --- |
| Página pública simples | Obscura lê e retorna snapshot compacto |
| Página pública com screenshot | Obscura retorna `ImageContent` |
| Página pública incompatível | fallback controlado para Chromium |
| X sem login | takeover humano, sem senha no MCP |
| Post no X | rascunho, confirmação e commit único |
| Carrinho Amazon | Chromium lê sem alterar itens |
| Portal da faculdade | takeover e screenshot pelo Chromium |
| Clique de submit desconhecido | confirmação ou bloqueio |
| Campo de senha | `browser_type` recusa e solicita takeover |
| URL para `127.0.0.1`/LAN/metadata | bloqueada nos dois backends |
| Prompt injection na página | não altera política nem aprova ação |
| Duas sessões simultâneas | somente uma recebe lease de escrita |
| Reinício do gateway | approvals pendentes são invalidadas |
| Reinício do Chromium | perfil autenticado persiste |
| MCP sem token | `401` com `WWW-Authenticate` correto |
| Token com audience/scope errado | rejeitado |
| GUI sem Access | bloqueada pela Cloudflare |
| Request direto ao IP da origem | bloqueado |
| CDP/Obscura a partir do host | inacessíveis |
| Restore de backup | perfil é recuperado em ambiente isolado |

## Viabilidade e limitações

| Objetivo | Viabilidade | Observação |
| --- | --- | --- |
| Gateway MCP totalmente em Go | Alta | SDK oficial disponível |
| Leitura pública com Obscura | Alta | fallback necessário para SPAs incompatíveis |
| Screenshot/PDF público com Obscura | Alta | renderização pode diferir do Chromium |
| Login humano remoto | Alta | GUI persistente + Cloudflare Access |
| Carrinho Amazon | Média/alta | depende de desafios e mudanças do site |
| Screenshot autenticado | Alta | Chromium preserva a sessão |
| Publicação no X | Média | UI, anti-bot e regras do serviço podem mudar |
| Ações genéricas em qualquer site | Média | classificação de efeitos nunca é perfeita |
| `/mcp` no mesmo hostname da GUI | Alta | exige bypasses de path cuidadosamente testados |
| Subdomínio MCP separado | Alta | desenho mais simples e seguro |
| Deploy no Dokploy/Traefik | Alta | validar sidecar de namespace no servidor real |
| PocketBase no release inicial | Desnecessário | memória + volume do browser são suficientes |
| Múltiplos usuários | Fora do v1 | exige perfis isolados, storage e autorização por usuário |

Obstáculos que não podem ser eliminados pelo código:

- sites podem bloquear automação ou exigir CAPTCHA;
- layouts e seletores mudam;
- contas podem ter regras que proíbem ou limitam automação;
- o usuário ainda precisa assumir controle para secrets e desafios;
- uma sessão autenticada comprometida tem os mesmos privilégios da conta;
- Chromium domina memória e tamanho mesmo com gateway pequeno em Go.

## Melhorias futuras

Depois do primeiro release estável:

- profiles Chromium separados por categoria de confiança, por exemplo social,
  compras e faculdade;
- PocketBase para múltiplos perfis, painel de sessões, auditoria consultável e
  artifacts com TTL;
- provider de busca explícito;
- redaction opcional de screenshots;
- recipes versionadas para X e outros fluxos de alto impacto;
- múltiplos workers Obscura;
- Cloudflare Tunnel para eliminar listener público da origem;
- proxy de saída que aplique bloqueio de rede também fora da aplicação.

## Ordem recomendada de decisões

Antes de concluir produção, fechar estas escolhas:

1. configurar o endpoint público `mcp.browser.lspr.dev/mcp` com OAuth;
2. escolher Secure MCP Tunnel como fallback de desenvolvimento ou endpoint público
   com OAuth;
3. escolher o authorization server compatível com MCP;
4. definir o domínio do portal da faculdade para `always_chromium`;
5. decidir se compras/pagamentos ficam completamente bloqueados no v1;
6. definir destino S3 e retenção dos backups do volume;
7. decidir se um único perfil Chromium é aceitável para X, Amazon e faculdade.

Minha recomendação é manter um único perfil de teste, compras bloqueadas e o
Secure MCP Tunnel apenas como fallback de desenvolvimento. O endpoint público
fica em `mcp.browser.lspr.dev` com OAuth.
