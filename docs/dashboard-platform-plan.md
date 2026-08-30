# Plano da plataforma Navego

Data da pesquisa: 29 de agosto de 2026.

Este documento substitui a visão de "um Chromium e um usuário por instalação"
como direção principal do projeto. O gateway Chromium-only atual continua sendo
a base funcional, mas passará a ser um worker de navegador dentro de uma
plataforma com dashboard, contas, múltiplas instâncias e integração MCP por
usuário.

## Objetivo do MVP

Entregar uma instalação única do Navego em um servidor Docker que permita:

- cadastro, login e logout de usuários;
- criação, edição de nome, início, parada e exclusão de Chromiums;
- um perfil e volume persistente isolado para cada Chromium;
- cards retangulares com preview da página atual;
- visualização interativa em um modal de aproximadamente 90% da tela;
- configuração guiada do MCP no ChatGPT;
- takeover humano para senha, MFA, passkey, OTP e CAPTCHA;
- cadastro de credenciais no dashboard sem armazenar senha em texto puro;
- uso da credencial pelo navegador sem expô-la ao modelo, ao MCP ou aos logs;
- deploy no Dokploy com Traefik, mantendo os Chromiums dinâmicos fora do ciclo de
  deploy da aplicação.

O objetivo inicial é funcionar bem em um único servidor para poucos usuários.
Alta disponibilidade, múltiplos hosts, cobrança e autoscaling ficam fora do MVP.

## Decisões principais

1. O frontend ficará em `/web`, usando Next.js App Router, TypeScript,
   Tailwind CSS e shadcn/ui.
2. PocketBase será usado como framework Go embutido no control plane, em vez de
   adicionar um backend PocketBase separado.
3. Dokploy implantará somente os serviços estáticos da plataforma. Ele não será
   chamado para criar uma aplicação ou Compose a cada novo Chromium.
4. Um agente Go privado usará a Docker Engine API para criar e reconciliar as
   instâncias dinâmicas.
5. Cada Chromium terá um worker Go próprio compartilhando seu namespace de
   rede. Assim, o CDP continua em loopback e não fica acessível aos outros
   navegadores.
6. Os cards usarão screenshots leves e periódicos. Apenas o navegador aberto no
   modal usará o stream interativo da GUI, evitando vários streams caros ao
   mesmo tempo.
7. As credenciais serão cifradas no servidor antes de chegar ao PocketBase. A
   chave mestra ficará em Docker Secret, nunca no banco ou no repositório.
8. Haverá um único endpoint MCP público por instalação. O OAuth identifica o
   usuário, e cada grant do ChatGPT é associado a um Chromium selecionado
   daquele usuário.

## Versões verificadas

Versões estáveis observadas nas fontes oficiais em 29 de agosto de 2026:

| Componente | Versão de referência | Decisão |
| --- | ---: | --- |
| Go | 1.27.0 | atualizar `go.mod` e a imagem de build de 1.26 para 1.27 |
| Node.js LTS | 24.20.0 | usar a linha LTS 24 na imagem do frontend |
| npm | 11.19.0 | incluído na imagem oficial do Node 24 e fixado em `packageManager` |
| Next.js | 16.3.3 | App Router e build standalone |
| React | 19.2.8 | versão estável compatível declarada pelo Next.js atual |
| Tailwind CSS | 4.3.3 | usar configuração CSS-first do Tailwind v4 |
| shadcn CLI/CSS | 4.19.0 | componentes em código-fonte; pacote fica somente no build para o CSS oficial |
| TypeScript | 6.0.3 | última linha compatível; o typescript-eslint do Next 16 ainda recusa TS 7 |
| ESLint | 9.39.5 | última linha aceita por todos os plugins incluídos pelo Next 16 |
| PocketBase | 0.40.1 | embutir como framework Go e fixar a versão |
| PocketBase JS SDK | 0.28.0 | uso server-side no BFF do Next.js |

Fontes:

