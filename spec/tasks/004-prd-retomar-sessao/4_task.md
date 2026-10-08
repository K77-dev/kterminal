# Tarefa 4.0: Reconstrução visual do histórico e hint bar `resumed` na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Sessão retomada é a mesma sessão

## Dependências

- 3.0 (wiring passa as mensagens carregadas à TUI)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

A última peça do resume: ao iniciar com sessão carregada, a TUI reconstrói as **últimas ~20 mensagens** no viewport com os blocos existentes do chat ao vivo — role `user` → bloco userBox, `assistant` → markdown + linha `▣`, `tool` → linha de tool — e exibe `resumed · N mensagens` no hint bar enquanto a sessão durar (N = total do snapshot, não o limite de renderização). Mensagens `system` (pós-compação) não renderizam bloco. O agente recebe o histórico **completo** via snapshot — só a renderização é limitada; nada funcional se perde.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + dependências de TUI existentes.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (histórico visível ao iniciar), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; `tui` não toca `session` (recebe mensagens prontas).
</skills>

<requirements>
- Opção de construção `tui.WithResumed(messages)` (ou campo/param equivalente no construtor) recebendo as mensagens carregadas.
- Reconstrução das últimas ~20 mensagens: `user` → userBox, `assistant` → markdown + `▣`, `tool` → linha de tool (truncado como hoje); `system` → sem bloco.
- Hint bar: `resumed · N mensagens` (N = total do snapshot) enquanto a sessão durar.
- Sem nova linguagem visual — mesmos blocos do chat ao vivo.
- O resume é imediato: histórico e prompt pronto, sem passos intermediários.
</requirements>

## Subtarefas

- [ ] 4.1 Adicionar a opção de construção da TUI para sessão retomada (`WithResumed` ou equivalente)
- [ ] 4.2 Implementar a reconstrução das últimas ~20 mensagens com os blocos existentes
- [ ] 4.3 Exibir `resumed · N mensagens` no hint bar
- [ ] 4.4 Escrever os testes 9-10 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras de reconstrução e hint bar: techspec, seção **Design de Implementação → Reconstrução na TUI**.
- Decisão "últimas ~20 mensagens no viewport": techspec, seção **Considerações Técnicas → Decisões Principais** (item 2).
- Mensagens `system` não renderizam bloco (interação com a techspec 003): techspec, seção **Design de Implementação → Reconstrução na TUI**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: 25 mensagens carregadas → viewport contém os blocos das últimas ~20 e não contém a mensagem mais antiga.
- Teste prova: render contém `resumed · N mensagens`.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 9-10):
  - `TestResumedReconstructsBlocks` — últimas ~20 reconstruídas (user box, markdown, linha de tool); a mais antiga ausente.
  - `TestHintBarShowsResumed` — render contém `resumed · N mensagens`.
- [ ] Testes de integração — cobertos pelos testes de session/agent (tasks 1.0 e 2.0).
- [ ] Testes E2E — deferidos para `kspec-qa` (histórico visível no viewport ao iniciar).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — reconstrução de blocos, hint bar `resumed`
- `internal/tui/tui_test.go` — reconstrução das últimas ~20, hint bar
- `main.go` — passa as mensagens carregadas à TUI (task 3.0)
