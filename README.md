# Navego

Gateway MCP em Go para controlar um Chromium visível e persistente. O Navego
permite ao ChatGPT ou a outro cliente MCP navegar, interagir, solicitar login
humano, capturar screenshots/PDFs e executar ações externas com confirmação em
duas etapas.

## Arquitetura

```text
Codex / ChatGPT
       |
       v
Gateway MCP Go ---- CDP em loopback ---- Chromium + GUI
                                             |
                                      volume /config
```

O gateway compartilha o namespace de rede do Chromium. Assim, o CDP permanece
em `127.0.0.1:9222`, sem exposição no host ou na Internet. O perfil persistente
fica no volume `navego-browser-data`.

Existem dois modos de navegação dentro do mesmo Chromium:

- abas persistentes compartilham cookies, storage e sessões do perfil;
- `browser_new_private_tab` cria um BrowserContext efêmero e isolado, equivalente
  ao uso prático de uma janela anônima. Ao fechar a aba proprietária, todo o
  contexto privado é descartado.

Uma janela privada isola cookies e storage, mas continua no mesmo processo e
container. Ela não é uma fronteira forte de segurança contra código hostil.

## O que funciona

- MCP Streamable HTTP com o SDK Go oficial;
- navegação, snapshots compactos, metadados Open Graph, busca e espera;
- clique e digitação de texto não secreto;
- abas persistentes e contextos privados efêmeros;
- screenshot inline via MCP Apps UI e exportação de PDF;
- bloqueio de senha, IPs locais/reservados, DNS misto, redirects e subrequests;
- takeover humano para senha, OTP, passkey, MFA ou CAPTCHA;
- `prepare -> confirmação -> commit` para publicar, enviar, comprar ou excluir;
- OAuth 2.1 para produção, com RFC 9728, JWT/JWKS, audience, subject allowlist e
  scopes progressivos por tool;
- Compose local, ngrok de teste e Compose Dokploy/Traefik de produção.

Não há tool de JavaScript arbitrário, cookies/storage, shell, filesystem, upload
ou CDP bruto.

## Subir localmente

Pré-requisitos: Docker Compose v2 e Go 1.26 para os testes locais.

```bash
cp .env.example .env
docker compose -f compose.yaml -f compose.go.yaml up --build -d
docker compose -f compose.yaml -f compose.go.yaml ps
```

Endpoints:

- MCP: `http://127.0.0.1:8001/mcp`
- health: `http://127.0.0.1:8001/healthz`
- Chromium GUI HTTPS: `https://127.0.0.1:3001`
- Chromium GUI HTTP: `http://127.0.0.1:3000`

O certificado HTTPS local é autoassinado. Não remova o volume
`navego-browser-data` ao recriar os containers; ele contém as sessões dos sites.

## Usar pelo Codex

O projeto inclui [`.codex/config.toml`](.codex/config.toml), apontando para o MCP
local. Depois de subir os containers:

1. reinicie o Codex para recarregar a configuração;
2. use `/mcp` para verificar `navego`;
3. peça algo inofensivo, como “abra example.com e tire um print”.

Para uma navegação isolada, peça naturalmente “abra este site anonimamente”. O
cliente deve usar `browser_new_private_tab`; não são necessários os antigos
prefixos de seleção de backend.

## Login humano

Quando aparecer senha, passkey, OTP, CAPTCHA ou MFA:

1. o cliente chama `browser_request_human_login`;
2. todas as operações de browser ficam bloqueadas;
3. abra `https://127.0.0.1:3001` e autentique diretamente no Chromium;
4. volte ao chat e responda `pronto`;
5. o cliente chama `browser_resume_after_human` e continua na mesma aba.

Nunca envie credenciais pelo chat. O takeover é reservado a autenticação; menus
e elementos difíceis continuam sendo controlados pelas tools do navegador.
Login concluído não aprova publicação, compra, exclusão ou outra ação externa.

## Publicar no X

O fluxo esperado é:

1. abrir `https://x.com` em uma aba persistente;
2. solicitar login humano, se necessário;
3. preencher o rascunho sem publicar;
4. chamar `browser_prepare_action` no botão final;
5. mostrar texto, destino e campos ao usuário;
6. aguardar confirmação explícita;
7. chamar `browser_commit_action` uma única vez.

## Validar

```bash
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/mcp-smoke
```

O smoke principal testa transporte Streamable HTTP, navegação, busca, espera,
takeover, retomada, abas e screenshot. O contexto privado também é validado por
um smoke CDP real durante o desenvolvimento.

## ngrok para teste no ChatGPT

```bash
docker compose \
  -f compose.yaml \
  -f compose.go.yaml \
  -f compose.ngrok.yaml \
  up --build -d
```

Descubra a URL em `http://127.0.0.1:4040/api/tunnels` e acrescente `/mcp`. Para
OAuth, a URL pública precisa permanecer estável e ser usada tanto em
`MCP_PUBLIC_URL` quanto em `MCP_OAUTH_AUDIENCE`.

Não deixe um túnel sem autenticação aberto além do teste e não use contas
sensíveis nessa configuração temporária.

## Deploy no Dokploy

O deploy usa [`compose.dokploy.yaml`](compose.dokploy.yaml):

- `https://browser.lspr.dev` — GUI protegida pelo Cloudflare Access;
- `https://mcp.browser.lspr.dev/mcp` — MCP protegido por OAuth.

Variáveis e instruções estão em [`.env.production.example`](.env.production.example)
e [`docs/production-deployment.md`](docs/production-deployment.md).

## Limites atuais

- um navegador e um usuário por instância;
- takeover e approvals ficam em memória;
- ainda faltam downloads, hover, seleção de opções e teclas especiais;
- a produção ainda deve adicionar uma política de egress para fechar totalmente
  a janela TOCTOU de DNS rebinding;
- OAuth, Cloudflare Access e Dokploy precisam ser validados com os secrets reais;
- snapshots ainda podem ser otimizados para reduzir tokens.

Veja o plano atual em [`docs/chromium-only-plan.md`](docs/chromium-only-plan.md)
e o inventário em [`docs/implementation-status.md`](docs/implementation-status.md).
