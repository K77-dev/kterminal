# Relatório de Code Review — 006 Telemetria de TPS: Task 4.0 (Wiring no `main.go` e tabela no `--doctor`)

## Resumo

- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados (escopo 4.0): 2 (`main.go`, `main_test.go`)
- Linhas Adicionadas (escopo 4.0): ~82
- Linhas Removidas (escopo 4.0): ~2

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário adicionado |
| Apenas stdlib (Go 1.27, módulo `kterminal`) | OK | Novos imports: `path/filepath`, `strings` (stdlib) + `kterminal/internal/telemetry` (interno) |
| Formatação/lint | OK | `gofmt -l .` sem output; `go vet ./...` limpo |
| Tratamento de erro best-effort (telemetria) | OK | Falha de load silenciosa com fallback interno do `telemetry.Load`; erros de `Close` ignorados — telemetria nunca derruba a sessão |
| Estrutura de pastas | OK | Wiring em `main.go` e testes em `main_test.go`, conforme arquivos relevantes da task |
| Sem secrets/PII | OK | Tabela exibe apenas nomes de modelo e números; `telemetry.json` já é escrito 0600 atômico (código da task 1.0, não alterado) |
| Segurança (API/CORS/SQL/etc.) | N/A | TUI local sem backend; único artefato é arquivo local já coberto pela techspec |

## Aderência à TechSpec

| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `main.go` cria o `Store` com load de `~/.local/share/kterminal/telemetry.json` | SIM | `store := telemetry.Load(telemetryPath())` antes do branch `--doctor` (ver Observação 1) |
| Injeção do store no agent | SIM | `ag.Telemetry = store` logo após `agent.New` |
| `defer store.Close()` (save incondicional no encerramento) | SIM | Registrado no ciclo do app, após o branch `--doctor` (ver Observação 2); `store.Close()` explícito antes do `os.Exit(1)` no erro do TUI (ver Observação 3) |
| `--doctor` imprime tabela `model \| measured (mean tok/s, samples) \| estimate (tok/s)` após as checagens existentes | SIM | Seção impressa após config/catalog/gateway/jev, antes do veredito final; tabela exibida mesmo com checagens falhando (janela de transparência em estado degradado) |
| Função extraída testável `doctorTelemetryTable(store, cat) string` | SIM | Retorna header + uma linha por modelo do catálogo; medido via `Store.GetMean` × `TPSEstimate` do YAML |
| Dados do `Store` carregado no doctor | SIM | Verificado comportamentalmente: doctor exibiu stats de um `telemetry.json` pré-existente (`glm-5.2 \| 88.5 tok/s, 6 samples \| 70 tok/s`) |
| Falha de load/save silenciosa com fallback | SIM | `telemetry.Load` retorna store vazio em arquivo ausente/corrompido; nenhum erro propagado |
| Persistência segue o padrão existente em `~/.local/share/kterminal/` | SIM | `telemetryPath()` replica o padrão `session.Dir()`/`config.Dir()`: `XDG_DATA_HOME` → `~/.local/share` → fallback `.kterminal` |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 4.1 Criar o store no `main.go`, injetar no agent e registrar `defer store.Close()` | COMPLETA | Store criado com load, injetado via campo exportado `Telemetry`, defer registrado no ciclo do app |
| 4.2 Extrair `doctorTelemetryTable(store, cat) string` e imprimi-la no `--doctor` | COMPLETA | Função extraída e impressa no `runDoctor` após as checagens existentes |
| 4.3 Escrever `TestDoctorPrintsTelemetryTable` | COMPLETA | Teste exige header com colunas medido/estimado e a linha completa esperada de cada modelo do catálogo |

## Testes

- Total de Testes: 122 (120 pré-existentes + 2 novos)
- Passando: 122
- Falhando: 0
- Coverage: `kterminal` (package main) 16.2% — `main()` executa a TUI e não é testável em unidade; os helpers testáveis (`resolveSession`, `telemetryPath`, `doctorTelemetryTable`) estão cobertos. Pacotes da feature: telemetry 92.5%, agent 85.4%, catalog 57.6%
- Comando: `go build ./... && go vet ./... && gofmt -l . && go test ./...` — tudo passando

