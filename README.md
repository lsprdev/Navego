# Navego

Plataforma em Go para criar e controlar Chromiums persistentes pelo dashboard e
pelo ChatGPT via MCP.

## Arquitetura atual

```text
Next.js BFF ── PocketBase + control plane Go ── Navego Agent ── Docker Engine
                                                        │
                                           Chromium + worker Go
                                           por navegador criado
```

- `web/`: dashboard Next.js 16, React 19, Tailwind e shadcn/ui;
- `cmd/navego-control`: PocketBase, autenticação, estado desejado, previews e
  tickets curtos do viewer;
- `cmd/navego-agent`: único processo com acesso ao socket Docker;
- `cmd/navego`: worker MCP que acompanha cada Chromium;
- `docker/chromium/`: imagem Chromium customizada e scripts de inicialização.

O dashboard nunca recebe o token PocketBase no JavaScript: as Route Handlers do
Next.js atuam como BFF e guardam a sessão em cookie HttpOnly. O agent aceita
somente uma configuração Docker definida no código, valida labels de ownership e
não expõe uma API de `docker run` arbitrário.

## Subir localmente

Pré-requisitos: Docker Compose v2. O Go local é necessário somente para testes e
desenvolvimento fora dos containers.

```sh
cp .env.example .env
docker compose --profile images build
docker compose up --build -d
docker compose ps
```

Abra:

- dashboard: `http://127.0.0.1:3000`;
- health do control plane: `http://127.0.0.1:8090/api/navego/healthz`.

Ao criar um navegador, o agent provisiona um volume de perfil e dois containers
com nomes derivados do ID do registro. Eles não publicam CDP nem portas no host.
O primeiro clique no card carrega uma captura estática; o segundo emite um ticket
de uso único e abre o Selkies em um diálogo de 90% da tela.

> O socket Docker concede poder equivalente a root no host. Ele é montado apenas
> no `navego-agent`. Este MVP é indicado para um servidor próprio e usuários
> confiáveis, não para cadastro público hostil.

## Estado das funcionalidades

Já implementado:

- cadastro, login, logout e sessão HttpOnly;
- perfil, troca de senha e trilha de atividade;
- isolamento dos registros por usuário no PocketBase;
- criar, renomear, ligar, desligar e excluir navegadores;
- reconciliação idempotente pelo agent, volumes, labels e limites de recursos;
- previews PNG autenticados;
- viewer reverso com ticket curto, cookie HttpOnly e suporte a WebSocket;
- takeover do ChatGPT com link autenticado para o dashboard, validação da mesma
  conta Navego e abertura automática do Chromium correto no diálogo de 90%;
- heartbeat com título e URL atuais dos Chromiums;
- CRUD de acessos cifrados com AES-256-GCM, sem leitura da senha pela API;
- worker MCP com navegação, handoff humano cooperativo, screenshots e ações
  externas vinculadas por approval de uso único;
- endpoint MCP público multiusuário, OAuth 2.1 com PKCE e tokens opacos;
- seleção explícita de Chromium por nome/ID e fallback configurável no dashboard.

Ainda em desenvolvimento:

- entrega just-in-time do vault ao worker com approval de uso único;
- convite/limites configuráveis e hardening final do Dokploy.

O atalho “Conectar ao ChatGPT” mostra a URL do endpoint `/mcp`. A conexão OAuth
dá acesso apenas aos navegadores do usuário autenticado. O ChatGPT pode chamar
`browser_list_instances`, informar `browser: "Nome"` em cada operação ou omitir
o seletor para usar o Chromium marcado como padrão.

### Testar com ngrok

Use o domínio HTTPS de desenvolvimento atribuído à sua conta ngrok:

```sh
# em .env
NGROK_AUTHTOKEN=...
NGROK_URL=https://seu-dominio-atribuido.ngrok-free.app

docker compose -f compose.yaml -f compose.ngrok.yaml up -d --build
```

Depois, adicione `https://seu-dominio-atribuido.ngrok-free.app/mcp` no modo de
desenvolvedor do ChatGPT. O cliente fará o registro dinâmico e abrirá o login do
Navego.

O túnel acima expõe somente o control plane/MCP. Em desenvolvimento,
`NAVEGO_PUBLIC_DASHBOARD_URL=http://127.0.0.1:3000` faz o ChatGPT devolver um
link que abre o dashboard na máquina do usuário. Ao encontrar login, MFA,
passkey, OTP ou CAPTCHA, o ChatGPT deve chamar o takeover no mesmo turno e já
mostrar esse link. Se a sessão do dashboard estiver ausente, o Navego preserva o
destino durante o login; uma conta diferente da vinculada no OAuth recebe uma
mensagem de acesso incompatível.

O controle humano não é uma trava permanente: a próxima ferramenta de navegador
retoma a automação automaticamente. Para posts, mensagens e formulários, um
pedido imperativo que já informe conteúdo e destino vale como autorização da
ação exata; o worker ainda executa `prepare -> commit` no mesmo turno para
validar página, campos e impedir replay. Compras, pagamentos, exclusões e logout
continuam exigindo confirmação final separada.

O domínio precisa ser exatamente o domínio de desenvolvimento exibido no painel
da mesma conta do `NGROK_AUTHTOKEN`; não escolha ou reutilize um subdomínio
aleatório. A inspeção HTTP local do ngrok fica desativada neste overlay para que
corpos de formulários de autenticação não sejam armazenados.

O cofre local usa `NAVEGO_VAULT_KEY`. A chave de desenvolvimento do exemplo é
intencionalmente pública; gere uma chave exclusiva com `openssl rand -base64 32`
antes de salvar qualquer credencial real ou fazer deploy.

## Validar

```sh
go test ./...
go vet ./...
cd web
npx tsc --noEmit
npm run lint
npm run build
```

O plano e as decisões de arquitetura estão em
[`docs/dashboard-platform-plan.md`](docs/dashboard-platform-plan.md).

## Deploy

[`compose.dokploy.yaml`](compose.dokploy.yaml) prepara:

- `https://browser.lspr.dev` para dashboard e viewer;
- `https://mcp.browser.lspr.dev/mcp` para MCP e OAuth;
- Traefik somente na frente das rotas públicas necessárias;
- control, agent, Docker socket e rede dos Chromiums fora da exposição direta.

Cloudflare Access continua recomendado para o dashboard. O viewer exige também
o ticket interno do Navego, então conhecer a URL base não concede acesso a um
Chromium.
