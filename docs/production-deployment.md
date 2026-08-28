# Deploy de produção: Dokploy, Auth0 e Cloudflare Access

Data de revisão: 28 de agosto de 2026.

Este runbook publica:

| URL | Destino | Autorização |
| --- | --- | --- |
| `https://browser.lspr.dev/` | GUI do Chromium | Cloudflare Access |
| `https://mcp.browser.lspr.dev/mcp` | Gateway MCP Go | OAuth 2.1 via Auth0 |
| `https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource` | metadata RFC 9728 | pública |
| `https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource/mcp` | metadata específica do endpoint | pública |

O arquivo de deploy é [`compose.dokploy.yaml`](../compose.dokploy.yaml). Ele não
publica portas no host. O Traefik alcança os ports `3000` e `8001` no namespace
de rede do `navego-browser`; o CDP continua privado em loopback.

## 1. Configurar o authorization server

Use um provedor OAuth/OIDC estabelecido. O primeiro deploy foi desenhado para
Auth0 porque ele suporta as partes atuais do MCP, incluindo discovery, PKCE,
Client ID Metadata Documents (CIMD) e o parâmetro `resource`.

No Auth0:

1. Crie uma API/resource server com identifier exato
   `https://mcp.browser.lspr.dev/mcp` e tokens JWT assinados assimetricamente.
2. Cadastre estas permissions:
   - `browser:read`
   - `browser:capture`
   - `browser:interact`
   - `browser:takeover`
   - `browser:write`
3. Em tenant settings, habilite **Resource Parameter Compatibility Profile**.
   Sem isso, o `resource` obrigatório enviado pelo cliente MCP pode não virar o
   audience correto do access token.
4. Habilite suporte a CIMD. Para a conexão do ChatGPT, importe o Client ID
   Metadata Document exato mostrado pelo criador do plugin. Com issuer
   identification habilitado, o documento estável do ChatGPT é normalmente
   `https://chatgpt.com/oauth/client.json`; confirme o valor mostrado na UI.
5. Permita `authorization_code` com PKCE S256 e `refresh_token`, usando rotação
   de refresh token para o cliente público.
6. Restrinja o login/consentimento ao proprietário no próprio Auth0. Copie o
   `user_id` exato do proprietário para `MCP_OAUTH_ALLOWED_SUBJECTS`; o gateway
   aplica essa segunda barreira mesmo se o tenant aceitar outro login.

O gateway busca o discovery document ao iniciar e recusa subir se o issuer não
anunciar `S256`. Em cada request ele valida assinatura via JWKS, issuer,
audience, expiração e subject. O audience precisa ser exatamente
`https://mcp.browser.lspr.dev/mcp`.

Referências:

