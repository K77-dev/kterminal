# PRD — Resiliência de streaming (timeouts em camadas + retry)

## Visão Geral

O kterminal aborta turnos com o erro `context deadline exceeded (Client.Timeout or context cancellation while reading body)` durante sessões agênticas. A causa é um deadline HTTP *total* de 10 minutos fixo que não distingue um stream travado (gateway parou de enviar bytes) de um stream longo e saudável (modelo lento roteado pelo Jev, contexto grande, resposta extensa) — ambos morrem da mesma forma. Não existe retry: qualquer falha transiente derruba o turno e o usuário precisa recomeçar do snapshot. O timeout de subagente agrava o problema: 5 minutos de relógio de parede para o subtask inteiro (todas as chamadas de LLM e tools) mata subtasks agênticas legítimas no meio.

Esta funcionalidade substitui o deadline único por timeouts em camadas (primeiro byte, idle do stream, capa total), adiciona retry com backoff para falhas transientes antes do primeiro delta exibido, e muda o timeout de subagente para *stall-based* (resetado a cada progresso) — tudo configurável via `config.toml`. O paradouro é o Claude Code, que combina timeout total por requisição (default 10 min), watchdogs de idle em streaming (≥5 min), deadline de primeiro byte, retry com backoff exponencial e timeout de subagente stall-based (default 10 min).

## Objetivos

- Eliminar abortos por deadline em streams saudáveis: um stream que continua entregando dados deve sobreviver além de 10 minutos quando configurado (`request_timeout = 0` ou maior).
- Detectar streams travados em minutos (idle), não no limite total: falha rápida com causa identificável.
- Tolerar falhas transientes (429, 5xx, conexão recusada, EOF) sem intervenção do usuário, sem duplicar texto na TUI.
- Subtasks agênticas longas com progresso contínuo deixam de morrer aos 5 minutos de relógio de parede.
- Todos os knobs expostos em `config.toml` e variáveis de ambiente, com defaults seguros que não exigem configuração.

## Histórias de Usuário

- Como **usuário do kterminal em projeto grande**, quero que uma resposta longa do modelo (acima de 10 min de streaming) complete sem abortar, para não perder o turno e o custo de tokens já gastos.
- Como **usuário atrás de gateway lento**, quero que um stream travado seja detectado e retentado em minutos, para não ficar olhando um spinner morto até o timeout total.
- Como **usuário do modo squad/subagentes**, quero que um subagente que trabalha continuamente (tools + deltas) conclua tarefas que levam mais de 5 minutos, para que subtasks reais não morram no meio.
- Como **usuário que hitou rate limit (429)**, quero retry automático honrando `Retry-After`, para que limites temporários não derrubem meu turno.
- Como **usuário avançado**, quero configurar cada timeout via `config.toml` ou env, para adaptar o kterminal a gateways e modelos exóticos.
- Como **usuário diagnosticando problemas**, quero mensagens de erro que digam o que aconteceu e o que foi tentado (ex: "stream parado 5m sem dados após 2 tentativas"), em vez do erro cru do Go.

## Funcionalidades Principais

### REQ-001 — Timeouts de streaming em camadas

Substituir o deadline total único do cliente HTTP por três deadlines independentes e configuráveis:

- **Primeiro byte** (default 60s): cobre conexão, TLS e espera pelos headers da resposta. Gateway que não responde falha rápido em vez de ocupar o turno por 10 minutos.
- **Idle do stream** (default 5m): janela máxima sem bytes recebidos durante o streaming. Cada dado recebido reinicia a janela; expirou sem dados, o stream é abortado com erro distinguível de stall. É o detector de stream travado.
- **Capa total** (default 10m, `0` desliga): limite máximo para a requisição inteira, preservando o comportamento atual como teto de segurança configurável.

#### Critérios de Aceite

- Stream que entrega headers e depois para de enviar bytes é abortado dentro da janela de idle, com erro distinguível de stall — sem esperar a capa total.
- Gateway que não entrega headers dentro da janela de primeiro byte falha com erro distinguível de timeout de conexão/resposta.
- Stream saudável mais longo que 10 min completa quando a capa total é aumentada ou desligada.
- Cada camada é configurável independentemente via config e env.

---

### REQ-002 — Retry com backoff para falhas transientes

Retentar automaticamente chamadas de streaming que falham por causas transientes: erros de rede, EOF inesperado, HTTP 429/500/502/503/504 e timeouts das camadas de REQ-001.

- Retry **somente antes do primeiro delta exibido ao usuário**: se o modelo já streamou texto, a falha aborta o turno como hoje (a TUI emite deltas ao vivo; retentar depois duplicaria o texto renderizado).
- Backoff exponencial com jitter entre tentativas; quando a resposta traz `Retry-After`, o valor do header prevalece sobre o backoff calculado.
- Default de 2 retentativas, configurável.
- Esgotadas as tentativas, o erro final informa modelo, número de tentativas e causa.

