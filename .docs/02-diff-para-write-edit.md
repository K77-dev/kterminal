# 02 — Diff colorido para write/edit

## Contexto

kterminal é um terminal agêntico em Go (Bubble Tea) com tema visual do opencode. O agente edita arquivos via tools `write` e `edit` (`internal/tools/fs.go`), todas auto-aprovadas (YOLO) por padrão, com `--confirm` opcional. Hoje o resultado de uma edição aparece como uma linha truncada — impossível saber o que mudou.

Arquitetura relevante:

- `internal/tools/fs.go` — tools `write` (sobrescreve) e `edit` (replace exato único).
- `internal/tools/tools.go` — `Registry.Execute(name, argsJSON) (string, error)`.
- `internal/agent/agent.go` — emite `EventToolStart`/`EventToolResult`/`EventConfirm`; o evento confirm carrega `Tool`, `Args`, `ApproveCh`.
- `internal/tui/theme.go` — paleta opencode (dark). O tema original do opencode define `diffAdded: #4fd6be` e `diffRemoved: #c53b53` — estas cores AINDA NÃO existem no nosso theme.go, adicione-as.
- `internal/session/session.go` — transcript JSONL.

## Objetivo

Mostrar diff unificado colorido de toda edição de arquivo: depois da execução (sempre) e antes da aprovação (modo `--confirm`).

## Especificação

1. Implementar um diff de linhas simples em `internal/tools/diff.go` (sem dependências externas): função `LineDiff(old, new string) []DiffLine` onde `DiffLine{Kind: "+"| "-" | " ", Text}`. LCS clássico serve; arquivos de código são pequenos o suficiente.
2. Tool `edit`: capturar conteúdo anterior, aplicar a edição, calcular diff. Tool `write`: se o arquivo existia, diff antigo→novo; se é arquivo novo, todas as linhas como `+`.
3. `Registry.Execute` retorna um struct `Result{Output string, Diff []DiffLine}` (ou segundo valor) em vez de só string — atualizar assinatura e o agente.
4. `EventToolResult` e `EventConfirm` ganham campo `Diff []tools.DiffLine`.
5. TUI: renderizar o diff como bloco com cores — linhas `+` em `#4fd88f`-style (`colDiffAdded`), `-` em `colDiffRemoved`, contexto em `colTextMuted`, prefixado por `+ `/`- `/`  `. Máximo ~40 linhas visíveis (truncar no meio com `… N more lines …`).
6. `confirmView()` mostra o diff completo antes do y/n.
7. Transcript: gravar o diff no evento JSONL (campo `Diff []string` com as linhas já formatadas).

## Critérios de aceitação

- Edit em arquivo existente mostra remoções e adições coloridas.
- Write em arquivo novo mostra tudo como adição.
- Modo `--confirm` exibe o diff antes da aprovação.
- Diff de arquivos sem mudanças não renderiza bloco vazio.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: `LineDiff` (inserção, remoção, mudança no meio, arquivos iguais, arquivo vazio); teste de TUI verificando que o bloco renderizado contém as cores certas (`\x1b[38;2;79;214;190` para added).

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Sem novas dependências — diff implementado à mão.
