# PRD — Sidebar de agentes do squad

## Visão Geral

No squad mode, a deliberação acontece em convocações sequenciais cujas contribuições aparecem inline no chat (`▸ <papel>:`). Isso deixa três perguntas sem resposta de relance: quem está na mesa (a lista só aparece no kickoff e se perde no scroll), o que cada persona está fazendo agora, e quanto cada papel já consumiu do orçamento anunciado. Acompanhar a mesa hoje exige reler o transcript.

Este PRD introduz um **sidebar na direita da TUI** (referência visual: sidebar do opencode), visível somente no modo squad, somente leitura, que mostra a mesa em tempo real: personas convocadas com status e cor da disciplina, atividade corrente de cada persona, modelo roteado pelo Jev, tokens/custo por papel e o orçamento da mesa contra os tetos do kickoff. O chat permanece inalterado — o sidebar é complementar, não substituto.

## Objetivos

- Transparência de relance: composição e estado da mesa visíveis num painel fixo, sem depender do scroll do chat
- Observabilidade de custo: consumo por papel e total da mesa contra o orçamento do kickoff, atualizado em tempo real
- Zero atrito: sidebar aparece automaticamente ao ativar squad e some ao voltar para sdd; nenhum comando extra
- Zero regressão: modo sdd renderiza exatamente como hoje; contribuições inline do squad permanecem
- Continuidade: estado da mesa persiste no snapshot e é reconstruído no resume

Métricas de sucesso: status de persona visível no frame seguinte ao evento; valores de custo/tokens do sidebar batem com o transcript JSONL; layout íntegro de 80 a 200+ colunas sem overflow.

## Histórias de Usuário

Usuário primário: **desenvolvedor** usando o squad mode. Secundário: **mantenedor** que configura personas e tetos do projeto.

- Como desenvolvedor, quero ver a mesa (quem foi convocado e seu status) num painel fixo à direita, para acompanhar a deliberação sem caçar no scroll.
- Como desenvolvedor, quero ver a ação corrente de cada persona (tool em execução), para entender o andamento em tempo real.
- Como desenvolvedor, quero ver modelo, tokens e custo por persona e o consumo contra o orçamento da mesa, para perceber estouro de teto antes que aconteça.
- Como desenvolvedor, quero o chat intacto com as contribuições inline, para ler a deliberação completa no fluxo.
- Como desenvolvedor retomando uma sessão, quero a mesa reconstruída no sidebar, para retomar o contexto sem reler o transcript.
- Como usuário em terminal estreito, quero o sidebar removido automaticamente, para nunca sacrificar a área de conversa.

## Funcionalidades Principais

### REQ-001 — Exibição condicionada ao modo squad

Sidebar na lateral direita da TUI, renderizado somente quando o modo ativo é `squad`. Aparece e some automaticamente com o `/mode`, sem comando extra. No modo `sdd` a TUI é idêntica ao layout atual. O painel é somente leitura: nunca recebe foco de teclado nem desvia input do usuário.

#### Critérios de Aceite

- Sidebar aparece ao ativar `squad` e some ao voltar para `sdd`, na mesma sessão, sem restart
- Modo `sdd` renderiza sem sidebar e sem qualquer perda de largura no chat
- Nenhuma tecla existente muda de comportamento; o sidebar não captura input

---

### REQ-002 — Painel da mesa

Lista das personas convocadas na mesa corrente, na ordem de convocação: nome, disciplina, cor da disciplina (paleta existente do tema) e status (aguardando, deliberando, concluída). O maestro é identificado no topo. Antes do kickoff, o painel exibe um estado explícito de mesa não iniciada — nunca um vazio silencioso.

#### Critérios de Aceite

- Cada persona exibida com nome textual e status; a cor nunca é o único indicador
- O status de cada persona muda em tempo real conforme os eventos da deliberação
- Estado de mesa não iniciada é exibido entre a ativação do modo e o kickoff
- Mesa encerrada (convergência, teto ou Esc) reflete o estado final das personas

---

### REQ-003 — Atividade ao vivo