#### Critérios de Aceite

- 429 com `Retry-After` antes de qualquer delta → nova tentativa aguarda o valor do header e pode suceder; a TUI não mostra texto duplicado.
- Falha transiente após deltas emitidos → nenhuma retentativa; turno aborta com snapshot preservado (comportamento atual).
- Sequência de falhas transientes além do limite de retentativas → erro único e final com resumo das tentativas.
- Retentativas não ocorrem para erros não transientes (ex: 401, 400, modelo inexistente).

---

### REQ-003 — Timeout de subagente stall-based

Mudar a semântica do timeout de subagente de relógio de parede total (5 min para todo o subtask) para *stall*: a janela (default 10 min) é resetada a cada evento de progresso do subagente (deltas, chamadas e resultados de tools, rotas). Subagente silencioso pela janela inteira é abortado; subagente que trabalha continuamente sobrevive indefinidamente.

#### Critérios de Aceite

- Subagente que emite progresso continuamente sobrevive além de 10 min de relógio de parede.
- Subagente que para de emitir progresso é abortado dentro da janela configurada, com mensagem de stall (não mais "timed out after 5m" fixo).
- A janela é configurável via `[agent] subagent_timeout` e env.
- Cancelamento por Esc continua derrubando subagentes imediatamente (comportamento inalterado).

---

### REQ-004 — Configuração e diagnóstico

Expor todos os knobs em `config.toml` com defaults seguros e validação clara:

- `[llm]`: `request_timeout`, `idle_timeout`, `first_byte_timeout` (durações em formato legível, ex: `"10m"`, `"90s"`) e `max_retries` (inteiro).
- `[agent]` (nova seção): `subagent_timeout`.
- Env overrides: `KTERMINAL_LLM_REQUEST_TIMEOUT`, `KTERMINAL_LLM_IDLE_TIMEOUT`, `KTERMINAL_LLM_FIRST_BYTE_TIMEOUT`, `KTERMINAL_LLM_MAX_RETRIES`, `KTERMINAL_AGENT_SUBAGENT_TIMEOUT`.
- Duração inválida no config → erro de inicialização com mensagem apontando a chave.
- `--doctor` exibe os valores efetivos de cada timeout e do retry.

#### Critérios de Aceite

- Sem config, o comportamento default é: 60s primeiro byte, 5m idle, 10m total, 2 retries, 10m stall de subagente.
- Cada knob configurável via TOML e via env, com env prevalecendo sobre o arquivo.
- `--doctor` imprime os valores efetivos.
- Valor inválido impede o boot com erro claro.

---

### REQ-005 — Erros acionáveis na TUI

Erros de timeout e exaustão de retry são renderizados com mensagem amigável e causa específica (stall de stream, timeout de primeiro byte, exaustão de retries), em vez da mensagem crua do `net/http`.

#### Critérios de Aceite

- Stall de stream exibe mensagem que menciona a janela de idle e as tentativas.
- Timeout de primeiro byte exibe mensagem que menciona a janela e sugere verificar o gateway.
- Nenhum erro cru `context deadline exceeded (Client.Timeout...)` chega à TUI vindo das camadas novas.

## Experiência do Usuário

O usuário não configura nada por padrão: os defaults cobrem o caso comum. A diferença percebida é (a) respostas longas não morrem mais aos 10 min, (b) streams travados falham em minutos com mensagem explicando o stall e as retentativas, (c) subagentes longos com progresso concluem. Durante retentativas, o estado do turno permanece visível (spinner/rota) sem texto duplicado. A TUI é terminal (Bubble Tea); não há requisitos de acessibilidade novos além dos existentes — mensagens de erro são texto plano, curtas e com a causa primeiro.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`, binário único auto-contido; sem novas dependências externas (só stdlib).
- Compatibilidade com gateways OpenAI-compatible via streaming SSE — o comportamento não pode depender de headers ou features não universais (além de `Retry-After`, padrão HTTP).
- Sem comentários no código (convenção do repo); código em inglês, docs em pt-BR.
- Sem mudança no cliente Jev (`internal/jev`, timeout 30s) — roteamento tem SLA próprio e fora de escopo.
- Defaults alinhados ao Claude Code (60s/5m/10m/2 retries/10m stall) — valores comprovados em produção.

## Fora de Escopo

- Re-render de parcial na TUI para permitir retry após o primeiro delta (evolução natural; exige evento de re-render e ajuste no transcript).
- Retry/watchdog no cliente Jev (decisão de roteamento).
- Retentativa de turnos inteiros ou de tool calls individuais.
- Métricas/telemetria novas sobre retentativas (a telemetria de TPS existente continua medindo streams completos).
- Timeouts para tools (`bash`, `vet`) — já têm limites próprios.
