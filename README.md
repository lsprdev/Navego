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
- gestão de convites pela UI e hardening final do Dokploy.

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

### Diagnosticar falhas na atividade

Em **Atividade → Ver detalhes**, cada evento mostra a mensagem de erro disponível,
a etapa, a duração da chamada no control plane e o código retornado pela ferramenta
(quando houver). **Copiar detalhes** copia o diagnóstico e o ID do evento para
correlacionar com os logs do servidor. A categoria é inferida da mensagem: timeout
de DNS, tempo limite excedido, cancelamento, transporte ou erro da operação.
Ela não comprova sozinha a causa raiz nem identifica quem cancelou a conexão.

As chamadas MCP ao worker passam a registrar também erros retornados com
`isError=true`, e não apenas erros de transporte. Não são gravados os argumentos
nem as respostas bem-sucedidas das ferramentas; mensagens têm URLs e padrões de
credenciais ocultados e tamanho limitado. O endpoint continua restrito ao dono dos
eventos e não entrega o restante dos metadados de auditoria.

Eventos antigos só exibem os detalhes que já foram salvos; não há reconstrução
retroativa. Faça rebuild/redeploy de **control** e **web** para habilitar o recurso.
Não é necessário atualizar o conector MCP ou recriar os Chromiums. O recurso não
altera timeouts nem repete ações automaticamente.

### Testes

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

O teste de menus legados usa Chrome/Chromium headless com perfil temporário
isolado e verifica snapshot, busca, hover e clique, incluindo menus baseados em
`td` com eventos de mouse como os do SIGAA. Também verifica a busca de menus
além dos primeiros 150 controles: `browser_find` prioriza correspondências
antes de aplicar o limite de elementos e retorna referências acionáveis.

```sh
NAVEGO_TEST_CHROME=/caminho/para/chromium go test -tags=integration ./internal/browser -run TestLegacyMenuSnapshotAndInteractions -count=1
```

## Deploy

### Acesso restrito e capacidade

Configure no ambiente do control plane (no Dokploy, em **Environment**):

```dotenv
NAVEGO_ALLOWED_EMAILS=voce@example.com,amigo@example.com
NAVEGO_MAX_BROWSERS_PER_USER=2
NAVEGO_MAX_BROWSERS_TOTAL=5
```

- A lista aceita endereços completos, separados por vírgula, sem curingas ou
  domínios inteiros. A comparação ignora maiúsculas/minúsculas. Lista vazia
  bloqueia usuários; o compose de produção exige preenchimento antes de subir.
- Além de estar na lista, a conta precisa ter `verified=true` no PocketBase.
  O cadastro cria uma conta pendente, sem sessão automática. Neste fluxo de
  acesso privado, o administrador confere a identidade e verifica a conta na
  coleção `users`, usando o acesso administrativo interno/SSH já protegido.
  Não libere contas apenas porque alguém digitou um e-mail conhecido e não
  exponha o painel administrativo publicamente. Envio automático de verificação
  por e-mail não foi configurado por esta alteração.
- **Antes de atualizar**, inclua também os e-mails das contas existentes e
  verifique-as; caso contrário, elas perderão acesso ao dashboard e ao MCP.
  Alterações na lista exigem redeploy do control plane. Tokens antigos do
  dashboard e do MCP também passam pela política; refresh OAuth não libera
  contas removidas. Superusuários mantêm o acesso administrativo.
- Os limites contam todos os perfis: ligados, desligados, em criação, com erro
  ou aguardando exclusão. A vaga só volta após a exclusão ser confirmada pelo
  agente. A contagem e a criação ocorrem na mesma transação, impedindo que
  requisições simultâneas ultrapassem a cota. A API nativa de criação de
  registros de navegadores continua fechada para usuários.
- Reduzir o limite não apaga nem desliga perfis existentes: novas criações
  ficam bloqueadas até haver capacidade. Ajuste o total à memória disponível:
  cada Chromium pode consumir até 2 GiB, além do worker e dos demais serviços.
  O limite de quantidade não substitui monitoração de RAM, CPU e disco.

Rebuild/redeploy do `navego-control` e do `navego-web` aplica estas alterações.

[`compose.dokploy.yaml`](compose.dokploy.yaml) prepara:

- `https://navego.lspr.dev` para dashboard e viewer;
- `https://mcp.navego.lspr.dev/mcp` para MCP e OAuth;
- Traefik somente na frente das rotas públicas necessárias;
- control, agent, Docker socket e rede dos Chromiums fora da exposição direta.

Cloudflare Access continua recomendado para o dashboard. O viewer exige também
o ticket interno do Navego, então conhecer a URL base não concede acesso a um
Chromium.

O agente recupera automaticamente workers que perderam a conexão com o Chromium:

- Após um reinício externo do container do Chromium (inclusive reboot do host),
  compara os horários de início e reinicia o worker que ainda é anterior a ele.
- Após um rebuild da imagem configurada em `NAVEGO_WORKER_IMAGE`, compara o ID
  imutável da imagem com o do worker existente e substitui somente o worker
  desatualizado. A imagem nova precisa estar disponível localmente; se não
  estiver, o agente informa o erro sem remover o worker antigo.
- Se `/healthz` continuar falhando por pelo menos dois minutos e três sondagens,
  reinicia somente o worker, com intervalo mínimo de dois minutos entre tentativas.
  Uma resposta saudável limpa a contagem; uma falha isolada não provoca reinício.
- Não apaga o perfil/volume do Chromium e não repete comandos MCP interrompidos.
  Uma ação interrompida deve ter seu resultado conferido antes de ser repetida.

Essa recuperação exige rebuild e redeploy do serviço `navego-agent`; também
monitora os workers já existentes, sem precisar recriar os navegadores. Nos logs
do agente, a recuperação por falhas persistentes aparece como
`restarting unresponsive browser worker`.

Para atualizar o código MCP, faça rebuild da imagem `navego-runtime:production`
e redeploy do agente. Apenas recarregar o plugin ou reiniciar o container do
worker não troca o binário antigo. A substituição automática aparece nos logs
como `replacing outdated browser worker`; faça deploy sem ações em andamento,
pois chamadas que estiverem usando o worker substituído podem ser interrompidas.
