# Tarefa 3.0: Popup de autocomplete de arquivos

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Autocomplete com popup

## Dependências

- 1.0 (`mentionPrefix` disponível para extrair o token em digitação)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

A ergonomia da feature: ao digitar `@`, a TUI abre um popup de sugestões de arquivos entre o viewport e o prompt box. O estado novo do Model (`mentionOpen`, `mentionItems`, `mentionSelected` — receivers por valor, padrão Bubble Tea) é alimentado por glob do prefixo digitado (máximo 10, ordenados, arquivos apenas). Teclas com popup aberto: `Tab` completa com o item selecionado; `Enter` completa **sem enviar**; `Esc` fecha; `↑`/`↓` movem a seleção; qualquer digitação re-glob. Itens em `colTextMuted`, selecionado em `colPrimary` — paleta existente, sem cores novas.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`path/filepath`, `sort`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (autocomplete com Tab), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; sem estado global (popup vive no Model); `strings.Builder` por ponteiro.
</skills>

<requirements>
- Estado no Model: `mentionOpen bool`, `mentionItems []string`, `mentionSelected int`.
- A cada atualização do input, `mentionPrefix` extrai o token `@...` sob o cursor → glob do prefixo → popup (máx. 10, ordenados, arquivos apenas); sem token → popup fechado.
- `Tab` → completa com o item selecionado (substitui o token no input); popup fechado.
- `Enter` com popup aberto → completa **sem enviar**; `Enter` com popup fechado → envia (comportamento da task 2.0 preservado).
- `Esc` → fecha o popup sem cancelar turno (agente ocioso).
- `↑`/`↓` → movem a seleção; digitação → re-glob.
- Renderização: entre viewport e prompt box; itens em `colTextMuted`, selecionado em `colPrimary`.
- Completar substitui o token sob o cursor, não o fim do input (cursor no meio do texto).
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar o estado do popup ao Model e alimentá-lo via `mentionPrefix` + glob a cada mudança de input
- [ ] 3.2 Implementar o key handling: Tab completa, Enter completa sem enviar, Esc fecha, ↑/↓ movem seleção
- [ ] 3.3 Renderizar o popup entre viewport e prompt box com a paleta existente
- [ ] 3.4 Escrever os testes 8-11 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras do popup, sugestões e teclas: techspec, seção **Design de Implementação** (subseção "Popup de autocomplete").
- Decisão "popup entre viewport e prompt, paleta existente": techspec, seção **Considerações Técnicas → Decisões Principais** (item 4).
- Cursor no meio do texto e boundary do token: techspec, seção **Considerações Técnicas → Riscos Conhecidos**.
- Interação com a techspec 007 (multiline): `mentionPrefix` opera sobre o texto do input multi-linha — a função recebe o texto corrente (compatível por construção).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: digitar `@` → popup aberto com arquivos do cwd (máx. 10, ordenados).
- Teste prova: `Tab` completa com o primeiro resultado e fecha o popup.
- Teste prova: `Enter` com popup aberto completa e **não** chama `agent.Run`; com popup fechado envia.
- Teste prova: `Esc` fecha o popup sem cancelar o turno.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, harness `newTestModel`/`step` com temp dir — techspec **Abordagem de Testes**, itens 8-11):
  - `TestPopupListsFilesOnAtSign` — `@` → popup com arquivos do cwd (máx. 10, ordenados).
  - `TestTabCompletesFirstResult` — `Tab` → input com caminho completo; popup fechado.
  - `TestEnterWithPopupCompletesNotSends` — `Enter` completa sem chamar `agent.Run`; popup fechado → envia.
  - `TestEscClosesPopup` — `Esc` fecha sem cancelar turno (agente ocioso).
- [ ] Testes de integração — cobertos pelo teste de envio da task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (autocomplete com Tab em fluxo real).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — estado do popup, key handling (Tab/Enter/Esc/↑↓), glob de sugestões, renderização
- `internal/tui/tui_test.go` — popup, teclas
- `internal/tui/mentions.go` — `mentionPrefix` consumido (task 1.0)
