# Navego

Gateway MCP híbrido em Go: Obscura para leitura de páginas públicas e Chromium
persistente para autenticação e interações, com login humano e confirmação em
duas etapas para ações externas.

O fluxo híbrido local, o OAuth Resource Server e o Compose de produção estão
implementados. O próximo passo operacional é configurar Auth0, Cloudflare
Access e Dokploy com os secrets reais e executar o E2E remoto no ChatGPT.

## Arquitetura local atual

```text
Codex / cliente MCP
        |
        v
http://127.0.0.1:8001/mcp
        |
        v
Gateway MCP Go ---- CDP em loopback ---- Chromium + GUI
        |                                    |
        +---- MCP privado ---- Obscura       +---- volume persistente /config
```

O gateway e o Chromium compartilham o namespace de rede, mantendo o CDP em
`127.0.0.1:9222`. O Obscura fica na rede interna do Compose, sem porta publicada.
Somente o gateway conhece seu MCP e usa uma allowlist fixa de seis tools.

## O que já funciona

- MCP Streamable HTTP com o SDK Go oficial;
- conexão persistente ao Chromium com `chromedp`;
- navegação, snapshot compacto, clique e digitação não secreta;
- metadados Open Graph estruturados, incluindo descrição e imagem destacada;
- `browser_find` compacto com refs/links e `browser_wait` com timeout rígido;
- listar, abrir, alternar e fechar abas persistentes do Chromium;
- screenshot MCP como bloco de imagem;
- PDF MCP como recurso incorporado;
- Obscura `v0.2.1` fixado por digest para leitura, Markdown, screenshot e PDF;
- roteamento público para Obscura, domínios autenticados para Chromium e fallback;
- circuit breaker do Obscura com fallback, cooldown e estado observável;
- bloqueio de senha, IPs locais/reservados, DNS misto, redirects e subrequests;
- takeover humano: a automação para até o usuário responder `pronto`;
- `prepare -> confirmação -> commit` para publicar, enviar, comprar ou excluir;
- approval de uso único, expiração curta e validação da página/campos;
- Bearer token opcional para clientes locais que aceitam headers customizados;
- OAuth 2.1 para produção, com discovery RFC 9728, JWT/JWKS, audience, subject
  allowlist e scopes progressivos por tool;
- desafios de step-up OAuth em `_meta["mcp/www_authenticate"]`;
- imagem Go distroless/nonroot e filesystem read-only no Compose;
- Compose do Dokploy com routers Traefik separados para GUI, MCP e discovery;
- testes unitários e smoke test ponta a ponta.

## Subir localmente

Pré-requisitos: Docker Desktop/Engine com Compose v2 e Go 1.26 para rodar os
testes fora do container.

```bash
cp .env.example .env
docker compose -f compose.yaml -f compose.go.yaml up --build -d
docker compose -f compose.yaml -f compose.go.yaml ps
```

Endpoints:

- MCP Go: `http://127.0.0.1:8001/mcp`
- health do gateway: `http://127.0.0.1:8001/healthz`
- Chromium GUI HTTPS: `https://127.0.0.1:3001`
- Chromium GUI HTTP: `http://127.0.0.1:3000`

O certificado HTTPS local da GUI é autoassinado. O perfil, cookies e sessões
ficam no volume `navego-browser-data` e sobrevivem à recriação dos
containers. O Obscura é efêmero e não recebe cookies do Chromium.

## Usar diretamente pelo Codex

O projeto inclui [`.codex/config.toml`](.codex/config.toml), apontando para o MCP
Go local. Depois de subir os containers:

1. reinicie o Codex para carregar a configuração do projeto;
2. use `/mcp` para verificar `navego`;
3. peça uma ação inofensiva, por exemplo: “abra example.com e tire um print”.

