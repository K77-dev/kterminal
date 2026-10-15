# PRD — Controle do Jev: toggle de roteamento e modelo fixo

## Visão Geral

O Jev é habilitado hoje de forma implícita: o `JevRouter` só é instanciado quando `typesafe.api_key` está presente, e quem tem a chave não consegue desligá-lo sem remover a chave da config. Sem a chave, o `HeuristicRouter` decide o modelo por keywords, variando a escolha a cada passo — comportamento que surpreende quem espera determinismo. O `/model <name>` existe como pin, mas é volátil: vale só para a sessão e se perde no restart.

Esta funcionalidade introduz um **toggle explícito e persistente do Jev** — campo `[typesafe] enabled` na config + comando `/jev on|off` na TUI — e, com o Jev desabilitado, **roteamento determinístico por modelo fixo**: escolhido num picker interativo com os modelos do catálogo ou via `/model`, persistido na config, com o `default_model` do catálogo como fallback quando nenhum modelo for definido.

## Objetivos

- Toggle explícito: usuário com chave Typesafe liga/desliga o Jev sem editar arquivo nem remover credenciais.
- Determinismo com Jev desabilitado: o mesmo modelo em todos os passos do agente e dos subagentes — zero decisão por keywords.
- Persistência: a escolha de modelo sobrevive a restart (config.toml).
- Zero chamadas ao Jev quando desabilitado — nem no agente, nem em subagentes/personas.
- Retro-compatibilidade: config existente sem os novos campos preserva o comportamento atual (Jev ativo com api key, heurística sem chave).
- Descoberta: estado do roteamento visível sem comandos (status bar, `--doctor`).

Métricas de sucesso: com `enabled = false`, nenhuma requisição ao endpoint Typesafe em toda a sessão; modelo fixo idêntico em 100% das rotas do transcript; escolha de modelo intacta após restart.

## Histórias de Usuário

- Como **usuário com chave Typesafe**, quero desligar o Jev temporariamente (para testar um modelo específico ou eliminar a latência da decisão de roteamento) sem remover minha chave da config.
- Como **usuário sem chave Typesafe**, quero escolher um modelo fixo do catálogo, para ter comportamento determinístico em vez do chute por keywords do `HeuristicRouter`.
- Como **usuário que escolheu um modelo**, quero que a escolha persista entre sessões, para não repetir `/model` a cada restart.
- Como **usuário que não decora nomes de modelos**, quero um picker interativo com os modelos do catálogo (preço e velocidade à vista), para escolher com setas e enter.
- Como **usuário no meio de uma sessão**, quero saber por olhada na status bar se o Jev está decidindo ou se há modelo fixo, para entender o comportamento sem rodar comandos.
- Como **usuário diagnosticando**, quero que `--doctor` mostre se o Jev está habilitado e qual modelo fixo está configurado.

## Funcionalidades Principais

### REQ-001 — Toggle persistente do Jev

Habilitar e desabilitar o Jev em runtime e na config:

- Campo `[typesafe] enabled` (bool, default `true`). Jev ativo ⇔ `enabled = true` **e** `api_key` presente.
- Comando `/jev on` habilita; `/jev off` desabilita e abre o picker de modelos (REQ-003); `/jev` sem argumentos exibe o estado atual (on/off, modelo fixo vigente, default do catálogo).
- `/jev on` sem api key configurada → erro amigável, estado inalterado.
- Toda alteração persiste na config e re-wira os routers na sessão corrente, sem restart.
- Env override `KTERMINAL_TYPESAFE_ENABLED`.

#### Critérios de Aceite

- `enabled = false` com api key presente → nenhuma chamada ao Jev na sessão inteira.
- `/jev off` persiste em `config.toml` e sobrevive restart.
- `/jev on` sem api key → mensagem de erro clara, nada persiste.
- Config existente sem `enabled` → comportamento atual preservado.

---

### REQ-002 — Modelo fixo determinístico com Jev desabilitado

Com o Jev desabilitado, o roteamento passa a ser determinístico:

