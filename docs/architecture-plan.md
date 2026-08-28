# Planejamento de arquitetura e segurança

Status: baseline para a versão pessoal e privada do Navego.

Evolução deste desenho: [arquitetura Chromium-only em Go](./chromium-only-plan.md).

## Objetivo

Disponibilizar ao ChatGPT um Chromium persistente por meio de um servidor MCP,
permitindo que o modelo navegue, leia páginas, prepare ações e capture imagens.
Quando um site exigir senha, passkey, 2FA, CAPTCHA ou outra autenticação privada,
a automação deve pausar para que o usuário assuma o mesmo navegador por uma
interface gráfica protegida. Depois do login manual, o MCP retoma a tarefa.

Exemplos desejados:

- pesquisar notícias atuais, preparar um texto e publicá-lo no X após confirmação;
- consultar, sem alterar, os produtos existentes no carrinho da Amazon;
- acessar o portal de uma faculdade e devolver um screenshot da página principal;
- manter cookies e sessões entre reinicializações do serviço.

## Viabilidade geral

A proposta é altamente viável para uso pessoal e privado. O protótipo atual já
comprovou o núcleo do fluxo: Chromium persistente, MCP por Streamable HTTP,
controle por CDP, login humano e retomada da automação.

O projeto atual não deve, entretanto, ser exposto permanentemente sem uma etapa
de endurecimento. Ele ainda oferece ferramentas de execução arbitrária e inicia
o Chromium sem o sandbox interno do navegador.

Uma versão pública ou multiusuário seria outro produto: exigiria OAuth 2.1,
isolamento de navegador e perfil por usuário, limites, auditoria e uma política
de privacidade própria.

## Arquitetura recomendada

O projeto deve ter dois planos de acesso independentes.

### Plano de controle do ChatGPT

```text
ChatGPT
   |
   v
OpenAI Secure MCP Tunnel
   |  conexão HTTPS iniciada pelo servidor
   v
MCP Gateway ------> Chromium persistente via CDP
```

Para a instalação pessoal, o MCP deve permanecer em uma rede privada do Docker.
O Secure MCP Tunnel oferece ao ChatGPT um caminho autenticado sem abrir a porta
do MCP para a internet. A disponibilidade depende do Developer Mode e das
permissões do workspace.

O ngrok continua útil para desenvolvimento e testes rápidos, mas não é a opção
preferida para a instalação permanente.

Referências:

