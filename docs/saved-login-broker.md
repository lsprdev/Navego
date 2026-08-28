# Broker de logins salvos

O broker permite que o Navego autentique uma conta sem enviar usuário ou senha
ao ChatGPT, ao protocolo MCP ou aos logs. Ele é opcional; sem manifesto, o fluxo
de takeover humano continua sendo o padrão.

## Limite de segurança

O modelo não recebe a senha, mas passa a controlar uma sessão autenticada depois
do login. Isso significa que ele pode ler e agir com os privilégios daquela
conta, respeitando as confirmações do Navego. Uma página legítima comprometida
no domínio autorizado também recebe a credencial durante o login, como receberia
em um login manual.

Use somente contas dedicadas e de baixo privilégio. Não configure e-mail
principal, banco, conta administrativa, gerenciador de senhas nem qualquer conta
capaz de recuperar as demais. MFA, passkey e CAPTCHA continuam humanos.

PocketBase não deve ser usado como cofre de senhas. O MVP usa Docker secrets
montados somente no gateway; um vault dedicado pode substituir os arquivos no
futuro.

## Proteções implementadas

- uma credencial por origem HTTPS exata (`scheme + host + porta`);
- manifesto e arquivos de secret fora da imagem e do Git;
- caminhos absolutos confinados a `MCP_SAVED_LOGIN_SECRETS_DIR`, inclusive após
  resolução de symlinks;
- arquivos regulares, limites de tamanho e manifesto com schema fechado;
- formulário vinculado à URL, geração e refs de username, password e submit;
- `prepare -> confirmação explícita -> commit` com approval de uso único e TTL;
- leitura dos arquivos somente no commit;
- preenchimento e submit em uma única operação, sem snapshot intermediário;
- password inputs e valores protegidos continuam mascarados nos snapshots;
- screenshot e PDF são recusados enquanto um valor protegido aparecer em texto
  visível, URL ou campo não-password da página;
- nenhuma credencial em respostas ou logs do gateway.

Essa proteção reduz a exposição ao modelo; ela não transforma o Chromium ou o
site visitado em ambiente confiável. O host deve usar disco criptografado, acesso
administrativo restrito e backups protegidos.

## Configuração local

Crie o diretório ignorado pelo Git e copie o manifesto de exemplo:

```bash
mkdir -p secrets
cp docs/saved-logins.example.json secrets/navego-logins.json
```

Edite somente `id`, `label`, `origin` e os nomes de arquivo. A origem aceita
apenas HTTPS e não pode conter path, query, fragment ou credenciais. Depois crie
os dois arquivos sem adicionar seus conteúdos ao `.env`:

```bash
printf '%s' 'usuario-da-conta' > secrets/example-username
printf '%s' 'senha-da-conta' > secrets/example-password
chmod 600 secrets/*
```

Suba o overlay de credenciais:

```bash
docker compose \
  -f compose.yaml \
  -f compose.go.yaml \
  -f compose.credentials.example.yaml \
  up --build -d
```

Os nomes dos Docker secrets no overlay precisam corresponder aos caminhos do
manifesto. Para mais contas, adicione dois secrets e uma entrada de manifesto
por origem.

## Fluxo MCP

1. O modelo abre a página e identifica os refs de usuário, senha e botão.
2. `browser_prepare_saved_login` verifica o formulário e encontra a única conta
   vinculada à origem atual.
3. O usuário vê apenas label e origem e confirma explicitamente.
4. `browser_commit_saved_login` consome o approval, lê os arquivos, preenche os
   campos e envia o formulário uma vez.
5. MFA, OTP, passkey ou CAPTCHA acionam o takeover humano normalmente.

Se a página, URL, origem, refs ou labels mudarem entre prepare e commit, o login
é recusado e deve ser preparado de novo.

## Dokploy

Monte o manifesto e cada credencial como Docker secrets somente no serviço
`navego-gateway`, todos sob `/run/secrets`. Depois configure:

```text
MCP_SAVED_LOGINS_FILE=/run/secrets/navego-logins.json
MCP_SAVED_LOGIN_SECRETS_DIR=/run/secrets
```

Não monte esses secrets no container do Chromium e não os coloque em variáveis
de ambiente do Dokploy. O navegador recebe a credencial apenas no momento do
commit aprovado.
