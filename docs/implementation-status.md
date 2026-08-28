# Status da implementação do gateway Go

Data da validação: 28 de agosto de 2026.

## Entregue neste milestone

O gateway Go híbrido está funcional contra o Chromium persistente e o Obscura
efêmero locais.

Componentes implementados:

- `cmd/navego`: processo HTTP, configuração, logs e graceful shutdown;
- `internal/mcpserver`: 18 tools MCP com schemas e annotations;
- `internal/browser`: conexão CDP, refs por geração, snapshots, tabs, find/wait e screenshots;
- `internal/obscura`: cliente MCP privado, contrato, allowlist, limites e parsing;
- `internal/metadata`: Open Graph e links HTML com dialer público fixado por IP;
- `internal/router`: seleção, fallback, circuit breaker e handoff Obscura -> Chromium;
- `internal/takeover`: bloqueio global enquanto o usuário controla a GUI;
- `internal/approval`: approvals aleatórios, expirando e de uso único;
- `internal/oauthresource`: discovery OIDC, JWT/JWKS, audience, subject allowlist
  e extração de scopes;
- `internal/httpserver`: Streamable HTTP, limites, headers, CSRF, Bearer local e
  metadata RFC 9728;
- `cmd/mcp-smoke`: teste real do transporte e do boundary de takeover;
- `cmd/obscura-smoke`: contract/E2E de leitura, links, PNG e PDF;
- `Dockerfile`: build multi-stage e runtime distroless/nonroot;
- `compose.go.yaml`: sidecar Go no namespace de rede do Chromium;
- `compose.dokploy.yaml`: deploy sem portas públicas e routers Traefik para GUI,
  MCP e discovery OAuth;
- `.codex/config.toml`: conexão local do Codex em `127.0.0.1:8001/mcp`.

Tools expostas:

- `browser_status`
- `browser_open`
- `browser_snapshot`
- `browser_find`
- `browser_wait`
- `browser_list_tabs`
- `browser_new_tab`
- `browser_switch_tab`
- `browser_close_tab`
- `browser_click`
- `browser_type`
- `browser_take_screenshot`
- `browser_export_pdf`
- `browser_request_human_login`
- `browser_resume_after_human`
- `browser_prepare_action`
- `browser_commit_action`
- `browser_cancel_action`

Não há tool de JavaScript arbitrário, cookie/storage, filesystem, shell, upload
ou CDP bruto. O protótipo Node e suas dependências foram removidos depois da
validação do gateway Go.

## Validação executada

Passaram:

```text
go test ./...
go test -race ./...
go vet ./...
docker build -t navego-gateway:local .
docker compose -f compose.yaml -f compose.go.yaml config
docker compose --env-file .env.production.example -f compose.dokploy.yaml config
go run ./cmd/mcp-smoke
```

O smoke real confirmou inicialização e descoberta MCP, página pública pelo
Obscura, find/wait, handoff ao Chromium pelo CDP privado, bloqueio durante
takeover, retomada, tabs e screenshot. O smoke do adapter confirmou Markdown,
links, PNG e PDF.

Os testes OAuth cobrem discovery OIDC e exigência de PKCE S256, metadata RFC
9728 nos dois caminhos, JWT assinado, issuer/audience/expiração/subject, união de
scopes, desafios por tool e publicação de `securitySchemes` no formato do
ChatGPT. O smoke final também confirmou screenshot CDP após fixar X11.

Também passaram testes reais adicionais:

1. a listagem do InfoMoney foi roteada ao Obscura e retornou `image_url`, site e
   descrição estruturados;
2. quando `browser_links` não reconheceu os cards, o fallback HTML seguro
   devolveu o link exato da notícia em `browser_find`;
3. uma falha transitória do Obscura em uma matéria caiu no Chromium, que devolveu
   os mesmos metadados do DOM;
4. abrir, alternar e fechar uma aba temporária funcionou;
5. a mesma aba e target ID sobreviveram à recriação somente do gateway Go.

O E2E real no X também passou:

1. logout protegido por `prepare/commit`;
2. pausa global para login humano pela GUI;
3. retomada sem transferir senha ou OTP pelo MCP;
4. preenchimento e verificação exata do rascunho;
5. aprovação vinculada ao texto e publicação de uso único.

