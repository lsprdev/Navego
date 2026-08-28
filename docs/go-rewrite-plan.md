# Planejamento da reescrita em Go

Status: o gateway mínimo descrito neste plano foi implementado. O inventário e
as diferenças para o desenho completo estão em
[status da implementação](./implementation-status.md).

Este documento complementa o [planejamento de arquitetura e segurança](architecture-plan.md).

Evolução deste desenho: [arquitetura híbrida em Go e deploy](./hybrid-go-deployment-plan.md).

## Resumo da decisão

A reescrita é viável e recomendada se o objetivo for manter o código da
aplicação em Go, reduzir dependências e controlar exatamente quais capacidades o
ChatGPT recebe.

O desenho sugerido é:

- SDK Go oficial do MCP para protocolo e Streamable HTTP;
- `chromedp` para conectar ao Chromium persistente por CDP;
- ferramentas MCP próprias, pequenas e orientadas a objetivos;
- um binário Go estático para MCP, políticas, takeover, screenshots e auditoria;
- Chromium e GUI continuando em um contêiner especializado;
- PocketBase somente quando existir uma necessidade concreta de persistência
  estruturada, administração ou múltiplos usuários.

A aplicação ficará menor, mas o deployment completo não diminuirá na mesma
proporção. Chromium, codecs, fontes, Wayland/X11 e a interface de streaming são a
maior parte do tamanho da imagem. A maior economia virá da remoção de
`node_modules`, Playwright e do servidor MCP genérico, não do navegador.

## O que poderá ser realmente Go

Pode ser implementado em Go:

- servidor MCP;
- transporte Streamable HTTP;
- autenticação Bearer local e OAuth futuro;
- conexão e reconexão com CDP;
- tabs, navegação, leitura, clique, digitação e espera;
- snapshots compactos e referências de elementos;
- screenshots retornados como conteúdo MCP de imagem;
- lease exclusivo do navegador;
- máquina de estados do takeover humano;
- aprovação em duas etapas para ações externas;
- política de URLs e domínios;
- auditoria, health checks e limpeza de artefatos;
- integração opcional com PocketBase;
- testes unitários, integração e smoke tests.

Continuarão sendo componentes externos:

- Chromium;
- Selkies, noVNC ou outra GUI de navegador;
- `cloudflared` para o acesso humano;
- cliente do OpenAI Secure MCP Tunnel;
- Docker e Docker Compose.

Portanto, “todo em Go” deve significar todo o código de negócio e controle do
projeto, não a substituição do Chromium por um navegador escrito em Go.

## Bibliotecas recomendadas

### MCP: SDK Go oficial

Usar:

```text
github.com/modelcontextprotocol/go-sdk/mcp
```

O SDK oficial oferece servidor, cliente, tools, schemas, conteúdo de imagem,
transporte Streamable HTTP e componentes de autorização. Ele também mantém
compatibilidade com versões anteriores do protocolo, o que é importante porque
o ChatGPT pode negociar uma versão anterior à versão mais nova do SDK.

Referências:

