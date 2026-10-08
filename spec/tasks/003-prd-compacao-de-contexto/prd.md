# PRD — Gestão de janela de contexto (compação)

## Visão Geral

No kterminal, o histórico inteiro da conversa é enviado ao LLM a cada chamada e cresce sem limite. Os modelos do catálogo têm janelas de contexto diferentes (deepseek 128k, GLM 200k). Hoje, uma sessão longa estoura a janela do modelo escolhido e a chamada falha — é a **limitação conhecida mais grave** do produto.

Este PRD define a compação automática do histórico: quando o contexto se aproxima do limite do modelo em uso, os turnos antigos são resumidos e substituídos, e o trabalho continua sem interrupção. O usuário primário é o Jev (Typesafe), que alterna entre modelos a cada chamada e precisa que sessões longas simplesmente funcionem.

## Objetivos

- **Robustez**: zero estouros de janela de contexto — sessões longas nunca falham por limite de tokens.
- **Continuidade**: a compação é automática e mecânica, sem perguntar ao usuário.
- **Preservação**: o trabalho recente (últimas mensagens) chega intacto ao modelo após a compação.
- **Visibilidade**: o usuário sabe quando o contexto foi compactado e o quanto encolheu.

## Histórias de Usuário

- Como Jev, quero sessões longas que nunca falhem por estouro de contexto, para trabalhar sem reiniciar a conversa.
- Como Jev, quero que a compação aconteça automaticamente, sem prompts de confirmação atrapalhando o fluxo.
- Como Jev, quero que as mensagens mais recentes sejam preservadas intactas, para que o agente não perca o fio da meada.
- Como Jev, quero ver um indicador sutil quando o contexto for compactado, para entender por que o agente "esqueceu" detalhes antigos.

## Funcionalidades Principais

### REQ-001 — Estimativa de tokens por chamada

Antes de cada chamada ao LLM, o agente estima o custo do próximo prompt:

- Usa a medição real de tokens da última chamada somada ao delta aproximado desde então.
- Fallback heurístico (tamanho do texto concatenado / 4) quando ainda não há medição.

#### Critérios de Aceite

- A contagem usa medição real após a primeira resposta do modelo.
- Sem medição disponível, a estimativa heurística é usada.

---

### REQ-002 — Compação automática em 70% da janela

Quando a estimativa excede **70%** da janela de contexto do modelo escolhido, a compação executa antes da chamada:

- Um prompt de resumo com os turnos antigos (tudo exceto as últimas 4 mensagens) é enviado a um modelo barato fixo (`deepseek-v4.1-flash`, com fallback para o default do catálogo se indisponível), pedindo um resumo denso: decisões, arquivos tocados, estado da tarefa.
- O histórico é substituído por: mensagem de sistema/resumo + últimas 4 mensagens.
- A chamada de resumo não usa tools.

#### Critérios de Aceite

- Sessão simulada com catálogo de janela pequena (ex.: 2k tokens) compacta antes de estourar e continua funcionando.
- O resumo preserva as últimas 4 mensagens intactas.
- A chamada de resumo ocorre antes da chamada que estouraria a janela.

---

### REQ-003 — Truncamento de tool results gigantes

Se mesmo compactado o prompt não couber (ex.: um único tool result gigante), tool results antigos são truncados para os primeiros 2000 caracteres com sufixo `… (truncated)`.

#### Critérios de Aceite

- Tool result gigante sozinho no contexto não causa estouro após truncamento.

---

### REQ-004 — Evento de compação

Um novo evento de compação (tokens antes/depois) é emitido à TUI e gravado no transcript.

#### Critérios de Aceite

- A TUI renderiza linha sutil `⚡ context compacted (12.4k → 3.1k tokens)` em cor de texto muted.
- O evento fica gravado no transcript JSONL.

## Experiência do Usuário

- A compação é invisível no fluxo normal — nenhuma pergunta, nenhuma pausa.
- O único sinal visual é a linha sutil de compação no chat, em cor muted, sem interromper a leitura.
- Após a compação, o agente mantém a coesão da tarefa corrente graças ao resumo denso + mensagens recentes preservadas.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- A compação é mecânica (limiar de tokens) — nunca pergunta ao usuário.
- O limiar de 70% e a preservação das últimas 4 mensagens são requisitos não negociáveis da v1.
- O resumo usa a mesma infraestrutura de streaming existente, sem tools.

## Fora de Escopo

- Compação manual (comando slash ou flag).
- Escolha do modelo de resumo pelo usuário.
- RAG ou memória de longo prazo além do resumo.
- Ajuste do limiar configurável pelo usuário.
- Compação seletiva por importância de mensagem.