O teste revelou que o X possui uma confirmação de logout em duas etapas. As
duas ficam protegidas e `logout`, `sign out` e `sair` passaram a ser classificados
como ações sensíveis.

## Decisões deste milestone

- O gateway Go usa `8001` localmente.
- O CDP permanece em loopback e não é publicado no host.
- O MCP interno do Obscura não é publicado no host.
- A imagem Obscura está fixada em `v0.2.1` pelo digest
  `sha256:e65cb455fc67543283da6901e8735c45aab5421e2ced8879b0a1fa70a4e38a2d`.
- O Chromium usa `PIXELFLUX_WAYLAND=false`: a imagem testada bloqueou
  `Page.captureScreenshot` no compositor Wayland, enquanto X11 preserva a
  captura CDP determinística.
- Somente `navigate`, `snapshot`, `markdown`, `links`, `screenshot` e `pdf`
  atravessam o adapter; tools internas perigosas não são chamáveis.
- Metadados e o fallback de links usam apenas HTTP GET público, tamanho limitado,
  redirects revalidados e um dialer que rejeita e não disca IPs privados.
- O circuit breaker abre após três falhas, espera 30 segundos e permite uma única
  tentativa em `half_open`; ambos os valores são configuráveis.
- O gateway se reconecta a uma aba existente e não cancela targets no shutdown
  normal, preservando as abas no Chromium separado.
- O gateway e o Chromium compartilham o namespace de rede.
- PocketBase não é necessário enquanto houver somente um usuário e estado
  efêmero de takeover/approval.
- Ações finais sensíveis não podem usar `browser_click`; exigem
  `prepare/commit`.
- Password inputs nunca aceitam `browser_type`.
- O gateway é um OAuth Resource Server; emissão de tokens, consentimento, PKCE
  e refresh tokens ficam em um authorization server estabelecido.
- As tools exigem scopes cumulativos `browser:read`, `browser:capture`,
  `browser:interact`, `browser:takeover` e `browser:write`.
- Como o SDK MCP Go v1.7.0 ainda não modela a extensão OpenAI
  `securitySchemes` no nível principal, um adaptador limitado a `tools/list`
  espelha o valor de `_meta` no formato exigido pelo ChatGPT. A autorização não
  depende desse adaptador e continua sendo validada no servidor.

## Limitações conhecidas

- O OAuth Resource Server está implementado, mas Auth0, CIMD e o cliente do
  ChatGPT ainda não foram configurados com credenciais reais. Ainda não há
  auditoria persistente nem multiusuário.
- O Chromium agora intercepta requests, valida DNS e bloqueia redirects e
  subrequests privadas. Ainda falta uma política de egress externa para remover
  completamente a corrida TOCTOU entre a resolução do gateway e a do Chromium.
- A detecção genérica de ações sensíveis depende do nome acessível do controle.
  Antes de produção haverá políticas e testes específicos para X e outros sites.
- Takeover e approvals são globais e ficam em memória; reiniciar o gateway os
  perde de forma segura.
- Não há ainda downloads, seleção de opções ou teclas especiais.
- O estado do circuit breaker não sobrevive ao restart do gateway; isso é
  aceitável no modo single-user e poderá ir ao PocketBase junto da auditoria.
- O Compose de produção e os routers Traefik estão versionados. O deploy no
  Dokploy, as policies do Cloudflare Access, DNS e secrets ainda precisam ser
  aplicados na infraestrutura externa.
- Os resultados de navegação ainda carregam snapshots extensos. A otimização
  de tokens foi deliberadamente movida para um milestone futuro: remover
  representações duplicadas, devolver deltas compactos depois de ações e
  limitar o snapshot aos elementos relevantes/paginados.

## Próxima sequência

1. Configurar Auth0, Cloudflare Access e Dokploy e executar o E2E OAuth remoto
   no ChatGPT.
2. Adicionar controle externo de egress para fechar a janela de DNS rebinding.
3. Adicionar downloads/select/press-key conforme os primeiros sites exigirem.
4. Adicionar auditoria persistente e vincular approvals à identidade OAuth;
   introduzir PocketBase apenas quando esse estado realmente precisar persistir.
5. Otimizar snapshots e respostas MCP para reduzir tokens antes da validação
   de custo e do uso recorrente em produção.
