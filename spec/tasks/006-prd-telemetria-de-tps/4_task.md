# Tarefa 4.0: Wiring no `main.go` e tabela de telemetria no `--doctor`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Persistência local
- REQ-004 — Visibilidade no `--doctor`

## Dependências

- 3.0 (agent gravando e injetando telemetria)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Composição final: o `main.go` cria o `Store` (load de `~/.local/share/kterminal/telemetry.json`), injeta no agent e registra `defer store.Close()` (save incondicional no encerramento — a última amostra nunca se perde). O `--doctor` ganha a tabela de transparência: `model | measured (mean tok/s, samples) | estimate (tok/s)` — dados do `Store` carregado × `TPSEstimate` do catálogo, via função extraída testável `doctorTelemetryTable(store, cat) string`.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (persistência entre reinícios, `--doctor`), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; falha de load/save silenciosa com fallback (telemetria nunca derruba a sessão).
</skills>

<requirements>
- `main.go`: `store := telemetry.New(~/.local/share/kterminal/telemetry.json)` (load interno), injeção no agent, `defer store.Close()`.
- Falha de load/save é silenciosa com fallback para store vazio/em memória — telemetria é best-effort.
- `--doctor`: após as checagens existentes, imprime a tabela `model | measured (mean tok/s, samples) | estimate (tok/s)`.
- Tabela extraída em função testável `doctorTelemetryTable(store, cat) string`.
</requirements>

## Subtarefas

- [ ] 4.1 Criar o store no `main.go`, injetar no agent e registrar `defer store.Close()`
- [ ] 4.2 Extrair `doctorTelemetryTable(store, cat) string` e imprimi-la no `--doctor`
- [ ] 4.3 Escrever `TestDoctorPrintsTelemetryTable` (ver Testes da Tarefa)

## Detalhes de Implementação

- Wiring do store e tabela no doctor: techspec, seção **Arquitetura do Sistema** (componentes `main.go`) e **Pontos de Integração** (item `--doctor`).
- Error tracking best-effort: techspec, seção **Monitoramento e Observabilidade → Error Tracking**.
- Formato da tabela: techspec, seção **Pontos de Integração** — `model | measured (mean tok/s, samples) | estimate (tok/s)`.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: saída da função de tabela contém colunas medido/estimado por modelo.
- O app inicia e encerra sem erro com o store ativo; `Close` persiste as amostras do turno.

## Testes da Tarefa

- [ ] Testes de unidade:
  - `TestDoctorPrintsTelemetryTable` — captura da saída contém colunas medido/estimado (função extraída testável `doctorTelemetryTable(store, cat) string` — techspec, item 12).
- [ ] Testes de integração — cobertos pelos testes de agent da task 3.0.
- [ ] Testes E2E — deferidos para `kspec-qa` (reinício do app preserva estatísticas; `--doctor` mostra a tabela).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `main.go` — wiring do store (load/debounce/Close), tabela no `--doctor`
- `internal/telemetry/telemetry.go` — `Store` consumido (task 1.0)
- `internal/catalog/catalog.go` — `TPSEstimate` consumido na tabela