- [Next.js no npm](https://registry.npmjs.org/next/latest)
- [React no npm](https://registry.npmjs.org/react/latest)
- [Tailwind CSS no npm](https://registry.npmjs.org/tailwindcss/latest)
- [shadcn CLI no npm](https://registry.npmjs.org/shadcn/latest)
- [PocketBase releases](https://github.com/pocketbase/pocketbase/releases/latest)
- [PocketBase JS SDK no npm](https://registry.npmjs.org/pocketbase/latest)
- [Node.js releases](https://nodejs.org/dist/index.json)
- [versão estável do Go](https://go.dev/VERSION?m=text)

As versões serão fixadas no lockfile e nas imagens. Antes do primeiro scaffold,
elas devem ser consultadas novamente. Se o `latest` de uma dependência não for
compatível com o restante do stack, será escolhida a versão estável compatível
mais recente e a exceção será registrada neste documento.

A imagem Chromium também continuará fixada por tag e digest. Uma imagem nova só
substituirá a atual depois dos smokes de CDP, screenshot, GUI, perfil persistente
e takeover. Usar `latest` diretamente em produção tornaria os navegadores
existentes diferentes sem revisão.

## Arquitetura proposta

```text
                                  Internet
                                     |
                                  Traefik
                 +-------------------+-------------------+
                 |                   |                   |
                 v                   v                   v
       browser.lspr.dev    mcp.browser.lspr.dev   view.browser.lspr.dev
                 |                   |                   |
                 v                   v                   v
            Next.js web       Navego control       viewer proxy
              (BFF)             Go + PB                 |
                 |              /  |  \                  |
                 +-------------+   |   +-----------------+
                                   |
                    API HTTP privada autenticada
                                   |
                             Navego agent
                                   |
                         /var/run/docker.sock
                                   |
                +------------------+------------------+
                |                                     |
       Chromium A + worker A                 Chromium B + worker B
          volume próprio                       volume próprio
          CDP em loopback                      CDP em loopback
```

### `web`: dashboard Next.js

Responsabilidades:

- renderizar login, cadastro e dashboard;
- manter o token PocketBase em cookie `HttpOnly`, `Secure` e `SameSite=Lax`;
- atuar como BFF para que o navegador do usuário não receba credenciais de
  superuser nem precise armazenar token de autenticação no `localStorage`;
- chamar apenas a API pública e autenticada do control plane;
- exibir screenshots com `Cache-Control: no-store`;
- abrir o viewer interativo por meio de um ticket curto e descartável.

Server Components serão o padrão. Componentes com polling, ações, iframe,
Dialog ou estado visual usarão `"use client"` somente nas folhas necessárias.

### `navego-control`: control plane Go

O processo principal incorporará o PocketBase e registrará rotas próprias. O
PocketBase oferece autenticação, SQLite, migrations, regras de coleção e backup,
enquanto o código Go mantém o controle das regras sensíveis.

Responsabilidades:

- autenticação e autorização de usuários;
- API de browsers e credenciais;
- estado desejado das instâncias no PocketBase;
- autorização OAuth do ChatGPT;
- resource server e endpoint MCP público;
- associação `usuário -> grant OAuth -> browser selecionado`;
- proxy das chamadas MCP para o worker correto;
- geração de previews e tickets de viewer;
- cifrar e decifrar credenciais somente em memória;
- auditoria de ações administrativas e uso de credenciais.

O dashboard administrativo nativo do PocketBase não será exposto publicamente.
Quando necessário, ficará restrito por loopback, VPN ou Cloudflare Access.

### `navego-agent`: controlador Docker privado

Usaremos o SDK Go oficial da Docker Engine com negociação de versão de API. O
agent será o único processo com acesso a `/var/run/docker.sock`.

Ele não aceitará uma representação livre de `docker run`. A API será pequena e
orientada a intenção:

- `EnsureBrowser(browserID)`;
- `StartBrowser(browserID)`;
- `StopBrowser(browserID)`;
- `DeleteBrowser(browserID, deleteProfile)`;
- `InspectBrowser(browserID)`;
- `Reconcile()`.

Imagem, comandos, mounts, redes, capabilities e limites serão definidos no
código. O usuário poderá escolher somente opções permitidas, como nome e, no
futuro, tamanho pré-definido.

Control e agent conversarão por uma API HTTP privada, autenticada por um token
de serviço independente. O agent não terá porta pública nem participará da rede
do frontend. Somente ele montará o Unix socket real do Docker. A documentação do
Docker alerta que controlar o daemon equivale a conceder poder de root no host;
por isso esse socket não pode ser montado no Next.js nem no control plane público:
[Docker Engine security](https://docs.docker.com/engine/security/).

### Par Chromium + worker

Cada registro de browser gera:

- um volume `navego-browser-<id>` para `/config`;
- um container Chromium `navego-browser-<id>`;
- um container worker `navego-worker-<id>`;
- labels imutáveis de ownership e reconciliação;
- limites de CPU, memória, PIDs e tamanho de logs.

O worker compartilhará o namespace de rede do Chromium, preservando o desenho
atual:

```text
worker -> http://127.0.0.1:9222 -> Chromium
```

O CDP não será publicado no host nem na rede compartilhada. O worker exporá
somente uma API interna autenticada para MCP, screenshot, takeover e health.

Labels propostas:

```text
dev.lspr.navego.managed=true
dev.lspr.navego.browser_id=<id>
dev.lspr.navego.owner_id=<pocketbase-user-id>
dev.lspr.navego.role=browser|worker
```

Ao iniciar, o agent comparará os registros do PocketBase com containers e
volumes que tenham essas labels. Isso permite recuperar o estado depois de um
redeploy sem confundir containers do Dokploy com instâncias do Navego.

## Por que não criar cada Chromium pelo Dokploy

O Dokploy possui API para criar e implantar aplicações e Composes, mas esse
fluxo é voltado a deploy, gera histórico e normalmente exige redeploy para
alterações. Um browser é um recurso de runtime: criar, parar e remover precisa
ser rápido, idempotente e reconciliável.

Para o MVP:

- Dokploy instala e atualiza `web`, `control`, `agent` e Traefik;
- Docker Engine API gerencia os Chromiums e workers dinâmicos;
- os volumes dos perfis sobrevivem aos deploys do Dokploy;
- nenhuma chave administrativa do Dokploy precisa entrar no Navego.

Essa abordagem é menor e reduz o acoplamento. Se no futuro houver vários
servidores, o agent poderá ser instalado em cada host, sem reescrever o
dashboard.

## Modelo de dados PocketBase

### `users` — auth collection

Campos iniciais:

- `email`, `password`, `verified` e campos nativos de autenticação;
- `name`;
- `status`: `active`, `disabled`;
- `max_browsers`, com padrão pequeno;
- `created`, `updated`.

Para evitar que qualquer visitante use CPU e memória criando Chromiums, o MVP
será invite-only por padrão. A tela de cadastro existirá, mas exigirá um código
de convite ou aprovação. Cadastro público poderá ser habilitado depois com
verificação de e-mail, rate limiting e quotas.

### `browsers`

- `owner` relation obrigatória para `users`;
- `name`;
- `status`: `provisioning`, `running`, `stopped`, `error`, `deleting`;
- `desired_status`: `running` ou `stopped`;
- `is_default`;
- `runtime_version`;
- `last_seen_at`, `last_error`;
- identificadores de runtime usados para diagnóstico.

O banco guarda o estado desejado. Labels Docker são a fonte de reconciliação do
estado real. Renomear um browser altera apenas o registro; não recria container
ou volume.

### `saved_credentials`

- `owner`;
- `label`;
- `origin` HTTPS exata, sem path, query ou wildcard;
- `encrypted_payload`;
- `key_version`;
- `created`, `updated`, `last_used_at`.

`encrypted_payload` conterá username e password no mesmo envelope cifrado. A API
do PocketBase não permitirá listagem ou leitura desse campo pelo cliente. A
interface receberá somente id, label, origin e datas.

### `oauth_grants` e `oauth_refresh_tokens`

- autorização do ChatGPT por usuário e scopes;
- browser vinculado ao grant;
- client id/metadata validado;
- refresh tokens armazenados somente como hash;
- expiração, rotação e revogação.

### `audit_events`

- owner, browser, action, origin, result e timestamp;
- nunca incluir texto digitado, senha, tokens, cookies ou conteúdo de página;
- retenção curta e configurável no MVP.

Migrations das coleções ficarão versionadas em `pb_migrations`.

## Cofre de credenciais

### Fluxo de cadastro

1. O usuário abre “Credenciais” no dashboard.
2. Informa label, origem HTTPS exata, username e password.
3. O formulário é enviado por HTTPS ao BFF e imediatamente repassado ao control.
4. O control valida usuário, quota, origem e tamanho dos campos.
5. Username e password são serializados em memória e cifrados com AES-256-GCM.
6. O PocketBase recebe somente ciphertext, nonce implícito no envelope,
   `key_version` e metadados não secretos.
7. Buffers temporários são limpos; logs e erros nunca incluem o payload.
8. O dashboard mostra apenas label e domínio, nunca oferece “revelar senha”.

No MVP local, a chave mestra de 32 bytes é fornecida em base64 por
`NAVEGO_VAULT_KEY`; no Dokploy ela é obrigatória e deve ser configurada como
secret. Associated data inclui versão, `owner_id` e origem canônica, impedindo
que um ciphertext seja reutilizado por outro usuário ou site. Montagem via
Docker Secret e rotação versionada permanecem como hardening de produção.

### Fluxo de uso

1. O worker reconhece um formulário de login e prepara o uso da conta vinculada
   à origem atual.
2. O usuário confirma explicitamente.
3. O control valida approval, owner, browser e origem novamente.
4. O control decifra a credencial somente em memória e a entrega ao worker por
   canal interno autenticado.
5. O worker preenche e envia o formulário em uma única operação.
6. Os buffers são apagados e o evento é auditado sem valores secretos.
7. MFA, OTP, passkey ou CAPTCHA continuam exigindo takeover humano.

O modelo nunca recebe a senha, mas controlará a sessão depois do login. Contas
bancárias, e-mail principal, administradores e contas capazes de recuperar
outras contas devem continuar exclusivamente no takeover humano.

### Por que não cifrar somente no navegador

Se apenas o dispositivo do usuário possuir a chave, o servidor não conseguirá
usar a senha quando uma automação for solicitada pelo ChatGPT. Para automação
server-side, o servidor precisa conseguir decifrar. A proteção realista do MVP é
separar a chave do banco, restringir a descriptografia ao control, exigir origem
exata e approval, e nunca devolver o segredo.

Integração com Bitwarden, 1Password Connect, OpenBao ou outro vault pode ser
adicionada futuramente como backend alternativo.

## Dashboard e experiência visual

### Rotas

```text
/login
/register
/dashboard/browsers
/dashboard/credentials
/dashboard/chatgpt
/dashboard/settings
```

### Sidebar

- **Navegadores** — lista e gerenciamento;
- **Credenciais** — contas salvas por origem;
- **Conectar ao ChatGPT** — endpoint, status e passo a passo;
- **Configurações** — conta e segurança;
- menu de usuário com logout no rodapé.

No mobile, a sidebar será exibida em `Sheet`.

### Grid de navegadores

Cada card terá proporção aproximada de 16:9 e conterá:

- screenshot atual ou `Skeleton` durante carregamento;
- nome do navegador;
- status com `Badge`;
- domínio/título atual, quando permitido;
- menu de ações com renomear, iniciar/parar e excluir;
- indicação do browser padrão usado pelo MCP.

Comportamento:

1. o primeiro clique seleciona o card e ativa um preview por screenshots com
   polling curto;
2. um segundo clique na área do preview, ou o botão “Abrir navegador”, cria um
   ticket e abre o viewer interativo;
3. o viewer usa um `Dialog` com cerca de `90vw x 90vh` e título acessível;
4. no mobile, o viewer ocupa a tela inteira;
5. fechar o Dialog revoga o ticket e encerra o proxy do stream.

Não abriremos um iframe/stream completo em todos os cards. Além do consumo de
CPU e rede, isso exporia várias sessões autenticadas simultaneamente no DOM do
dashboard.

### Componentes shadcn/ui previstos

- `Sidebar` e `Sheet` para navegação;
- `Card`, `Badge`, `Skeleton` e `Empty` para a grade;
- `Dialog` para criação, edição e viewer;
- `AlertDialog` para exclusão;
- `DropdownMenu` para ações de cada browser;
- `FieldGroup`, `Field`, `Input` e validação acessível para formulários;
- `Button`, `Tooltip`, `Separator` e `sonner` para ações e feedback.

Usaremos tokens semânticos do tema, composição padrão dos componentes e `gap-*`
para espaçamento. O visual proposto é dark-first, com superfícies neutras,
contraste alto no conteúdo do navegador, tipografia limpa e movimento discreto.
Não serão criados cards, alerts ou modais paralelos quando já houver componente
shadcn correspondente.

## Preview e viewer interativo

### Preview dos cards

- endpoint autenticado `GET /api/browsers/{id}/preview`;
- screenshot reduzido, preferencialmente WebP ou JPEG;
- atualização somente para card visível/selecionado;
- intervalo inicial entre 2 e 5 segundos;
- `Cache-Control: private, no-store`;
- nenhuma persistência no PocketBase por padrão.

### Viewer

Usaremos `view.browser.lspr.dev` como host único. O control emitirá um ticket de
uso único com owner, browser, expiração curta e nonce. O viewer troca o ticket
por um cookie `HttpOnly` e remove o token da URL antes de iniciar o proxy HTTP e
WebSocket para a GUI do Chromium.

O primeiro spike deve confirmar como a GUI Selkies da imagem LinuxServer lida
com proxy, cookies, WebSocket e `iframe`. Se ela exigir caminhos absolutos
incompatíveis, alternativas em ordem de preferência:

1. manter um host fixo e uma sessão de viewer ativa por usuário;
2. usar subdomínios dinâmicos `b-<id>.browser.lspr.dev` com ForwardAuth;
3. substituir somente a camada de viewer, preservando Chromium e CDP.

Esse spike ocorre antes de finalizar os cards, pois é o maior risco técnico da
interface.

## Integração MCP com ChatGPT

O endpoint continuará público em:

```text
https://mcp.browser.lspr.dev/mcp
```

O control plane será OAuth resource server e authorization server. O login do
OAuth reutilizará o usuário do PocketBase; o access token terá `sub` igual ao id
do usuário e audience igual ao endpoint MCP.

Fluxo inicial:

1. usuário escolhe um browser padrão no dashboard;
2. conecta o Navego no ChatGPT usando o endpoint acima;
3. OAuth identifica o usuário e concede os scopes;
4. o grant OAuth da conexão é vinculado ao browser padrão e os access tokens
   carregam uma referência verificável a esse vínculo;
5. mudar o padrão afeta somente novas conexões ou uma reautorização; um grant já
   emitido permanece vinculado para não trocar de navegador no meio da tarefa;
6. futuramente uma tool explícita poderá listar e selecionar browsers.

A página “Conectar ao ChatGPT” mostrará:

- botão para copiar a URL MCP;
- botão para abrir a área de Plugins do ChatGPT;
- instruções para ativar Developer mode quando necessário;
- estado da autorização e opção de revogar;
- browser que será associado a novas autorizações;
- teste simples: abrir `example.com` e capturar screenshot.

Não há fluxo oficial documentado que permita ao Navego cadastrar sozinho um
plugin privado no ChatGPT com um clique. O atalho será um wizard guiado: a
documentação atual orienta abrir Plugins, clicar no botão de adicionar e informar
o endpoint HTTPS com `/mcp`.

Requisitos oficiais que serão preservados:

- Streamable HTTP público por HTTPS;
- protected resource metadata;
- OAuth 2.1 authorization code com PKCE S256;
- `resource` propagado e validado como audience;
- CIMD, DCR ou cliente predefinido;
- validação de issuer, scopes, expiração e usuário em todas as chamadas.

Referências:

- [Autenticação de plugins](https://developers.openai.com/plugins/build/auth)
- [Conectar e testar o MCP no ChatGPT](https://developers.openai.com/plugins/deploy/connect-chatgpt)

## Fluxos de gerenciamento

### Criar browser

1. `POST /api/browsers` cria registro `provisioning`.
2. O control solicita `EnsureBrowser` ao agent.
3. O agent cria volume, Chromium e worker com labels e limites.
4. Health checks validam GUI, CDP e worker.
5. O registro passa para `running` e o primeiro preview aparece.

Criação será idempotente. Repetir a solicitação com o mesmo id não poderá criar
volumes ou containers extras.

### Renomear

Atualiza somente o PocketBase. O nome técnico do container usa id imutável e não
é alterado.

### Parar e iniciar

Parar preserva o volume. Iniciar reconcilia a versão do runtime antes de subir.
Atualização automática de imagem não ocorrerá no meio de uma sessão.

### Excluir

`AlertDialog` exigirá confirmação e oferecerá duas intenções distintas:

- remover runtime e manter o perfil para recuperação;
- remover runtime e excluir definitivamente o perfil.

A segunda opção deve ser visualmente destrutiva e não será o padrão. A operação
marca `deleting`, remove apenas objetos com labels correspondentes ao owner/id e
finaliza o registro depois da confirmação do Docker.

## Estrutura desejada do repositório

```text
/
├── cmd/
│   ├── navego-control/
│   ├── navego-agent/
│   ├── navego-worker/
│   └── mcp-smoke/
├── internal/
│   ├── browser/
│   ├── control/
│   ├── runtime/
│   ├── vault/
│   ├── oauth/
│   └── ...
├── pb_migrations/
├── web/
│   ├── src/app/
│   ├── src/components/
│   ├── src/lib/
│   ├── public/
│   ├── Dockerfile
│   ├── components.json
│   └── package.json
├── docker/
│   └── chromium/
├── docs/
├── Dockerfile
├── compose.yaml
├── compose.ngrok.yaml
└── compose.dokploy.yaml
```

## Auditoria dos arquivos atuais

### Por que existem vários Compose

Os arquivos atuais usam o recurso de overlays do Docker Compose:

| Arquivo | Objetivo atual | Destino no novo desenho |
| --- | --- | --- |
| `compose.yaml` | stack local com web, control, agent e imagem dinâmica | consolidado |
| `compose.go.yaml` | adicionava o gateway Go ao Chromium único | removido |
| `compose.ngrok.yaml` | expõe o MCP local para testes futuros | mantido como overlay opcional |
| `compose.dokploy.yaml` | web, control, agent, Traefik e runtime dinâmico | base de produção pronta |
| `compose.credentials.example.yaml` | montava manifesto e senha por Docker Secrets | removido; CRUD do vault pronto, entrega ao worker pendente |

A separação anterior evitava duplicar a configuração do Chromium entre local,
Go e ngrok. Ela fazia sentido para o teste incremental de um navegador, mas
ficou difícil de entender e não representa uma plataforma dinâmica.

Os arquivos antigos foram removidos somente depois de o novo Compose reproduzir
o provisionamento, preview, viewer e exclusão no smoke local.

### Para que serve a pasta `docker`

Ela não é um resíduo do projeto Node antigo. Contém a customização necessária da
imagem LinuxServer Chromium:

- `docker/chromium/Dockerfile`: fixa a imagem base e instala os scripts;
- `start-browser`: acrescenta as flags de CDP ao wrapper do Chromium;
- `autostart` e `autostart-wayland`: iniciam o wrapper nos dois compositores;
- `install-autostart`: reaplica os launchers em perfis persistentes já criados,
  porque a imagem base copia defaults somente na criação inicial.

Sem essa pasta, um volume antigo poderia iniciar Chromium sem CDP e o worker
perderia o controle. Portanto, ela deve permanecer. Podemos reorganizar os
scripts sob `docker/chromium/` e reduzir duplicação depois de um smoke X11 e
Wayland, mas não há ganho em removê-la apenas por estética.

### Outras limpezas planejadas

- mover planos históricos substituídos para `docs/archive/` ou removê-los após
  consolidar as decisões ainda relevantes;
- atualizar README e `implementation-status.md` para apontar a este plano;
- remover o broker baseado em manifesto somente depois de o vault ter testes de
  não vazamento e um caminho de migração;
- manter `mcp-smoke`, pois ele protege o contrato mais importante do projeto;
- avaliar `cmd/mcp-call` após o novo inspector/test client existir;
- usar um único Dockerfile Go com targets para control, agent e worker, em vez
  de três receitas quase iguais;
- adicionar `/web/node_modules`, `.next` e artefatos de PocketBase aos ignores;
- não versionar `pb_data`, chaves, screenshots ou perfis de navegador.

## Compose futuro

### Local

`compose.yaml` deverá subir:

- `navego-web`;
- `navego-control` com volume `pb-data`;
- `navego-agent` com Docker socket;
- rede interna estável `navego-runtime`;
- opcionalmente um proxy local para reproduzir os três hosts.

Chromiums e workers não aparecem como serviços fixos: são criados pelo agent.

### Ngrok

`compose.ngrok.yaml` continuará sendo um overlay de desenvolvimento. Para testar
OAuth, o domínio precisa ser estável e todas as URLs de issuer/audience devem
usar exatamente esse domínio. O viewer local pode continuar em loopback durante
os primeiros testes.

### Dokploy

`compose.dokploy.yaml` terá somente os serviços estáticos e usará a rede externa
do Dokploy. O agent montará o Docker socket e criará runtimes com labels fora do
projeto Compose.

Domínios propostos:

```text
https://browser.lspr.dev             dashboard
https://mcp.browser.lspr.dev/mcp     MCP e OAuth discovery
https://view.browser.lspr.dev        viewer autenticado
```

O PocketBase admin não terá router público. O MCP não ficará atrás de
Cloudflare Access, pois o cliente ChatGPT precisa alcançar discovery, OAuth e o
endpoint. Dashboard e viewer usarão a autenticação do próprio Navego; uma camada
Cloudflare Access pode permanecer apenas para administração ou durante o beta.

## Segurança mínima do MVP

- convite ou aprovação para novos usuários;
- quotas de browsers e rate limiting desde o início;
- Docker socket somente no agent;
- allowlist rígida de imagens, mounts, redes, flags e labels no agent;
- containers sem privilégios adicionais, com limites de recursos e logs;
- CDP somente em loopback dentro de cada par browser/worker;
- PocketBase superuser fora da Internet, com MFA e IP allowlist quando usado;
- backups cifrados de `pb_data` e dos volumes de perfil;
- chave do vault em Docker Secret e fora dos backups comuns;
- cookies HttpOnly/Secure, CSRF e CSP que permitam apenas o viewer conhecido;
- screenshots e respostas sempre `no-store`;
- nenhuma senha, cookie, token, conteúdo digitado ou HTML em logs/auditoria;
- approvals de uso único para login salvo e ações externas;
- delete de volume separado da remoção do runtime;
- reconciliação que só altera objetos marcados com labels do Navego.

## Limites e impedimentos conhecidos

1. **Docker socket é poder de root.** O agent reduz a superfície, mas não elimina
   o risco. Uma vulnerabilidade nele pode comprometer o host.
2. **Chromium consome memória.** O cadastro precisa de quota; um ponto de partida
   conservador é um browser por usuário e limites ajustáveis depois de medir.
3. **Containers não são VMs.** Isso é adequado para uso pessoal e usuários
   confiáveis, não para oferecer imediatamente um serviço público hostil.
4. **Proxy da GUI é o principal spike.** HTTP, WebSocket, cookies e iframe da
   imagem LinuxServer precisam ser testados antes de fechar o design do viewer.
5. **PocketBase ainda está abaixo de 1.0.** A versão será fixada, migrations serão
   revisadas e backups serão obrigatórios antes de upgrades.
6. **ChatGPT exige passo manual de conexão.** O dashboard pode guiar e copiar a
   URL, mas não há API oficial documentada para instalar automaticamente um
   plugin privado na conta do usuário.
7. **Credencial invisível não torna a sessão inofensiva.** Depois do login, o
   modelo age com os privilégios da conta; contas sensíveis continuam humanas.
8. **Um único host é intencional.** Se ele falhar, dashboard, banco e browsers
   ficam indisponíveis. Multi-host não será simulado no MVP.

## Milestones

### M0 — baseline e limpeza segura — concluído

- atualizar este plano como fonte principal;
- criar testes/smokes que preservem o Chromium atual;
- atualizar Go para 1.27;
- consolidar Compose sem apagar ainda o caminho antigo;
- preparar targets `control`, `agent` e `worker`.

Saída: o fluxo atual continua funcionando e a nova estrutura não contém código
Node legado nem arquivos sem responsabilidade clara.

### M1 — PocketBase e autenticação — base concluída

- embutir PocketBase 0.40.1 no control;
- migrations de `users`, `browsers` e `audit_events`;
- cadastro por convite, login, refresh, logout e desativação;
- cookies e BFF do Next.js;
- testes de regras de ownership.

Saída: um usuário entra no dashboard e enxerga somente seus próprios registros.

### M2 — scaffold e design do dashboard — concluído

- criar `/web` com as versões fixadas;
- inicializar shadcn via CLI e registrar `components.json`;
- implementar layout, sidebar, login, cadastro e estados vazios;
- adicionar tema, responsividade, acessibilidade e testes básicos.

Saída: dashboard real conectado ao PocketBase, ainda sem criar containers.

### M3 — agent e lifecycle de browsers — concluído

- SDK Docker no agent; API privada autenticada entre control e agent;
- create/start/stop/rename/delete com idempotência;
- volumes e labels;
- health, quotas, resource limits e reconcile no boot;
- adaptar o gateway atual para `navego-worker` dinâmico.

Saída: o usuário cria e remove Chromiums pelo dashboard sem acessar Dokploy.

### M4 — previews e viewer — implementação concluída, E2E pendente

- endpoint de screenshot reduzido;
- cards com preview sob demanda;
- spike e implementação do proxy Selkies;
- tickets curtos, Dialog 90% e revogação;
- takeover humano dentro do viewer.

Saída: primeiro clique mostra preview; segundo abre o navegador interativo.

### M5 — vault de credenciais — CRUD cifrado concluído, uso pelo worker pendente

- chave mestra e envelope AES-256-GCM — concluído;
- CRUD sem endpoint de leitura do segredo — concluído;
- associação por owner e origem exata — concluído;
- approval, decifragem just-in-time e entrega ao worker;
- redaction, screenshot guard e testes de não vazamento;
- remover o manifesto/overlay antigo depois da migração.

Saída: usuário cadastra uma conta pelo dashboard e o worker faz um login de
teste sem senha no banco em texto puro ou no ChatGPT.

### M6 — MCP multiusuário e ChatGPT — pendente

- OAuth integrado ao usuário PocketBase;
- endpoint público único e routing pelo grant OAuth;
- browser padrão e vínculo estável do grant;
- página “Conectar ao ChatGPT”;
- testes no MCP Inspector e no ChatGPT Developer mode;
- refresh, revogação e auditoria.

Saída: dois usuários não conseguem enxergar nem controlar browsers um do outro.

### M7 — deploy Dokploy e hardening — Compose inicial pronto

- Compose Dokploy final;
- Traefik para dashboard, MCP e viewer;
- backups, restore testado e rotação de chaves;
- limites, logs e métricas mínimas;
- E2E de cadastro até controle pelo ChatGPT;
- remoção dos arquivos e docs substituídos.

## Estratégia de testes

- `go test ./...`, `go test -race ./...` e `go vet ./...`;
- testes do agent contra um Docker Engine de integração, sempre filtrando por
  labels de teste;
- typecheck, lint, unit tests e build standalone via npm;
- Playwright para login, CRUD de browsers, preview, modal e credenciais;
- testes de ownership com dois usuários;
- teste de reinício/reconcile sem perder volume;
- teste de upgrade de worker preservando Chromium e perfil;
- busca automática por secrets em logs, responses e snapshots;
- MCP Inspector antes de cada teste no ChatGPT;
- conjunto fixo de prompts positivos, negativos e com confirmação.

## Ordem recomendada para começar

1. fazer o spike do viewer da imagem atual;
2. consolidar os Compose e separar worker/agent/control;
3. embutir PocketBase e implementar autenticação;
4. criar o scaffold `/web` e o dashboard sem runtime;
5. ligar o CRUD ao agent;
6. implementar previews e viewer;
7. migrar credenciais;
8. finalizar OAuth e o wizard do ChatGPT;
9. implantar no Dokploy.

O spike do viewer vem primeiro porque uma incompatibilidade ali pode mudar a
forma de expor cada Chromium. Todo o restante da arquitetura permanece útil
mesmo se a camada de GUI precisar ser substituída.