- [Secure MCP Tunnels](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels)
- [Conectar um plugin ao ChatGPT](https://developers.openai.com/plugins/deploy/connect-chatgpt)

### Plano de acesso humano

```text
Usuário
   |
   v
https://browser.seudominio.com
   |
Cloudflare Access: e-mail autorizado + MFA ou OTP
   |
Cloudflare Tunnel
   v
Interface gráfica do Chromium
```

A interface gráfica deve possuir uma URL estável, mas não deve usar um token
permanente na query string. Tokens em URLs podem vazar por histórico, logs,
screenshots, analytics e cabeçalhos de referência.

O acesso recomendado é Cloudflare Tunnel com Cloudflare Access, liberando
somente o e-mail do proprietário. A origem permanece sem uma porta pública. Uma
VPN como Tailscale ou WireGuard é uma alternativa ainda mais privada, com o custo
de exigir o cliente VPN em cada dispositivo.

Referências:

- [Cloudflare Tunnel](https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/)
- [Cloudflare Access policies](https://developers.cloudflare.com/cloudflare-one/access-controls/policies/)
- [Cloudflare One-time PIN](https://developers.cloudflare.com/cloudflare-one/integrations/identity-providers/one-time-pin/)

## Fluxo de autenticação humana

1. O ChatGPT navega até a página desejada.
2. O site solicita senha, passkey, CAPTCHA, 2FA ou aprovação em outro dispositivo.
3. O MCP chama `browser_request_human_login`.
4. O servidor adquire um bloqueio exclusivo e pausa todas as ações de automação.
5. O ChatGPT devolve a URL estável da interface gráfica.
6. O usuário digita as credenciais diretamente no site, nunca no chat.
7. O usuário clica em `Concluir login` na GUI ou responde `pronto` no chat.
8. O MCP captura um novo snapshot e retoma a tarefa.
9. Se a próxima ação produzir um efeito externo, o ChatGPT solicita confirmação.

Enquanto o estado for `HUMAN_ACTIVE`, nenhuma ferramenta de clique, digitação,
navegação ou avaliação deve poder operar. Isso evita uma disputa entre usuário e
modelo sobre a mesma aba.

Uma máquina de estados simples é suficiente:

```text
AUTOMATION_ACTIVE -> HUMAN_REQUIRED -> HUMAN_ACTIVE -> RESUMING
        ^                                             |
        +---------------------------------------------+
```

As senhas nunca passam pelo MCP. Depois do login, entretanto, os cookies daquele
perfil concedem à automação o mesmo acesso da sessão humana e devem ser tratados
como credenciais sensíveis.

## Política para ações sensíveis

Podem ser executadas sem confirmação adicional:

- navegar e pesquisar;
- ler conteúdo;
- consultar um carrinho sem modificá-lo;
- capturar um screenshot;
- preparar um rascunho;
- preencher campos que ainda não serão enviados.

Exigem confirmação imediatamente antes do efeito externo:

- publicar no X ou outra rede social;
- enviar mensagens, e-mails ou formulários;
- alterar itens de um carrinho;
- comprar, pagar ou assinar;
- excluir conteúdo;
- alterar dados ou configurações de uma conta.

O desenho recomendado para escritas é `prepare` e `commit`. A primeira operação
preenche ou descreve a mudança. A segunda exige um identificador curto de
aprovação criado somente depois de o usuário confirmar o conteúdo final.

Para o pedido "publique no X sobre as últimas notícias", o fluxo deve:

1. pesquisar fontes atuais e confiáveis;
2. produzir um rascunho e apresentar as fontes;
3. preencher o compositor do X sem publicar;
4. mostrar ao usuário o texto final;
5. pedir confirmação;
6. clicar em `Postar` somente após a aprovação.

Referência:

- [Segurança e privacidade para plugins](https://developers.openai.com/plugins/guides/security-privacy)

## Conjunto de ferramentas MCP

O modelo não deve receber todo o Playwright ou uma API genérica de JavaScript.
Uma camada pequena e orientada a objetivos reduz riscos, falhas e consumo de
tokens.

Conjunto inicial sugerido:

- `browser_open`
- `browser_read_page`
- `browser_find`
- `browser_click`
- `browser_type`
- `browser_wait`
- `browser_tabs`
- `browser_take_screenshot`
- `browser_request_human_login`
- `browser_prepare_external_action`
- `browser_commit_external_action`

Ferramentas como execução arbitrária de código, avaliação livre de JavaScript e
upload de caminhos do sistema devem ficar indisponíveis por padrão.

As ferramentas devem ter schemas pequenos, respostas estruturadas, descrições
estáveis e anotações corretas de leitura, escrita, destrutividade e acesso ao
mundo externo.

Referência:

- [Construção de servidores MCP](https://developers.openai.com/plugins/build/mcp-server)

## Screenshots

O MCP deve oferecer captura de:

- viewport atual;
- página completa;
- elemento específico;
- PNG por padrão, com JPEG e WebP opcionais.

A resposta preferida é um bloco MCP do tipo `image`, permitindo que o ChatGPT
exiba a imagem diretamente. Se a integração do ChatGPT não renderizar o bloco,
o fallback será um link autenticado e de curta duração.

Screenshots podem conter dados pessoais e não devem permanecer indefinidamente
no volume. O arquivo deve ser removido depois da entrega ou por uma rotina de
expiração curta.

## Segurança do navegador e da rede

Antes da publicação permanente:

- remover completamente ferramentas equivalentes a RCE;
- desativar avaliação livre de JavaScript e upload por padrão;
- restaurar o sandbox do Chromium ou reforçar o isolamento do contêiner;
- manter CDP, MCP e a origem da GUI fora da internet pública;
- bloquear navegação para localhost, redes privadas e endpoints de metadados;
- permitir somente `http` e `https` para navegação normal;
- não montar o Docker socket;
- limitar CPU, memória, processos e armazenamento;
- usar usuário não root para MCP e Chromium;
- restringir capabilities e mounts do contêiner;
- armazenar o perfil do navegador em disco criptografado;
- guardar secrets fora da imagem e nunca incluí-los nos logs.

Bloqueios de rede devem cobrir, pelo menos:

- loopback;
- redes privadas IPv4 e IPv6;
- link-local;
- serviços internos da rede Docker;
- endpoints de metadados como `169.254.169.254`.

## Prompt injection e páginas não confiáveis

Todo conteúdo vindo de uma página deve ser considerado não confiável. Texto de
um site não pode alterar a política do MCP, solicitar arquivos locais, conceder
aprovações ou decidir que uma ação externa deve ser executada.

As proteções principais são:

- ferramentas mínimas e específicas;
- validação do domínio e destino no servidor;
- bloqueio de arquivos e código arbitrário;
- confirmação humana fora do conteúdo da página;
- separação entre `prepare` e `commit`;
- auditoria de cada ação externa.

## Persistência e privacidade

O volume do Chromium contém cookies, tokens de sessão, histórico e possivelmente
dados baixados. Ele deve ser tratado como um cofre de credenciais.

Recomendações:

- criptografia do disco do host;
- backups criptografados ou decisão explícita de não fazer backup;
- retenção curta para screenshots, snapshots e downloads;
- nenhum DOM completo ou credencial em logs;
- logs de auditoria contendo somente horário, domínio, categoria da ação,
  resultado e identificador de aprovação;
- um único usuário e perfil no primeiro release.

Uma implantação multiusuário nunca deve compartilhar o mesmo perfil. Cada
usuário precisaria de contêiner, diretório de perfil, secrets, sessão e fila de
ações isolados.

## Observabilidade e operação

- health check do MCP e do CDP;
- reconexão automática quando o Chromium reiniciar;
- indicador de estado `AUTOMATION_ACTIVE` ou `HUMAN_ACTIVE`;
- registro de chamadas sensíveis sem conteúdo privado;
- alertas de indisponibilidade dos túneis;
- atualização periódica do Chromium e das dependências;
- teste de restauração do perfil, se houver backup;
- limpeza automática de artefatos temporários.

## Plano de implementação

### Fase 1: endurecimento local

- criar allowlist de ferramentas;
- remover código arbitrário, avaliação livre e upload;
- implementar lease exclusivo do navegador;
- implementar confirmação `prepare`/`commit`;
- bloquear destinos internos;
- implementar screenshot como imagem MCP;
- limitar retenção de artefatos;
- criar testes unitários e de integração de segurança.

Critério: uma página não consegue executar código, acessar arquivos locais nem
produzir uma escrita externa sem confirmação.

### Fase 2: interface gráfica permanente

- registrar um subdomínio;
- adicionar `cloudflared` ao Docker Compose;
- criar aplicação Cloudflare Access;
- permitir somente o e-mail do proprietário e habilitar MFA ou OTP;
- atualizar `HUMAN_TAKEOVER_URL`;
- validar WebSocket e streaming da interface;
- confirmar que a origem não responde diretamente pela internet.

Critério: o proprietário abre a GUI sem SSH e usuários não autorizados recebem
bloqueio do Access.

### Fase 3: ChatGPT privado

- adicionar o cliente do Secure MCP Tunnel ao Compose;
- manter a porta do MCP somente na rede Docker;
- registrar o túnel no ChatGPT Developer Mode;
- configurar instruções, schemas e anotações das ferramentas;
- manter ngrok somente como fallback de desenvolvimento.

Critério: o ChatGPT acessa o MCP sem uma porta pública do MCP.

### Fase 4: validação ponta a ponta

- login no X, rascunho, confirmação e publicação;
- consulta somente leitura do carrinho da Amazon;
- login no portal da faculdade e retorno do screenshot;
- CAPTCHA, 2FA, passkey e sessão expirada;
- reinício preservando login;
- tentativa de GUI sem autorização;
- duas conversas disputando o mesmo navegador;
- página tentando induzir upload, código ou publicação;
- queda e reconexão dos túneis.

### Fase 5: instalação permanente

- separar os serviços de Chromium, MCP Gateway e túneis;
- aplicar limites de recursos e política de reinício;
- configurar armazenamento e secrets;
- habilitar auditoria e limpeza;
- documentar atualização, recuperação e rotação de credenciais.

### Fase 6: distribuição pública opcional

- isolamento de perfil e navegador por usuário;
- OAuth 2.1 compatível com MCP;
- provisionamento e expiração de ambientes;
- quotas, cobrança e rate limits;
- política de privacidade e exclusão de dados;
- endpoint HTTPS estável para distribuição pública.

Referência:

- [Autenticação de MCP](https://developers.openai.com/plugins/build/auth)

## Matriz de viabilidade

| Cenário | Viabilidade | Observação |
| --- | --- | --- |
| MCP privado para uma pessoa | Alta | Melhor cenário para a arquitetura atual |
| Login manual e retomada | Alta | O núcleo já foi validado com o X |
| Link permanente protegido | Alta | Cloudflare Tunnel e Access |
| Ler carrinho da Amazon | Média/alta | Sessões e detecção de automação podem interferir |
| Publicar no X | Média/alta | A interface muda e o site pode aplicar políticas antibot |
| Screenshot enviado no chat | Alta | Pode exigir wrapper MCP específico para imagem |
| CAPTCHA, passkey e 2FA | Média | O usuário resolve, mas o site ainda pode bloquear a automação |
| Compras totalmente autônomas | Baixa | Deve sempre existir confirmação final |
| Plugin público multiusuário | Baixa na arquitetura atual | Exige isolamento e OAuth |
| Chromium compartilhado por várias pessoas | Inviável com segurança | Misturaria cookies, histórico e contas |

## Decisão recomendada

O primeiro release deve ser pessoal, privado e single-user:

- Secure MCP Tunnel para o plano de controle;
- Cloudflare Tunnel e Access para a GUI;
- nenhuma credencial permanente na URL;
- login humano com bloqueio exclusivo;
- confirmação obrigatória antes de qualquer efeito externo;
- conjunto mínimo de ferramentas MCP;
- nenhuma execução arbitrária de código;
- perfil persistente protegido e retenção curta de artefatos.
