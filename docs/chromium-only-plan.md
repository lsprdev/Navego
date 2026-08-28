# Plano Chromium-only do Navego

Data: 28 de agosto de 2026.

## Objetivo

Permitir que ChatGPT/Codex controle um Chromium hospedado pelo usuário para:

- navegar e extrair conteúdo público;
- continuar em sessões autenticadas persistentes;
- entregar a GUI ao usuário somente quando credenciais, MFA ou CAPTCHA forem
  necessários;
- retomar automaticamente após o usuário responder `pronto`;
- capturar e enviar screenshots e PDFs;
- executar publicações, compras, envios e exclusões somente após confirmação;
- abrir, quando solicitado, uma janela privada que não compartilhe a sessão.

## Arquitetura escolhida

```text
                  ChatGPT / Codex
                         |
                         v
                MCP Streamable HTTP
                         |
                         v
                Gateway Navego (Go)
                  |             |
                  | CDP         | estado efêmero
                  v             v
        Chromium persistente   takeover/approval
             /          \
    perfil normal     BrowserContexts privados
         |                  |
  volume /config      descartados ao fechar
```

Há uma única engine. Isso elimina routing, fallback e handoff entre navegadores,
e garante que leitura, screenshot e interação enxerguem exatamente o mesmo DOM.

## Sessões

### Persistente

É o padrão. `browser_open` navega a aba ativa e `browser_new_tab` abre outra aba
no perfil salvo. É o modo apropriado para X, Amazon, faculdade e demais sites
nos quais a sessão deve sobreviver ao restart do gateway ou do container.

### Privada

Quando o usuário pedir “anonimamente”, “em modo privado”, “isolado” ou
“efêmero”, o cliente usa `browser_new_private_tab`.

O gateway cria `Target.createBrowserContext(disposeOnDetach=true)` e uma página
visível nesse contexto. O contexto possui cookies, cache e storage próprios. A
aba criadora é sua proprietária; fechá-la chama `Target.disposeBrowserContext`.

Não há promessa de isolamento de processo/container. Se for necessária uma
fronteira forte para conteúdo não confiável, o passo futuro é criar um container
Chromium descartável separado.

## Contrato MCP

O conjunto público permanece pequeno e orientado a intenção:

- leitura: status, open, snapshot, find, wait;
- tabs: list, new persistent, new private, switch, close;
- interação: click, type não secreto, hover, scroll, teclas permitidas e select;
- artefatos: screenshot e PDF;
- humano: request login e resume;
- efeitos externos: prepare, commit e cancel.

Não expor JavaScript arbitrário, cookies, storage, filesystem, shell ou CDP.

## Fluxos principais

### Login humano

1. Gateway abre a página de login.
2. O modelo tenta apenas etapas não secretas.
3. Ao encontrar senha/MFA/CAPTCHA, chama `browser_request_human_login`.
4. O estado bloqueia todas as demais tools de browser.
5. O usuário abre a GUI protegida, conclui a autenticação e diz `pronto`.
6. `browser_resume_after_human` devolve o controle e um snapshot novo.

O modelo não deve usar takeover como escape para menus difíceis.

### Login salvo opcional

1. O gateway identifica o formulário e a origem HTTPS exata.
2. Uma conta configurada em Docker secrets é selecionada somente pelo servidor.
3. `browser_prepare_saved_login` mostra label e origem, sem usuário ou senha.
4. O usuário confirma explicitamente.
5. `browser_commit_saved_login` lê os secrets, preenche e envia sem snapshot
   intermediário; o approval é consumido uma vez.
6. MFA, passkey, OTP e CAPTCHA continuam no takeover humano.

O modelo passa a controlar a sessão autenticada, mesmo sem conhecer a senha.
Por isso, o recurso é reservado a contas dedicadas e de baixo privilégio.

### Ação com efeito externo

1. O modelo preenche e revisa os campos.
2. O clique final é classificado como sensível.
3. `browser_prepare_action` vincula URL, target, geração e valores dos campos.
4. O cliente mostra o efeito exato e espera confirmação explícita.
5. `browser_commit_action` consome um approval de uso único.

### Screenshot

1. Navegar até a tela desejada.
2. Se houver login, executar o takeover humano.
3. Continuar a navegação após `pronto`.
4. Chamar `browser_take_screenshot`.
5. Retornar `ImageContent` e o card MCP Apps inline.

## Deploy

Serviços:

- `navego-browser`: Chromium + GUI + volume persistente;
- `navego-gateway`: binário Go no namespace de rede do browser;
- `ngrok`: somente no overlay temporário de desenvolvimento.

Produção:

- `browser.lspr.dev` -> GUI Chromium, protegida por Cloudflare Access;
- `mcp.browser.lspr.dev/mcp` -> gateway, protegido por OAuth;
- Traefik alcança `3000` e `8001` pela `dokploy-network`;
- CDP `9222` continua em loopback e não é roteado;
- nenhum container Obscura ou rede interna adicional é necessário.

O volume `navego-browser-data` nunca deve ser removido em deploys normais.

## Milestones

### M1 — simplificação Chromium-only

- remover Obscura, router híbrido e configuração associada;
- remover prefixos `ob/ch` e a tool de seleção de backend;
- manter todas as leituras e interações no Manager Chromium;
- atualizar Compose, testes e documentação.

### M2 — contexto privado

- criar BrowserContext efêmero visível;
- marcar tabs privadas;
- permitir switch e screenshot normalmente;
- descartar o contexto ao fechar a aba proprietária;
- comprovar que cookies do perfil persistente não atravessam.

### M3 — interação robusta

- `browser_hover` implementado;
- `browser_press_key` com allowlist implementado;
- `browser_select_option` para selects nativos implementado;
- `browser_scroll` por pixels ou ref implementado;
- broker opcional de login salvo por origem implementado;
- melhorar suporte futuro a popovers em portais DOM, iframes e comboboxes
  customizados;
- testes reais em X, Amazon e SIGAA.

### M4 — produção

- Auth0/OAuth E2E no ChatGPT;
- Cloudflare Access para GUI;
- deploy Dokploy e backup do volume;
- rate limiting, observabilidade e política de egress;
- auditoria vinculada ao subject OAuth.

### M5 — eficiência

- snapshots incrementais;
- paginação e recortes por região/ref;
- deduplicação entre conteúdo textual e estruturado;
- métricas de tempo, erros e tokens por workflow.

## Critérios de aceitação

- uma sessão autenticada persiste após recriar apenas o gateway;
- nenhuma credencial passa por MCP ou chat;
- takeover bloqueia concorrência até `resume`;
- janela privada inicia sem cookies do perfil e é descartada ao fechar;
- screenshots aparecem inline no ChatGPT;
- ações externas exigem approval explícito e não podem ser repetidas;
- CDP e GUI não ficam publicamente acessíveis sem suas proteções;
- `go test`, race, vet, builds e validação dos dois Composes passam.

## Riscos remanescentes

- DOMs mudam e podem exigir melhorias genéricas nos elementos;
- páginas podem detectar automação ou impor CAPTCHA;
- OAuth e Cloudflare Access precisam ser configurados corretamente;
- BrowserContext privado não é isolamento contra comprometimento do processo;
- um único browser implica controle global e não suporta múltiplos usuários
  simultâneos sem um futuro session manager.