As tools de escrita usam aprovação `writes` no Codex. A configuração e o formato
atual de servidores MCP estão documentados em
[OpenAI — MCP no Codex](https://developers.openai.com/codex/mcp).

## Login humano

Quando aparecer senha, passkey, OTP, CAPTCHA ou 2FA:

1. o cliente chama `browser_request_human_login`;
2. todas as operações de browser ficam bloqueadas;
3. abra `https://127.0.0.1:3001` e autentique diretamente no Chromium;
4. volte ao chat e responda `pronto`;
5. o cliente chama `browser_resume_after_human` e continua na mesma sessão.

Nunca envie senha ou código pelo chat. Login concluído também não significa que
uma publicação, compra ou exclusão foi aprovada.

## Publicar no X

O fluxo esperado é:

1. abrir `https://x.com`;
2. pausar para login humano, se necessário;
3. preencher o compositor sem publicar;
4. chamar `browser_prepare_action` no botão final;
5. mostrar texto, destino e campos ao usuário;
6. aguardar uma confirmação explícita;
7. chamar `browser_commit_action` uma única vez.

O primeiro teste deve usar uma mensagem sem consequência relevante. Mudanças do
DOM do X podem exigir ajustes na extração de elementos, embora o gateway não use
seletores específicos do site.

## Validar

```bash
go test ./...
go vet ./...
go run ./cmd/mcp-smoke
OBSCURA_MCP_ENDPOINT=http://127.0.0.1:18080/mcp go run ./cmd/obscura-smoke
```

O smoke principal conecta por Streamable HTTP, abre `example.com` no Obscura,
valida busca e espera compactas, faz o handoff ao Chromium, valida o bloqueio de
takeover, retoma, lista abas e captura um screenshot. O smoke isolado do adapter
exige um Obscura publicado temporariamente
em `127.0.0.1:18080`; o Compose normal não publica essa porta.

## ngrok para o futuro teste no ChatGPT

O overlay encaminha o ngrok ao gateway Go em `8001`:

```bash
docker compose \
  -f compose.yaml \
  -f compose.go.yaml \
  -f compose.ngrok.yaml \
  up --build -d
```

Descubra a URL em `http://127.0.0.1:4040/api/tunnels` e acrescente `/mcp`.
Para testar OAuth pelo ngrok, a URL pública precisa ser estável durante todo o
fluxo. Deixe `NAVEGO_API_KEY` vazio e configure `MCP_OAUTH_ENABLED=true`,
`MCP_PUBLIC_URL=<url-ngrok>/mcp`, o mesmo valor em `MCP_OAUTH_AUDIENCE`, além de
`MCP_OAUTH_ISSUER` e `MCP_OAUTH_ALLOWED_SUBJECTS`.

Não mantenha um túnel sem autenticação aberto e não use contas sensíveis nesse
estágio. O plano de produção está em
[`docs/hybrid-go-deployment-plan.md`](docs/hybrid-go-deployment-plan.md).

## Deploy no Dokploy

O deploy usa [`compose.dokploy.yaml`](compose.dokploy.yaml) e as variáveis de
[`.env.production.example`](.env.production.example). O passo a passo para
Auth0, Cloudflare Access, Dokploy, validação e rollback está em
[`docs/production-deployment.md`](docs/production-deployment.md).

## Limites do milestone atual

- o OAuth está implementado no gateway, mas o Auth0 e o cliente do ChatGPT ainda
  precisam ser configurados e validados com tokens reais;
- takeover e approvals ficam em memória e há apenas um navegador/usuário;
- a interceptação CDP valida DNS, redirects e subrequests, mas a produção ainda
  precisará de controle de egress para eliminar a janela TOCTOU de DNS rebinding;
- o circuit breaker é persistente entre chamadas, mas volta ao estado inicial
  quando o gateway reinicia;
- ainda faltam downloads, seleção de opções, teclas especiais e auditoria;
- o Compose do Dokploy/Traefik está pronto, mas o deploy e as políticas do
  Cloudflare Access dependem da infraestrutura externa.

Veja o inventário detalhado em
[`docs/implementation-status.md`](docs/implementation-status.md).
