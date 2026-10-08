# Tarefa 3.0: Bloco de output ao vivo na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Bloco de output ao vivo na TUI

## Dependências

- 2.0 (`EventToolOutput` sendo emitido com linhas agrupadas)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

A experiência visível do stream: cada `EventToolOutput` acumula linhas num bloco ao vivo sob a linha da tool `bash`, em `colTextMuted`, indentado como o resultado atual. O bloco mantém janela deslizante das últimas 15 linhas com contador de omitidas (`… +N lines` no topo). O `EventToolResult` final descarta o bloco ao vivo e renderiza o resultado consolidado como hoje — o histórico final fica idêntico ao de hoje (o live é efêmero por design). Comando silencioso (nenhuma linha) → nenhum bloco; o spinner segue normal.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + dependências de TUI existentes.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (stream visível linha a linha), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; estado `[]string` (nunca `strings.Builder` por valor — bug conhecido do projeto).
</skills>

<requirements>
- Estado no Model: `liveLines []string` + `liveOmitted int` (por valor, sem `strings.Builder` por valor).
- `EventToolOutput` → append das linhas no bloco ao vivo.
- Janela deslizante: manter últimas 15 linhas; excedentes contabilizados em `liveOmitted` → prefixo `… +N lines` no topo do bloco.
- Renderização: sob a linha `● bash(...)`, linhas em `colTextMuted`, indentadas como o resultado atual.
- `EventToolResult` → bloco ao vivo descartado; resultado consolidado renderizado como hoje (truncamento normal).
- Comando silencioso: nenhuma linha → nenhum bloco; spinner inalterado.
- Output renderizado como texto plano (sem interpretação ANSI — fora de escopo do PRD).
</requirements>

## Subtarefas

- [x] 3.1 Adicionar `liveLines`/`liveOmitted` ao Model e tratar `EventToolOutput` no loop de eventos
- [x] 3.2 Implementar a janela deslizante de 15 linhas com `… +N lines`
- [x] 3.3 Descartar o bloco ao vivo no `EventToolResult` (resultado consolidado como hoje)
- [x] 3.4 Escrever os testes 12-13 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Regras de renderização do bloco ao vivo: techspec, seção **Design de Implementação → Renderização do bloco ao vivo**.
- Decisão "bloco ao vivo descartado no resultado" (histórico final idêntico ao de hoje): techspec, seção **Considerações Técnicas → Decisões Principais** (item 5).
- Risco conhecido `strings.Builder` por valor: techspec, seção **Considerações Técnicas → Riscos Conhecidos** (item 1).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: 3 `EventToolOutput` → bloco contém as linhas em muted; 20 linhas → últimas 15 + `… +N lines`; `EventToolResult` → bloco substituído pelo consolidado.
- Teste prova: `EventToolStart` + `EventToolResult` sem output → nenhum bloco ao vivo; spinner inalterado.

## Testes da Tarefa

- [x] Testes de unidade (`internal/tui/tui_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 12-13):
  - `TestLiveBlockAccumulatesAndReplaces` — acumulação em muted; janela 15 + `… +N lines`; substituição pelo resultado.
  - `TestSilentCommandNoLiveBlock` — sem `EventToolOutput` → nenhum bloco; spinner inalterado.
- [x] Testes de integração — cobertos pelos testes de agent da task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (`for i in $(seq 1 10); do echo $i; sleep 0.3; done` mostra números um a um; `go test ./...` demorado com spinner + linhas ao vivo).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — `liveLines`/`liveOmitted`, janela 15, substituição pelo resultado
- `internal/tui/tui_test.go` — acumulação, janela, substituição, comando silencioso