- [OpenAI: autenticação de plugins MCP](https://developers.openai.com/plugins/build/auth)
- [MCP: autorização](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
- [Auth0: Auth for MCP](https://auth0.com/ai/docs/mcp/intro/overview)
- [Auth0: Resource Parameter Compatibility Profile](https://auth0.com/ai/docs/mcp/guides/resource-param-compatibility-profile)

## 2. Configurar Cloudflare Access

Crie uma aplicação self-hosted para `browser.lspr.dev` sem path:

- action `Allow`;
- include somente o e-mail exato do proprietário;
- exija MFA no IdP ou independent MFA;
- use uma duração de sessão compatível com o uso pessoal.

Não inclua `mcp.browser.lspr.dev` nessa aplicação do Cloudflare Access. O
ChatGPT não apresenta a sessão do Access nas chamadas MCP; esse host é protegido
pelo OAuth do Auth0 e `/mcp` responde `401` até receber um JWT válido. Os dois
documentos `/.well-known` são públicos por definição. O DNS do subdomínio MCP
pode continuar proxied pela Cloudflare, desde que regras de bot/WAF não exibam
challenges interativos ao cliente MCP.

Não publique `/healthz`, CDP ou qualquer rota futura de controle. Como GUI e MCP
usam hosts separados, não são necessárias policies `Bypass` por path.

Referências:

- [Cloudflare: application paths](https://developers.cloudflare.com/cloudflare-one/access-controls/policies/app-paths/)
- [Cloudflare: bypass de endpoint público](https://developers.cloudflare.com/cloudflare-one/access-controls/policies/common-policies/#bypass-a-public-endpoint)

## 3. Criar o Compose no Dokploy

1. Crie um serviço do tipo Docker Compose apontando para este repositório.
2. Use `./compose.dokploy.yaml` como Compose Path.
3. Não crie entradas adicionais na aba Domains: as duas rotas e prioridades já
   estão nas labels Traefik do Compose.
4. Copie os valores de [`.env.production.example`](../.env.production.example)
   para o Environment do Dokploy e substitua os placeholders.
5. Confirme que a network externa `dokploy-network` existe. A instalação oficial
   do Dokploy já a cria.
6. Faça deploy e confirme que os dois containers iniciaram.

Se habilitar logins salvos, monte manifesto, usuário e senha como Docker secrets
somente no `navego-gateway`, sob `/run/secrets`. Não use variáveis de ambiente
para os valores e não monte os secrets no Chromium. O procedimento e o modelo
de ameaça estão em [saved-login-broker.md](./saved-login-broker.md).

O volume persistente tem nome fixo `navego-browser-data`. Inclua-o no
backup do servidor. Ele contém cookies, sessões e o perfil do Chromium e deve ser
tratado como dado sensível.

A imagem base do Chromium foi fixada na versão testada
`version-be140933@sha256:4c7b9086...cee21d`. Atualizações devem passar pelos
smokes antes de trocar o pin.
`PIXELFLUX_WAYLAND=false` também é intencional: no teste local desta versão, o
CDP `Page.captureScreenshot` ficou bloqueado no compositor Wayland. O fallback
X11 eliminou a incompatibilidade e deve continuar até uma atualização passar
pelo smoke de screenshot.

O gateway compartilha o namespace de rede do Chromium para manter o CDP em
loopback. Uma recriação normal do projeto no Dokploy recria os dois juntos. Se
somente `navego-browser` for reiniciado manualmente, recrie também
`navego-gateway`; caso contrário, o gateway pode continuar preso ao namespace
antigo.

Referência: [Dokploy: Docker Compose com Traefik](https://docs.dokploy.com/docs/core/docker-compose/example).

## 4. Validar antes de conectar o ChatGPT

Execute de uma máquina externa:

```bash
curl -i https://mcp.browser.lspr.dev/mcp
curl -sS https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource
curl -sS https://mcp.browser.lspr.dev/.well-known/oauth-protected-resource/mcp
curl -sS "$MCP_OAUTH_ISSUER/.well-known/openid-configuration"
```

Resultados esperados:

- `/mcp` sem token: `401` e `WWW-Authenticate` apontando para resource metadata,
  com `scope="browser:read"`;
- ambos os documentos RFC 9728: `200`, resource exato e os cinco scopes;
- issuer metadata: issuer exato, JWKS e `code_challenge_methods_supported`
  contendo `S256`;
- `/`: tela de login do Cloudflare Access para uma sessão não autenticada;
- portas `3000`, `3001`, `8001` e `9222`: inacessíveis diretamente pela
  Internet.

Depois adicione `https://mcp.browser.lspr.dev/mcp` no criador de plugins do ChatGPT,
selecione OAuth e conclua o login. Teste na ordem:

1. `browser_status` e uma leitura pública;
2. screenshot, provocando o scope `browser:capture`;
3. click/digitação reversível, com `browser:interact`;
4. takeover humano, com `browser:takeover`;
5. `prepare -> confirmação -> commit`, com `browser:write`.

O ChatGPT deve receber um novo desafio `_meta["mcp/www_authenticate"]` quando uma
tool exigir um scope ainda não concedido.

Na inspeção de `tools/list`, confirme também que cada tool contém
`securitySchemes` no nível principal e o espelho em `_meta`. O segundo formato
mantém compatibilidade com o SDK Go atual; o primeiro é o formato usado pela UI
de autenticação do ChatGPT.

## 5. Rollback

- Remova/desabilite temporariamente os routers Traefik ou proxie o DNS para
  interromper o acesso público.
- Reverta o deploy do gateway sem remover o volume do Chromium.
- Revogue a aplicação/consentimento no Auth0 para invalidar a conexão do
  ChatGPT.
- Se houver suspeita de exposição do perfil, encerre sessões nos sites e rotacione
  credenciais; trocar apenas o JWT do MCP não invalida cookies do Chromium.