- Campo top-level `model` (string, default `""`) na config; vazio = automático.
- Jev desabilitado + `model` definido → todas as chamadas (agente e subagentes) usam esse modelo.
- Jev desabilitado + `model` vazio → todas as chamadas usam o `default_model` do catálogo.
- O `HeuristicRouter` deixa de ser roteador principal; permanece apenas como fallback de erro quando o Jev está habilitado (comportamento atual).
- Modelo inválido na config → erro claro no startup (validação contra o catálogo no wiring, não no parsing da config).
- Env override `KTERMINAL_MODEL`.

#### Critérios de Aceite

- Jev desabilitado + `model` vazio → `default_model` do catálogo em 100% das rotas do transcript.
- Jev desabilitado + `model = X` → `X` em todas as rotas, inclusive subagentes.
- `model` com nome fora do catálogo → boot falha com erro apontando o valor.
- Nenhuma decisão por keyword ocorre com Jev desabilitado.

---

### REQ-003 — Picker de modelos e `/model` persistente

Escolha de modelo sem decorar nomes, com persistência:

- Picker no estilo do `/mode`: lista os modelos do catálogo (nome + preço/velocidade resumidos) e uma entrada `auto`; setas navegam, enter seleciona, esc cancela sem alterar.
- `/jev off` abre o picker; a seleção persiste `model` na config, aplica como pin e re-wira.
- `/model` sem argumentos abre o mesmo picker (comportamento novo — hoje remove o pin); `/model <name>` persiste direto; `/model auto` limpa `model` (Jev volta a decidir se habilitado; senão default do catálogo).
- Com Jev habilitado e modelo fixo definido, o modelo fixo continua vencendo (semântica de pin atual); `/model auto` devolve a decisão ao Jev.

#### Critérios de Aceite

- `/model <name>` persiste na config e sobrevive restart.
- `/model auto` limpa o campo `model` na config.
- Picker: esc não altera nada; enter persiste a seleção e re-wira.
- `/jev off` abre o picker automaticamente.

---

### REQ-004 — Descoberta: `/config`, `--doctor`, help e status bar

O estado do roteamento visível em todos os pontos de contato:

- Tela `/config` ganha os campos `jev enabled (yes/no)` e `model`, salvos junto com os demais.
- `--doctor` imprime `jev enabled` e o modelo fixo efetivo.
- `/help` documenta `/jev` e o novo `/model`; `/jev` entra nas sugestões de comando.
- Status bar indica o roteamento corrente: `jev` quando o Jev decide, `jev off` quando desabilitado, `pinned <model>` quando há modelo fixo (indicador existente).

#### Critérios de Aceite

- `--doctor` reflete `enabled` e o modelo fixo.
- `/help` lista `/jev` e descreve o `/model` persistente.
- `/jev` aparece nas sugestões de comando ao digitar `/`.
- Status bar distingue Jev ativo, Jev desabilitado e modelo fixo.

## Experiência do Usuário

O fluxo principal é de um comando: `/jev off` → picker abre com os modelos do catálogo → enter na escolha → confirmação curta na tela e status bar atualizada. Esc no picker mantém o modo automático (default do catálogo). Usuários que já conhecem `/model` ganham persistência sem mudar o gesto — o mesmo comando agora sobrevive ao restart. Mensagens de confirmação são curtas e no vocabulário existente da TUI ("model pinned: X", "config saved"). Terminal (Bubble Tea), texto plano, navegação por setas idêntica ao popup `/mode` — sem requisitos de acessibilidade novos além dos padrões do repo.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`, binário único; sem novas dependências.
- Sem mudanças no cliente Jev (`internal/jev`) nem no protocolo Typesafe.
- Retro-compatibilidade da config é mandatória: campos novos têm defaults que reproduzem o comportamento atual.
- Código em inglês, sem comentários (convenção do repo); docs em pt-BR.
- Env override prevalece sobre o arquivo, como nos knobs existentes.

## Fora de Escopo

- Squad pins (`[squad] pins`) — pinagem por persona é mecanismo próprio do squad mode.
- Telemetria nova (a de TPS continua medindo independentemente do roteador).
- `HeuristicRouter` como roteador principal em qualquer cenário.
- Roteamento multi-modelo com Jev desabilitado (ex: modelo rápido para rotina e forte para raciocínio) — o modo fixo é um modelo único.
- Mudanças na lógica de rate-limit fallback entre modelos.
- Toggle de Jev por sessão/snapshot (o estado vive na config global).
