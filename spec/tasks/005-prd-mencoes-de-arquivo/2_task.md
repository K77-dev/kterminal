# Tarefa 2.0: Expansão no envio, chat com texto original e aviso inline

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Expansão de menções
- REQ-003 — Proteções de expansão
- REQ-004 — Transcript fiel

## Dependências

- 1.0 (`ExpandMentions` disponível como função pura)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Esta task liga a expansão ao fluxo de envio: no Enter, a TUI chama `ExpandMentions(input)`, renderiza o bloco do usuário no chat com **só o texto original**, e chama `agent.Run(expanded)`. O transcript grava a mensagem expandida (o que o modelo efetivamente viu) automaticamente — o agent recebe e grava a mensagem já expandida, sem conhecer o conceito de menções (requisito não negociável do PRD). Warnings (ex.: "max 5 file mentions") renderizam inline em `colWarning` acima do prompt, sem bloquear o envio.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (fluxo E2E de menção), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; zero mudanças em `internal/agent`.
</skills>

<requirements>
- No envio (Enter): `expanded, warnings := ExpandMentions(input)` antes de `agent.Run`.
- O bloco do usuário no chat renderiza **só o texto original** (com os tokens intactos).
- `agent.Run(expanded)` recebe a mensagem expandida — o agente nunca conhece menções.
- O transcript grava a mensagem expandida no evento `user` (automático por construção — REQ-004).
- Warnings renderizam inline em `colWarning` acima do prompt, sem bloquear o envio.
- Nenhuma mudança em `internal/agent` ou `internal/session`.
</requirements>

## Subtarefas

- [ ] 2.1 Chamar `ExpandMentions` no ponto de envio, antes de `agent.Run`
- [ ] 2.2 Renderizar o bloco do usuário no chat com o texto original (não o expandido)
- [ ] 2.3 Renderizar warnings inline em `colWarning` acima do prompt
- [ ] 2.4 Escrever os testes 12-13 da techspec com captura do input do agent (ver Testes da Tarefa)

## Detalhes de Implementação

- Fluxo de envio e de aviso: techspec, seção **Arquitetura do Sistema → Fluxo de dados** (item 2 e 3).
- Decisão "transcript expandido vs chat original": techspec, seção **Considerações Técnicas → Decisões Principais** (item 3).
- Decisão "expansão na TUI, agente alheio" (requisito não negociável): techspec, seção **Considerações Técnicas → Decisões Principais** (item 1) e PRD, seção **Restrições Técnicas de Alto Nível**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: enviar `@a.txt o que é?` → bloco do chat contém só o texto original; a mensagem recebida pelo agent (mock/capture) contém o bloco expandido.
- Teste prova: 6 menções → aviso `max 5 file mentions` visível em `colWarning`.
- O evento `user` no JSONL contém a mensagem expandida completa (validado via captura/transcript no teste).

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, harness `newTestModel`/`step` com temp dir — techspec **Abordagem de Testes**, itens 12-13):
  - `TestSendExpandsButChatShowsOriginal` — chat com original; agent (mock/capture) recebe o expandido.
  - `TestWarningRendersInline` — 6 menções → aviso `max 5 file mentions` em `colWarning`.
- [ ] Testes de integração — `TestSendExpandsButChatShowsOriginal` cobre TUI→agent com captura do input (contrato ponta a ponta, conforme techspec **Abordagem de Testes → Testes de Integração**).
- [ ] Testes E2E — deferidos para `kspec-qa` (`@main.go o que esse arquivo faz?` responde sem tool call de leitura; menção inexistente chega intacta).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — expansão no envio, bloco do chat com original, aviso inline
- `internal/tui/tui_test.go` — fidelidade chat×transcript, aviso inline
- `internal/agent/agent.go` — sem mudança (recebe a mensagem expandida)
- `internal/session/session.go` — sem mudança (evento `user` grava o expandido por construção)
