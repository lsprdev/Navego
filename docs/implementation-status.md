# Status da implementação do gateway Go

Data da validação: 28 de agosto de 2026.

## Estado atual

O Navego usa uma arquitetura Chromium-only. O antigo adapter Obscura, o router
híbrido e os prefixos `ob:`/`ch:` foram removidos. Leitura pública, navegação
autenticada, interação, screenshot e PDF usam o mesmo Chromium controlado pelo
gateway Go.

Componentes:

- `cmd/navego`: processo HTTP, configuração, logs e graceful shutdown;
- `internal/mcpserver`: 25 tools MCP com schemas, annotations e instruções;
- `internal/browser`: CDP, política de URL, snapshots, refs, metadados, tabs,
  BrowserContexts privados, find/wait, screenshot e PDF;
- `internal/credentials` e `internal/loginapproval`: manifesto de contas,
  confinamento de Docker secrets e approvals de login por origem;
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
- `browser_hover`
- `browser_press_key`
- `browser_select_option`
- `browser_scroll`
- `browser_take_screenshot`
- `browser_export_pdf`
- `browser_request_human_login`
- `browser_resume_after_human`
- `browser_prepare_saved_login`
- `browser_commit_saved_login`
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
- Logins salvos opcionais são vinculados à origem HTTPS exata, lidos somente
  depois de confirmação e nunca retornados por MCP; valores protegidos que
  reapareçam no DOM são redigidos dos snapshots.
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
5. o smoke completo confirmou as 19 tools então existentes, takeover, resume,
   tabs e screenshot;
6. somente o gateway foi recriado; o ID do Chromium e o volume
   `navego-browser-data` permaneceram iguais.

Na extensão atual, o smoke local confirmou as 25 tools. `browser_scroll` e
`browser_press_key` passaram no fluxo principal; `browser_select_option` mudou
um `<select>` público de teste de `dd3` para `dd5`, e `browser_hover` moveu o
cursor sem clicar. O broker desativado reconheceu um formulário real e recusou
o prepare antes de preencher qualquer campo. O commit de login permanece
coberto por teste isolado com secrets descartáveis; o E2E real deve usar apenas
uma conta de teste criada para esse fim.

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
2. Fazer um E2E local do broker apenas com uma conta descartável de teste.
3. Configurar Auth0, Cloudflare Access e Dokploy com secrets reais.
4. Adicionar egress externo contra DNS rebinding.
5. Implementar iframes, comboboxes customizados e menus que usam portais DOM.
6. Adicionar downloads e auditoria persistente conforme a necessidade.
7. Reduzir tokens com snapshots incrementais e respostas mais compactas.
