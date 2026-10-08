# Tarefa 4.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Expansão de menções
- REQ-002 — Autocomplete com popup
- REQ-003 — Proteções de expansão
- REQ-004 — Transcript fiel

## Dependências

- 2.0, 3.0 (feature completa: expansão, envio e autocomplete implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código. Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0 ou 3.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: menção responde sem tool call de leitura; Tab no autocomplete; menção inexistente intacta), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; receivers por valor na TUI; `strings.Builder` sempre por ponteiro; zero mudanças em `internal/agent` (agente alheio às menções).
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (menções de diretório, URLs, fuzzy matching, expansão em mensagens do agente, anexos binários).
</requirements>

## Subtarefas

- [ ] 4.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 4.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 4.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários, receivers por valor, agente sem mudanças)
- [ ] 4.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 função pura, 2.0 envio/transcript, 3.0 popup) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - `@main.go o que esse arquivo faz?` → modelo recebe o conteúdo e responde sem tool call de leitura (E2E — QA; base coberta por `TestSendExpandsButChatShowsOriginal`).
  - Menção inexistente passa intacta ao agente (`TestExpandMissingFilePassesThrough`).
  - Popup lista arquivos reais conforme o prefixo (`TestPopupListsFilesOnAtSign`).
  - Tab completa, Enter completa sem enviar, Esc fecha (`TestTabCompletesFirstResult`, `TestEnterWithPopupCompletesNotSends`, `TestEscClosesPopup`).
  - Menção dentro de code fence não é expandida (`TestExpandIgnoresCodeFences`).
  - 6+ menções → 5 expandidas + aviso inline visível (`TestExpandLimitFive` + `TestWarningRendersInline`).
  - Evento `user` no JSONL contém a mensagem expandida completa (REQ-004, por construção — `TestSendExpandsButChatShowsOriginal`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0 e 3.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos por `TestSendExpandsButChatShowsOriginal` (task 2.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 4.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/mentions.go`, `internal/tui/mentions_test.go` — diff da task 1.0
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff das tasks 2.0 e 3.0
- `spec/tasks/005-prd-mencoes-de-arquivo/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/005-prd-mencoes-de-arquivo/techspec.md` — cenários E2E para o QA