A persona em deliberação exibe sua ação corrente (tool em execução com argumentos resumidos); personas concluídas exibem a última ação finalizada. O conteúdo é truncado à largura do painel com elisão explícita.

#### Critérios de Aceite

- Persona deliberando mostra a ação corrente, atualizada a cada evento
- Persona concluída mostra a última ação finalizada
- Conteúdo maior que a largura do painel é truncado com indicador de elisão, sem quebrar o layout

---

### REQ-004 — Métricas por persona e orçamento da mesa

Por persona: modelo roteado pelo Jev, tokens consumidos e custo acumulado. No rodapé do painel: tetos declarados no kickoff (convocações e token budget) e o consumo corrente contra eles.

#### Critérios de Aceite

- Modelo, tokens e custo por persona refletem os mesmos valores registrados no transcript JSONL
- O rodapé exibe os tetos do kickoff e o consumo acumulado da mesa contra cada teto
- Valores atualizam a cada evento de persona, sem ação do usuário

---

### REQ-005 — Layout responsivo

Largura do sidebar proporcional à largura do terminal, com piso e teto fixados. Abaixo da largura mínima de terminal suportada, o sidebar é omitido e o chat ocupa a largura total. Redimensionamento em tempo real não corrompe chat nem painel.

#### Critérios de Aceite

- Largura do sidebar permanece dentro do piso/teto em terminais de 80 a 200+ colunas
- Em terminal estreito o sidebar é omitido automaticamente e o chat usa a largura total
- Redimensionar o terminal com a sessão aberta mantém os dois painéis íntegros, sem overflow

---

### REQ-006 — Persistência e resume

O estado da mesa (personas, status, métricas acumuladas) persiste no snapshot no fim do turno e é reconstruído no resume quando o modo restaurado é `squad`. O campo é aditivo: snapshots antigos carregam normalmente.

#### Critérios de Aceite

- Snapshot de sessão em modo squad carrega o estado da mesa
- Resume em modo squad reconstrói o sidebar com personas, status e métricas salvas
- Snapshots anteriores à funcionalidade carregam sem erro e exibem mesa não iniciada

## Experiência do Usuário

**Fluxo principal**: `/mode` → `squad` → sidebar surge à direita com estado de mesa não iniciada → kickoff popula o painel (personas, orçamento) → convocações atualizam status, atividade e métricas ao vivo → convergência marca personas concluídas → `/mode sdd` → sidebar some.

**Considerações de UI/UX**:

- Sidebar somente leitura, sem foco; todos os atalhos e popups existentes inalterados
- Chat inalterado: contribuições inline `▸ <papel>:` permanecem como fonte de leitura completa
- Hint bar continua indicando o modo ativo como hoje
- Separação visual clara entre sidebar e chat (borda ou gutter da paleta do tema)

**Acessibilidade**:

- Nome e status sempre textuais; cor nunca é o único indicador de persona ou estado
- Cores da paleta do tema com contraste adequado em fundo claro e escuro
- Nenhuma dependência de mouse; informação do sidebar também existe no chat/transcript

## Restrições Técnicas de Alto Nível

- TUI Bubble Tea/Lipgloss existente; sidebar composto na renderização por junção horizontal com o chat
- Dados derivados dos eventos existentes do agente (campos `Agent`, `Model`, `Tokens`, `SessionCost`); nenhum novo loop agêntico ou canal de eventos
- Snapshot aditivo e retrocompatível (precedentes: campos `Skill` e `Mode`)
- Cores por disciplina reutilizam a paleta existente do tema
- Go 1.27, módulo único `kterminal`; código em inglês, sem comentários; spec em pt-BR
- v1 acompanha a mesa sequencial existente (uma persona deliberando por vez)

## Fora de Escopo

- Interação com o sidebar (navegação, seleção de persona, ver histórico, abortar persona) — fast-follow
- Sidebar no modo `sdd` para subagentes de skills
- Toggle manual de visibilidade (atalho ou comando)
- Configuração de largura/piso/teto via config.toml
- Suporte a mouse
- Progresso de múltiplas personas em paralelo (depende do paralelismo do squad, fast-follow da feature 12)