- [SDK Go oficial do MCP](https://github.com/modelcontextprotocol/go-sdk)
- [Documentação do MCP Go SDK](https://go.sdk.modelcontextprotocol.io/)
- [Releases e compatibilidade do SDK](https://github.com/modelcontextprotocol/go-sdk/releases)

Não há motivo para escrever JSON-RPC ou o transporte MCP manualmente.

### Automação: chromedp

Recomendação inicial:

```text
github.com/chromedp/chromedp
github.com/chromedp/cdproto
```

O `chromedp` fala CDP diretamente em Go e possui `RemoteAllocator` para se
conectar a um Chromium já em execução. Isso combina com o perfil persistente e a
GUI humana do projeto.

Referências:

- [chromedp](https://github.com/chromedp/chromedp)
- [RemoteAllocator](https://github.com/chromedp/chromedp/blob/main/allocate.go)
- [Chrome DevTools Protocol](https://chromedevtools.github.io/devtools-protocol/)

O `go-rod/rod` também é uma opção válida e mais orientada a uma API de alto
nível. Antes de congelar a escolha, deve existir um spike curto com as duas
bibliotecas nos seguintes fluxos:

- conectar ao Chromium existente;
- descobrir e alternar abas;
- extrair árvore de acessibilidade;
- clicar e digitar em X e em uma página com iframe;
- capturar screenshot;
- desconectar sem fechar o Chromium.

Se ambas passarem, a preferência permanece com `chromedp` por ser direto,
pequeno e possuir bindings CDP gerados. Se a quantidade de código de interação
ficar excessiva, Rod pode ser adotado antes do início das tools MCP.

Referência alternativa:

- [Rod](https://github.com/go-rod/rod)

### Por que não Playwright Go

O binding Go do Playwright ainda depende do driver do ecossistema Playwright e
não produz uma pilha operacional realmente nativa em Go. Além disso, continuar
com Playwright manteria parte considerável da dependência que a reescrita busca
eliminar.

O CDP já é a interface utilizada pelo projeto atual para anexar ao Chromium. A
própria documentação do Playwright alerta que a conexão por CDP possui fidelidade
inferior ao protocolo Playwright completo. Como construiremos um conjunto menor
de operações, o CDP direto é uma escolha aceitável, desde que testado contra uma
versão de Chromium fixada.

Referência:

- [Playwright `connectOverCDP`](https://github.com/microsoft/playwright/blob/main/docs/src/api/class-browsertype.md#async-method-browsertypeconnectovercdp)

## Arquitetura de runtime

```text
                         Docker host

  +-----------------------------------------------------------+
  |                                                           |
  |  +------------------+       CDP em loopback                |
  |  | Chromium + GUI   |<---------------------+               |
  |  | perfil /config   |                      |               |
  |  +--------+---------+               +------+-----------+   |
  |           | GUI                     | navego-gateway    |   |
  |           |                         | MCP + políticas   |   |
  |  +--------v---------+               +------+-----------+   |
  |  | cloudflared      |                      | MCP privado    |
  |  +------------------+               +------v-----------+   |
  |                                     | OpenAI tunnel    |   |
  |                                     +------------------+   |
  +-----------------------------------------------------------+
```

O ideal é que o gateway Go e o Chromium sejam processos separados, mas
compartilhem o namespace de rede no Docker. Assim, o CDP pode continuar em
`127.0.0.1:9222`, sem ser aberto nem mesmo para toda a rede Docker.

No Compose isso pode ser alcançado com um sidecar usando
`network_mode: service:navego-browser`. Outra opção é mantê-los temporariamente na
mesma imagem durante o desenvolvimento e separá-los antes da instalação
permanente.

Serviços previstos:

- `navego-browser`: navegador, GUI e perfil persistente;
- `navego-gateway`: binário Go;
- `mcp-tunnel`: cliente do Secure MCP Tunnel;
- `gui-tunnel`: Cloudflare Tunnel para a interface humana;
- `pocketbase`: inexistente no início; preferencialmente embutido no binário se
  vier a ser necessário.

## Estrutura de diretórios proposta

```text
cmd/
  navego/
    main.go

internal/
  app/
    app.go
  config/
    config.go
  mcpserver/
    server.go
    tools.go
    content.go
  browser/
    client.go
    targets.go
    navigation.go
    snapshot.go
    refs.go
    actions.go
    screenshot.go
    events.go
  policy/
    urls.go
    networks.go
    actions.go
    secrets.go
  takeover/
    state.go
    lease.go
  approval/
    prepare.go
    token.go
    commit.go
  artifacts/
    store.go
    cleanup.go
  audit/
    logger.go
  storage/
    storage.go
    memory.go
    pocketbase.go       # somente se necessário

test/
  integration/
  e2e/
  fixtures/

docs/
  architecture-plan.md
  go-rewrite-plan.md

go.mod
go.sum
Dockerfile
compose.yaml
```

Não haverá um diretório `pkg` até surgir uma API realmente reutilizável fora da
aplicação. As interfaces devem existir nos limites que precisamos simular em
testes, especialmente navegador, relógio, tokens e armazenamento.

## API HTTP

O binário deve expor somente:

- `POST/GET/DELETE /mcp`, conforme o transporte negociado;
- `GET /healthz`, para vida do processo;
- `GET /readyz`, validando conexão com o Chromium;
- opcionalmente uma rota interna para concluir takeover, sem exposição pública.

A raiz não precisa revelar versão, configuração ou estado detalhado. Respostas
devem usar `Cache-Control: no-store`, limites de corpo, timeouts e graceful
shutdown.

O endpoint MCP continuará privado quando o Secure MCP Tunnel for usado. Caso o
produto um dia seja distribuído publicamente, o SDK Go oferece os componentes
para integrar OAuth, mas essa será uma fase separada.

Referências OpenAI:

- [Construção de servidores MCP](https://developers.openai.com/plugins/build/mcp-server)
- [Conectar ao ChatGPT](https://developers.openai.com/plugins/deploy/connect-chatgpt)
- [Autenticação de MCP](https://developers.openai.com/plugins/build/auth)

## Modelo interno do navegador

### Conexão persistente

O pacote `browser` cria um `RemoteAllocator` apontando para
`http://127.0.0.1:9222`, descobre os targets existentes e se anexa ao perfil
padrão. O cancelamento do contexto Go deve encerrar apenas a conexão CDP, nunca o
processo do Chromium.

O cliente deve:

- reconectar com backoff quando CDP cair;
- invalidar referências ao perder a conexão;
- expor um estado de readiness;
- observar criação, navegação e fechamento de tabs;
- limitar o número de abas;
- nunca criar ou remover perfis do usuário implicitamente.

### Snapshot semântico compacto

Esta é a parte mais complexa da reescrita.

O Playwright MCP atual produz snapshots e referências de elementos. Na versão
Go, isso será implementado sobre:

- `Accessibility.getFullAXTree` para role, nome e estado acessível;
- `DOMSnapshot.captureSnapshot` como complemento para DOM, iframes, shadow DOM e
  layout;
- IDs de backend do DOM para reencontrar os elementos;
- árvore reduzida contendo somente texto útil e nós interativos.

Referências CDP:

- [Accessibility domain](https://chromedevtools.github.io/devtools-protocol/tot/Accessibility/)
- [DOMSnapshot domain](https://chromedevtools.github.io/devtools-protocol/tot/DOMSnapshot/)

Exemplo de resposta compacta:

```text
page title="Home / X" url="https://x.com/home"
[e1] link "Página inicial"
[e2] textbox "O que está acontecendo?"
[e3] button "Postar" disabled=false
```

Cada snapshot recebe um `snapshot_id`. Cada referência associa:

- snapshot;
- target/tab;
- frame;
- backend node ID;
- role e nome conhecidos;
- geração da página;
- hash dos atributos relevantes.

Uma ação deve receber `snapshot_id` e `ref`. Depois de navegação ou mudança
relevante, refs antigas são rejeitadas com uma resposta pedindo novo snapshot.
Isso evita clicar em um botão diferente após a página mudar.

O snapshot deve impor limites de profundidade, caracteres e quantidade de nós.
Também deve oferecer busca direcionada para não devolver a página inteira a cada
ação. Essa camada pode reduzir bastante o consumo de tokens em comparação com um
snapshot genérico extenso.

### Interações

As ferramentas devem trabalhar com referências opacas, nunca com JavaScript
arbitrário fornecido pelo modelo.

Operações internas necessárias:

- resolver ref para backend node;
- validar visibilidade, estabilidade e estado habilitado;
- trazer elemento para a viewport;
- focar;
- clicar ou digitar por eventos de input;
- aguardar navegação ou estabilidade esperada;
- retornar URL, título e mudança resumida;
- invalidar o snapshot quando necessário.

O CDP oferece captura de screenshot e eventos de mouse/teclado diretamente:

- [Page.captureScreenshot](https://chromedevtools.github.io/devtools-protocol/tot/Page/#method-captureScreenshot)
- [Input domain](https://chromedevtools.github.io/devtools-protocol/tot/Input/)

## Ferramentas MCP da versão Go

### Leitura e navegação

- `browser_status`
- `browser_open`
- `browser_snapshot`
- `browser_find`
- `browser_tabs`
- `browser_select_tab`
- `browser_wait`
- `browser_take_screenshot`

### Interações reversíveis

- `browser_click`
- `browser_type`
- `browser_select_option`
- `browser_press_key`

Essas ferramentas ainda passam pela política. `browser_type` deve recusar campos
de senha e outros campos classificados como secretos, mesmo que o conteúdo tenha
sido enviado no chat.

### Takeover humano

- `browser_request_human_login`
- `browser_takeover_status`

### Ações externas

- `browser_prepare_action`
- `browser_commit_action`
- `browser_cancel_action`

Não existirão:

- execução de código Go, shell ou JavaScript recebido pelo MCP;
- seletor CSS/XPath livre como ferramenta pública;
- leitura arbitrária de arquivo;
- upload arbitrário;
- chamada HTTP genérica;
- acesso direto ao CDP pelo cliente MCP.

## Takeover e concorrência

O servidor deve possuir um único lease de escrita do navegador. Uma sessão MCP
adquire o lease antes da primeira ação. Outras sessões podem consultar status,
mas não podem navegar, clicar ou digitar.

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

Regras:

- somente o dono do lease pode executar ações;
- `HUMAN_ACTIVE` bloqueia toda automação;
- takeover tem identificador opaco e expiração;
- concluir takeover não concede aprovação para publicar ou comprar;
- disconnect não libera imediatamente um lease durante ação sensível;
- existe expiração segura e uma operação administrativa local para recuperar um
  lease órfão;
- toda transição é testada com `go test -race`.

Para o primeiro release, responder `pronto` no chat é suficiente. Depois, a GUI
pode receber um pequeno botão `Concluir login`, enviando o takeover ID para uma
rota interna autenticada.

## Aprovação de ações externas

`browser_prepare_action` deve produzir uma descrição estruturada:

```json
{
  "action": "publish_post",
  "site": "x.com",
  "summary": "Publicar no X",
  "content": "Texto final do post",
  "target": "button:Postar",
  "expires_at": "...",
  "approval_id": "..."
}
```

O token de aprovação deve vincular:

- sessão MCP;
- tab, URL e origem;
- tipo da ação;
- hash do conteúdo;
- hash do elemento final;
- instante de expiração;
- nonce de uso único.

No `commit`, todos os campos são comparados novamente. Se texto, URL, elemento
ou página mudarem, a aprovação é invalidada. O token pode ser assinado com HMAC
e mantido em memória; reiniciar o servidor invalida aprovações pendentes, o que é
um comportamento seguro.

Isso elimina a necessidade de banco de dados para aprovações no primeiro
release.

## Screenshots e artefatos

`Page.captureScreenshot` devolve bytes codificados em base64 pelo protocolo. O
servidor Go pode convertê-los diretamente para `ImageContent` do MCP, sem criar
um arquivo permanente.

Regras propostas:

- PNG como padrão;
- viewport como padrão;
- full page opcional e com limite de dimensões;
- JPEG com qualidade limitada para páginas muito grandes;
- limite máximo de bytes retornados;
- nenhuma gravação em disco quando o cliente aceitar conteúdo de imagem;
- fallback em arquivo temporário com URL autenticada e TTL curto;
- limpeza idempotente de arquivos expirados;
- opção futura de redigir regiões sensíveis.

## Política de segurança

### URLs e rede

O gateway valida protocolo, hostname, porta e redirecionamentos antes de cada
navegação. Deve recusar loopback, link-local, redes privadas, multicast, esquemas
não HTTP e endpoints de metadados.

Validação apenas por hostname não elimina DNS rebinding. A instalação permanente
deve combinar:

- validação no gateway Go;
- resolução DNS e verificação de todos os IPs;
- proxy de saída ou firewall que também bloqueie redes internas;
- interceptação de navegações e redirecionamentos;
- rede Docker sem acesso desnecessário à LAN do host.

O navegador precisa de internet aberta para visitar sites arbitrários, portanto
uma allowlist fixa de domínios não serve ao objetivo geral. A política deve
impedir redes internas e permitir origens públicas, com confirmação adicional
para novos domínios quando a tarefa for sensível.

### Secrets

- nunca aceitar senha, OTP, recovery code ou cookie como argumento de tool;
- detectar campos de senha e redirecionar para takeover humano;
- nunca registrar conteúdo digitado em campos sensíveis;
- não devolver cookies, local storage ou headers de autenticação;
- usar comparação constante para Bearer local;
- carregar secrets por arquivo ou secret store, não pela imagem.

### Browser

A reescrita Go não corrige o `--no-sandbox` do Chromium atual. Isso deve ser um
trabalho paralelo:

- avaliar uma imagem que permita sandbox por user namespaces;
- executar navegador e gateway como usuários não root;
- separar perfil, outputs e binário em mounts mínimos;
- aplicar seccomp, capabilities e limites compatíveis com o sandbox escolhido;
- não expor CDP;
- não montar diretórios do host além do perfil necessário.

## PocketBase

PocketBase pode ser usado como framework Go ou aplicação standalone. Ele inclui
SQLite, autenticação, realtime, dashboard e uma API administrativa em um binário
pequeno. A documentação atual também ressalta que o projeto ainda não chegou à
versão 1.0 e pode exigir migrações manuais entre releases.

Referências:

- [PocketBase](https://pocketbase.io/docs/)
- [Estender PocketBase com Go](https://pocketbase.io/docs/go-overview/)
- [PocketBase como framework](https://pocketbase.io/docs/use-as-framework/)

### Recomendação para o primeiro release

Não usar banco de dados inicialmente.

O estado já possui destinos naturais:

- login e cookies: perfil persistente do Chromium;
- sessões MCP: memória;
- lease: memória;
- aprovações: tokens HMAC curtos e nonces em memória;
- configuração: ambiente e secrets;
- auditoria inicial: JSON estruturado com retenção e conteúdo mínimo;
- screenshots: resposta MCP ou arquivos temporários.

Isso mantém o binário e o modelo operacional menores.

### Quando PocketBase passa a valer a pena

Adicionar PocketBase se precisarmos de pelo menos um destes itens:

- múltiplos usuários;
- painel administrativo de sessões;
- histórico consultável de ações e aprovações;
- gerenciamento de artefatos e TTL persistente;
- associação de usuários a instâncias de navegador;
- regras e domínios configuráveis pela GUI;
- eventos realtime para um painel de takeover;
- provisionamento de containers ou perfis.

Coleções possíveis:

- `browser_sessions`
- `takeovers`
- `approvals`
- `audit_events`
- `artifacts`
- `browser_instances`
- `domain_policies`

Não armazenar no PocketBase:

- senhas;
- códigos de 2FA;
- cookies do navegador;
- local storage;
- dumps completos de DOM;
- screenshots permanentes por padrão;
- tokens de sessão em texto aberto.

PocketBase não deve substituir Cloudflare Access na GUI nem OAuth na conexão
MCP. Ele seria armazenamento e painel interno, não a única fronteira de
segurança.

## Docker e tamanho

### Build do gateway

Usar Dockerfile multi-stage:

1. imagem Go para `go mod download`, testes e build;
2. build reproduzível com `-trimpath` e símbolos removidos;
3. runtime distroless/nonroot com certificados e dados de timezone;
4. nenhum compilador, shell ou cache de módulos no runtime.

Se PocketBase não for utilizado, o gateway pode permanecer `CGO_ENABLED=0`.
PocketBase também oferece SQLite puro Go por padrão, mas adiciona funcionalidades
e tamanho que não são necessários no primeiro release.

### Separação dos serviços

A menor quantidade de containers não é necessariamente a arquitetura mais
segura. Recomenda-se separar gateway e navegador para atualizar, limitar e
observar cada processo independentemente.

O binário Go será pequeno; a imagem do Chromium continuará sendo o principal
componente do deployment.

## Testes

### Unitários

- parsing e validação de configuração;
- autenticação Bearer em tempo constante;
- URLs IPv4, IPv6, IDN, redirects e DNS rebinding;
- máquina de estados do takeover;
- concorrência e expiração de lease;
- tokens de aprovação, expiração e replay;
- refs válidas, inválidas e obsoletas;
- redução e limite do snapshot;
- limpeza de artefatos;
- redaction de logs.

### Integração

- servidor MCP e negociação de protocolo;
- listagem e schemas das tools;
- conexão por `RemoteAllocator`;
- Chromium indisponível e reconexão;
- criação, seleção e fechamento de tabs;
- iframe e shadow DOM;
- screenshot como `ImageContent`;
- cancelamento e timeout de tools;
- múltiplas sessões MCP disputando o lease.

### End-to-end

- página pública estática controlada por fixture;
- formulário local seguro para prepare/commit;
- takeover humano simulado;
- X com rascunho e confirmação manual;
- Amazon somente leitura;
- portal da faculdade e screenshot;
- reinício mantendo cookies;
- página maliciosa com prompt injection;
- tentativa de acesso a localhost e metadata;
- falha dos túneis e recuperação.

Comandos de qualidade previstos:

```text
go test ./...
go test -race ./...
go vet ./...
```

Também devem existir golden tests dos snapshots para detectar mudanças causadas
por upgrades de Chromium ou `cdproto`.

## Estratégia de migração

O baseline Node foi preservado durante a implementação inicial e removido em 27
de agosto de 2026, por decisão do proprietário, depois que o transporte MCP Go,
o Chromium e o takeover passaram pelo smoke real. O status atual está em
[implementation-status.md](./implementation-status.md).

### Fase 0: preservar baseline — concluída e encerrada

- o protótipo foi mantido durante a fundação Go;
- o planejamento e os fluxos foram registrados;
- o volume persistente do Chromium foi preservado;
- os fontes e dependências Node foram removidos depois do smoke Go.

Critério: conseguir comparar o comportamento novo com o protótipo sem arriscar o
perfil real.

### Fase 1: spike Go

- criar `go.mod`;
- subir um MCP mínimo com `browser_status`;
- testar Streamable HTTP com um cliente MCP;
- conectar `chromedp` ao Chromium remoto;
- executar o mesmo spike com Rod;
- escolher definitivamente a biblioteca de browser.

Critério: conexão, navegação, tabs, input e screenshot funcionando sem fechar o
Chromium quando o gateway desconectar.

### Fase 2: núcleo do navegador

- implementar manager de conexão e targets;
- health e readiness;
- navegação segura;
- tabs;
- screenshot;
- timeouts, cancelamento e reconexão.

Critério: operações básicas estáveis contra uma fixture local e sites públicos.

### Fase 3: snapshot e refs

- extrair AXTree e DOMSnapshot;
- produzir formato compacto;
- criar mapa de refs por geração;
- implementar busca por role e nome;
- resolver refs em elementos;
- cobrir iframe e shadow DOM;
- criar golden tests.

Critério: o modelo consegue encontrar e interagir com elementos sem selector ou
JavaScript arbitrário.

### Fase 4: tools de interação

- click, type, select, key e wait;
- validação de visibilidade e estabilidade;
- schemas e anotações MCP;
- respostas curtas e estruturadas;
- limites de tamanho e timeout.

Critério: paridade funcional mínima com os fluxos úteis do protótipo.

### Fase 5: takeover e lease

- máquina de estados;
- exclusão mútua entre sessões;
- URL da GUI;
- retorno `human_action_required`;
- retomada segura;
- testes de corrida e expiração.

Critério: durante login humano nenhuma ação automática pode alcançar o browser.

### Fase 6: aprovação e políticas

- prepare/commit;
- tokens HMAC e replay protection;
- classificação de ações sensíveis;
- bloqueio de campos secretos;
- política de URL e rede;
- logs redigidos;
- auditoria mínima.

Critério: publicação, envio, alteração e compra não podem acontecer sem aprovação
vinculada ao conteúdo e elemento finais.

### Fase 7: Docker e conectividade privada

- build multi-stage do gateway;
- serviços separados;
- CDP somente em namespace privado/loopback;
- Cloudflare Tunnel para GUI;
- Secure MCP Tunnel para ChatGPT;
- volumes, secrets e limites;
- revisão do sandbox do Chromium.

Critério: nenhuma porta sensível fica diretamente pública.

### Fase 8: validação e corte

- executar toda a matriz end-to-end;
- conectar a versão Go ao ChatGPT;
- comparar screenshots, snapshots e custo de tokens;
- migrar o perfil real somente depois dos testes;
- validar o corte já realizado do servidor Node e dos fontes JavaScript;
- manter um procedimento documentado de rollback por uma release.

Critério: versão Go realiza os três casos principais e preserva o takeover.

### Fase 9: PocketBase opcional

- revisar se existe necessidade real após uso pessoal;
- adicionar coleções e migrations somente para dados necessários;
- criar retenção e exclusão;
- proteger dashboard administrativo;
- testar upgrades e backup/restore.

Critério: PocketBase resolve um requisito observado, não apenas uma possibilidade
futura.

## Riscos e impedimentos

### Snapshot e seletores

É o maior risco técnico. Playwright possui anos de tratamento de frames, shadow
DOM, acessibilidade, elementos instáveis e páginas reativas. Nossa implementação
será menor e mais segura, mas terá menos compatibilidade inicialmente.

Mitigação: escopo reduzido de tools, refs por snapshot, fixtures complexas,
versão de Chromium fixada e testes reais nos sites prioritários.

### Mudanças do CDP

Partes úteis, como a árvore completa de acessibilidade, ainda são marcadas como
experimentais no protocolo. Atualizações de Chromium podem mudar comportamento.

Mitigação: fixar Chromium e `cdproto`, atualizar de forma controlada e executar
golden/E2E tests antes do deploy.

### Detecção de automação

Go não torna o navegador invisível. X, Amazon e outros sites podem detectar CDP,
alterar a interface, exigir CAPTCHA ou restringir contas.

Mitigação: takeover humano, baixa frequência, ausência de bypasses e respeito às
regras do site. Não há garantia de funcionamento permanente em todo site.

### Segurança do contêiner

Trocar Node por Go remove RCE exposta pelo Playwright MCP, mas não protege por si
só um Chromium sem sandbox nem um CDP aberto.

Mitigação: tratar sandbox, rede, mounts, usuário e túneis como requisitos do
release, não como melhorias posteriores.

### PocketBase pré-1.0

PocketBase é prático e compacto, mas sua própria documentação informa que
compatibilidade completa ainda não é garantida antes de 1.0.

Mitigação: não utilizá-lo no núcleo inicial; se adotado, fixar a versão, manter
migrations, backup testado e acompanhar o changelog.

## Matriz de viabilidade da reescrita

| Componente | Viabilidade em Go | Complexidade |
| --- | --- | --- |
| MCP Streamable HTTP | Alta | Baixa |
| Auth local e health checks | Alta | Baixa |
| Conexão ao Chromium persistente | Alta | Baixa/média |
| Tabs e navegação | Alta | Média |
| Screenshot como imagem MCP | Alta | Baixa |
| Takeover humano e lease | Alta | Média |
| Prepare/commit de ações | Alta | Média |
| Snapshot semântico compacto | Alta | Alta |
| Iframes e shadow DOM genéricos | Média/alta | Alta |
| Paridade completa com Playwright | Desnecessária | Muito alta |
| Redução do código da aplicação | Alta | — |
| Grande redução da imagem total | Baixa/média | Chromium domina |
| PocketBase single-user | Viável, mas dispensável | Média |
| PocketBase multiusuário | Alta, com cautela pré-1.0 | Média/alta |

## Recomendação final

Refazer em Go é uma boa decisão, desde que a meta não seja reproduzir todas as
capacidades do Playwright MCP. O valor da reescrita está justamente em criar um
servidor menor, previsível e com poucas ferramentas seguras.

Decisão proposta:

1. Go SDK oficial do MCP.
2. Spike de `chromedp` e Rod, com preferência inicial por `chromedp`.
3. CDP conectado ao mesmo Chromium persistente e visível.
4. Snapshot semântico próprio com refs opacas.
5. Nenhuma execução arbitrária, selector livre ou upload genérico.
6. Takeover humano e aprovação de escritas como partes do núcleo.
7. Sem banco de dados no primeiro release.
8. PocketBase somente após um requisito observado de persistência ou gestão.
9. Migração paralela usando perfil de teste separado.
10. Remoção da versão Node somente depois da validação no ChatGPT.
