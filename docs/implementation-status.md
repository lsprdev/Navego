# Status da implementação do gateway Go

Data da validação: 28 de agosto de 2026.

## Estado atual

O Navego usa uma arquitetura Chromium-only. O antigo adapter Obscura, o router
híbrido e os prefixos `ob:`/`ch:` foram removidos. Leitura pública, navegação
autenticada, interação, screenshot e PDF usam o mesmo Chromium controlado pelo
gateway Go.

Componentes:

- `cmd/navego`: processo HTTP, configuração, logs e graceful shutdown;
- `internal/mcpserver`: 19 tools MCP com schemas, annotations e instruções;
- `internal/browser`: CDP, política de URL, snapshots, refs, metadados, tabs,
  BrowserContexts privados, find/wait, screenshot e PDF;
- `internal/takeover`: bloqueio global durante o controle humano;
- `internal/approval`: approvals aleatórios, expirando e de uso único;
- `internal/oauthresource`: discovery OIDC, JWT/JWKS, audience, subject allowlist
  e scopes;
- `internal/httpserver`: Streamable HTTP, limites, headers, CSRF, Bearer local e
  metadata RFC 9728;
- `cmd/mcp-smoke`: teste E2E do transporte e dos boundaries principais;
- `compose.go.yaml`: gateway no namespace de rede do Chromium;
- `compose.dokploy.yaml`: deploy Traefik sem CDP ou portas diretas públicas.

Tools expostas:

- `browser_status`
- `browser_open`
- `browser_snapshot`
- `browser_find`
- `browser_wait`
- `browser_list_tabs`
- `browser_new_tab`
- `browser_new_private_tab`
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

## Abas persistentes e privadas

`browser_new_tab` cria uma aba que compartilha o perfil persistente, incluindo
cookies e sessões autenticadas.

`browser_new_private_tab` cria um novo BrowserContext CDP com uma janela visível:

- cookies, cache e storage ficam isolados do perfil persistente;
- `browser_list_tabs` marca suas páginas com `private: true`;
- a aba inicial é proprietária do contexto;
- ao fechar a proprietária, o gateway descarta todo o BrowserContext;
- após o descarte, uma aba persistente volta a ser a ativa.

Essa separação equivale ao uso prático de uma janela anônima, mas permanece no
mesmo processo/container e não deve ser tratada como sandbox forte.

## Segurança e fluxo humano

- CDP permanece em loopback e não é publicado no host.
- O perfil persistente fica no volume `navego-browser-data`.
- Password inputs nunca aceitam `browser_type`.
- `browser_request_human_login` é usado somente para senha, MFA, passkey, OTP ou
  CAPTCHA; menus difíceis continuam automatizados.
- Ações finais sensíveis não podem usar `browser_click`; exigem
  `prepare/commit` após confirmação explícita.
- URLs, redirects e subrequests são validados contra destinos privados e
  reservados.
- O gateway é OAuth Resource Server; emissão, consentimento, PKCE e refresh
  token pertencem a um authorization server estabelecido.
- Scopes cumulativos: `browser:read`, `browser:capture`, `browser:interact`,
  `browser:takeover` e `browser:write`.

## Validações

Suíte esperada:

```text
go test ./...
go test -race ./...
go vet ./...
docker build -t navego-gateway:local .
docker compose -f compose.yaml -f compose.go.yaml config
docker compose --env-file .env.production.example -f compose.dokploy.yaml config
go run ./cmd/mcp-smoke
```

O E2E real anterior no X validou logout protegido, login humano sem transferência
de credenciais, preenchimento verificado e publicação após approval de uso único.
O teste de screenshot no ChatGPT também validou o card MCP Apps inline.

O smoke Chromium-only deste milestone também passou:

1. as duas abas existentes do perfil foram preservadas e não foram marcadas como
   privadas;
2. `browser_new_private_tab` abriu `example.com` em uma janela marcada com
   `private: true`;
3. um cookie `NavegoIsolation=profile` ficou visível no perfil persistente e o
   mesmo endpoint retornou `{}` no contexto privado;
4. ao fechar a aba proprietária, o contexto privado desapareceu e o gateway
   voltou a uma aba persistente;
5. o smoke completo confirmou as 19 tools, takeover, resume, tabs e screenshot;
6. somente o gateway foi recriado; o ID do Chromium e o volume
   `navego-browser-data` permaneceram iguais.

## Decisões

- Um único Chromium reduz código, memória operacional, ambiguidades de routing e
  divergências de renderização.
- Páginas públicas que pedirem isolamento usam BrowserContext privado em vez de
  outro engine.
- O Chromium usa `PIXELFLUX_WAYLAND=false`; a imagem testada bloqueou
  `Page.captureScreenshot` no Wayland, enquanto X11 preservou a captura CDP.
- Gateway e Chromium compartilham namespace de rede para manter CDP em loopback.
- PocketBase não é necessário enquanto houver um usuário e estado efêmero.
- O gateway não cancela targets persistentes no shutdown normal, preservando
  abas quando apenas o sidecar Go é recriado.

## Limitações e próximos passos

1. Validar novamente o connector do ChatGPT após atualizar o schema das tools.
2. Configurar Auth0, Cloudflare Access e Dokploy com secrets reais.
3. Adicionar egress externo contra DNS rebinding.
4. Implementar hover, teclas especiais, select/combobox e menus complexos.
5. Adicionar downloads e auditoria persistente conforme a necessidade.
6. Reduzir tokens com snapshots incrementais e respostas mais compactas.