### Qualidade dos testes novos

- `TestDoctorPrintsTelemetryTable` — cobre caminho feliz (modelo com 3 amostras → `84.0 tok/s, 3 samples`), edge case (modelos sem amostras → `no samples`), coluna de estimativa para todos os 7 modelos do catálogo embutido e o header exato da techspec. Asserta a linha completa por modelo (colunas lado a lado), não apenas substrings soltas.
- `TestTelemetryPathRespectsXDGDataHome` — cobre a resolução do novo código de path (padrão XDG do projeto).

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema bloqueante encontrado | — |

### Observações (interpretações da spec, verificadas — não exigem correção)

1. **`telemetry.Load` em vez de `telemetry.New`** — o texto da task menciona `telemetry.New(...)` "(load interno)", mas a Visão Geral da task, a techspec ("cria o Store (load de `~/.local/share/kterminal/telemetry.json`)") e o REQ-002 ("Carregadas no início da sessão"; "Reiniciar o app não perde as estatísticas") exigem a leitura do arquivo. `New` isolado nunca leria o JSON e o `Close` sobrescreveria o histórico com apenas o turno corrente, violando o REQ-002. `Load` = `New` + load interno com fallback silencioso — exatamente o "(load interno)". Verificado comportamentalmente: o doctor renderizou stats de um `telemetry.json` pré-populado.
2. **`defer store.Close()` posicionado após o branch `--doctor`** — o doctor é somente-leitura (nunca grava amostras); com o defer antes do branch, o doctor reescreveria/criaria `telemetry.json` em máquinas limpas. Verificado: `--doctor` com `XDG_DATA_HOME` vazio não criou o arquivo. O save incondicional aplica-se ao ciclo do app, onde existem amostras.
3. **`store.Close()` explícito antes de `os.Exit(1)` no erro do `p.Run()`** — `os.Exit` não executa defers; o código existente já trata `sess.Close()` explicitamente nesse ponto pelo mesmo motivo. A adição serve ao requisito "a última amostra nunca se perde" também na saída por erro. Pequena adição além do texto literal da task, dentro da sua intenção.
4. **`telemetryPath()` segue o padrão XDG existente** (`session.Dir()`/`config.Dir()`) — o PRD pede "o padrão existente em `~/.local/share/kterminal/`", que no código do projeto é XDG-aware; isso também tornou o wiring testável.

## Pontos Positivos

- Composição final mínima e cirúrgica: nenhum arquivo fora do escopo da task foi alterado; `internal/telemetry` e `internal/catalog` consumidos sem modificação.
- Tabela do doctor funciona como janela de transparência mesmo com gateway/jev indisponíveis (impressa antes do `os.Exit(1)`).
- Verificação comportamental executada além dos testes de unidade: saída integral do `--doctor` inspecionada com `telemetry.json` pré-populado e com dados ausentes.
- O formato da tabela casa 1:1 com o especificado na techspec (`model | measured (mean tok/s, samples) | estimate (tok/s)`), com rótulo de seção `telemetry:` consistente com as demais seções do doctor.

## Recomendações

- Para a task 5.0 (verificação final): validar E2E via `kspec-qa` os fluxos deferidos — reinício do app preserva estatísticas e `--doctor` mostra a tabela (já antecipado aqui em verificação manual isolada).
- A techspec 010 (subagentes) deve confirmar o compartilhamento do `Store` do agent principal, conforme anotado na própria techspec.

## Conclusão

A task 4.0 foi implementada em conformidade com a techspec e o PRD: o `main.go` cria o `Store` com load silencioso de `~/.local/share/kterminal/telemetry.json`, injeta no agent, garante persistência no encerramento (defer + close explícito na saída por erro) e o `--doctor` exibe a tabela medido × estimado por modelo do catálogo via função extraída testável. Todos os checks passam (build, vet, gofmt, 122/122 testes). As quatro observações documentadas são interpretações verificadas da spec, não desvios. **Veredito: APROVADO.**
