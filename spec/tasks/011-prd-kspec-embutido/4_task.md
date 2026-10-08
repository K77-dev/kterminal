# Tarefa 4.0: Tool ask_user com evento bloqueante e wizard na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-005 — Tool `ask_user`

## Dependências

- Nenhuma (paralelizável com 1.0–3.0: não depende do pacote kspec)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Nova tool `ask_user`, sempre disponível ao agente principal (não só com skill ativa), para perguntas estruturadas com opções navegáveis, seleção múltipla e texto livre. O fluxo segue o precedente da confirmação write/edit: a tool emite um evento com canal de resposta e bloqueia; a TUI renderiza um prompt bloqueante (wizard sequencial, uma pergunta por tela) e devolve as respostas pelo canal.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md` (eventos via canal, TUI com receivers por valor, tools no padrão do Registry), `tests.md`.
</skills>

<requirements>
- Schema: `questions[]` com `question`, `header`, `options[]` (label + description), `multiple`
- Múltiplas perguntas por chamada, apresentadas sequencialmente (uma por tela)
- Navegação por setas; espaço alterna multi-seleção; enter confirma; "Type your own answer" sempre disponível como texto livre
- Cancelamento (esc) fecha o canal com `nil`; a tool devolve "user declined to answer"
- Respostas voltam ao agente como tool result (uma resposta por pergunta, na ordem; multi-seleção junta labels com ", ")
- Registrada via método do Agent (precedente `AttachTaskTool`), apenas no agente principal — subagentes não perguntam
- Eventos novos sem quebrar o protocolo existente
</requirements>

## Subtarefas

- [ ] 4.1 Definir tipos `AskQuestion`/`AskOption` e o `EventAskUser` (campos `Questions []AskQuestion`, `AnswerCh chan []string`) no pacote agent
- [ ] 4.2 Implementar `AttachAskUserTool()`: schema da tool, validação de args (questions não vazio, options com label), bloqueio no canal, formatação do result (pares pergunta/resposta)
- [ ] 4.3 Implementar o wizard na TUI: estado `stateAsk`, uma pergunta por tela, navegação por setas, toggle de multi-seleção, entrada de texto livre, confirmação e cancelamento
- [ ] 4.4 Integrar no `Update`/`View` da TUI (`handleAskKey`, `askView`) seguindo o padrão do fluxo de confirmação existente

## Detalhes de Implementação

Seguir a techspec, seção "Modelos de Dados" (tipos do evento e semântica do canal) e "Decisões Principais" (itens 6 e 7 — registro via método do Agent e wizard sequencial). O precedente de fluxo bloqueante está em `internal/tui/tui.go:1131` (`handleConfirmKey`) e `internal/agent/agent.go:256` (`AttachTaskTool`).

## Critérios de Sucesso

- O agente pode chamar `ask_user` em qualquer conversa, com múltiplas perguntas por chamada
- TUI oferece navegação por teclado, seleção múltipla e texto livre — o fluxo nunca bloqueia por indisponibilidade de opção
- A resposta estruturada volta ao agente como tool result
- Subagentes (Registry novo) não têm a tool; o protocolo de eventos existente não quebra

## Testes da Tarefa

- [ ] Testes de unidade
  - Contrato da tool: args válidos/inválidos, bloqueio até resposta no canal, formatação do result, cancelamento com `nil`
  - Wizard: navegação entre opções, toggle multi-seleção, seleção de texto livre, esc cancela, sequência de múltiplas perguntas
- [ ] Testes de integração
  - `ask_user` → `EventAskUser` → resposta via canal → tool result registrado no transcript
- [ ] Testes E2E (se aplicável)
  - N/A — verificação interativa entra no checklist manual da task 7.0

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tools/ask.go` (novo)
- `internal/tools/ask_test.go` (novo)
- `internal/agent/agent.go` (tipos de evento + `AttachAskUserTool`)
- `internal/agent/agent_test.go`
- `internal/tui/ask.go` (novo)
- `internal/tui/tui.go`
- `internal/tui/tui_test.go`
- `internal/tui/tui.go:1131` (referência — precedente de fluxo bloqueante)
