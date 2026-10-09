# Relatório de Code Review - Sidebar de agentes do squad (Task 1.0: Tipo `squad.Mesa`)

## Resumo
- Data: 2026-10-09
- Branch: 014-prd-sidebar-squad
- Status: APROVADO
- Arquivos Modificados: 2 (novos: `internal/squad/mesa.go`, `internal/squad/mesa_test.go`)
- Linhas Adicionadas: 519
- Linhas Removidas: 0

Review independente — o review do implementador (`review_1.md`) foi lido, mas todas as verificações abaixo foram refeitas do zero contra PRD, techspec, task e código.

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código | OK | Nenhum comentário em `mesa.go`/`mesa_test.go`; o comentário do snippet da techspec (`// waiting | deliberating | done`) foi corretamente omitido — os literais vivem nas constantes exportadas |
| Código em inglês | OK | Identificadores, mensagens de teste e literais de status em inglês |
| `squad` é pacote leaf | OK | `mesa.go` importa apenas `sync`; não importa `agent`; params primitivos (`string`, `int64`, `float64`) + `Kickoff` do próprio pacote |
| Padrões do código-base (receivers, events) | OK | Receivers em `*Mesa` em todos os métodos; sufixo `Locked` em `entryIndexLocked` documenta o contrato de lock sem comentário (idiomático Go) |
| Formatação/lint | OK | `gofmt -l .` vazio; `go vet ./...` limpo (sem copylocks — `Mesa` nunca é copiada por valor) |
| Sem dependências novas | OK | Apenas stdlib (`sync`); zero mudanças em `go.mod` |
| Spec em pt-BR | OK | Artefatos da spec em português; código em inglês |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| Struct `MesaEntry` (campos, tipos, tags JSON) | SIM | Byte a byte com a seção "Interfaces Principais": `name`/`status` obrigatórios; `discipline`/`model`/`tokens`/`cost` com `omitempty` |
| Struct `Mesa` (campos exportados, tags JSON) | SIM | Idêntico ao snippet; todos os campos com `omitempty` |
| 8 assinaturas de métodos | SIM | `Reset`, `ResetTurn`, `AddTokens`, `AddConvocation`, `StartDeliberation`, `FinishDeliberation`, `FinishTurn`, `ObservePersona` com assinaturas exatas |
| Mutex interno, todos os acessos sob lock | SIM | Os 8 métodos travam `mu` com `defer Unlock` antes de qualquer acesso a campo; `entryIndexLocked` só é chamado com lock segurado; sem exceção |
| Status literais `waiting`/`deliberating`/`done` | SIM | Constantes `StatusWaiting`/`StatusDeliberating`/`StatusDone` (adição além da lista da techspec, idiomática e com precedente no pacote — `SourceProject`/`SourceEmbedded`) |
| Baseline em `StartDeliberation`/`ObservePersona` | SIM | Baseline por persona em `baselines` (unexported, invisível ao JSON); `ObservePersona` faz `entrada = baseline + cumulativo` — não duplica na mesma convocation, acumula entre convocations |
| `FinishTurn` deliberating→done | SIM | Preserva `waiting` e `done` (testado) |
| Persona inexistente no-op | SIM | `entryIndexLocked` retorna -1 → return; seguro inclusive em Mesa zerada sem `Reset` (mapa/slice nil não causam panic) |
| Campos unexported adicionais (`mu`, `baselines`) | SIM | `mu` é mandato da task; `baselines` é a única forma de implementar a semântica exigida; superfície serializada permanece exatamente a da techspec |

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.1 Structs `MesaEntry`/`Mesa` com tags da techspec | COMPLETA | Verificado campo a campo contra o snippet |
| 1.2 `Reset(k, disciplines)` | COMPLETA | Roles copiados defensivamente, tetos, entradas na ordem de convocação com status `waiting`, `baselines` zerados; cópia do slice testada |
| 1.3 `ResetTurn`/`AddTokens`/`AddConvocation` | COMPLETA | `ResetTurn` zera contadores sem tocar em `Entries` (testado com `reflect.DeepEqual`) |
| 1.4 `StartDeliberation`/`FinishDeliberation`/`FinishTurn` | COMPLETA | Transições e baseline conforme spec |
| 1.5 `ObservePersona` | COMPLETA | Drain cumulativo somado ao baseline; modelo atualizado |
| 1.6 Testes de unidade | COMPLETA | 12 testes cobrindo todos os cenários exigidos + extras (ver seção Testes) |

## Testes
- Total de Testes (pacote `squad`, sob `-race`): 35 (12 novos da Mesa + 23 existentes)
- Passando: 35
- Falhando: 0
- Coverage: N/A para reprovação — todos os caminhos de código da Mesa são exercitados (8 métodos + helper + serialização)

Cenários exigidos pela task, todos presentes e passando:
- `TestResetBuildsMesaFromKickoff` — 3 roles + disciplinas: ordem, status `waiting`, tetos, contadores zerados
- `TestResetRebuildsDirtyMesa` — `Reset` sobre Mesa suja reconstrói estado limpo (extra, além do mínimo)
- `TestResetCopiesRolesSlice` — sem aliasing com o slice do kickoff (extra)
- `TestResetTurnZeroesCountersKeepsEntries` — zera `Convocations`/`Tokens`, `Entries` intactas
- `TestDeliberationTransitions` — `waiting`→`deliberating`→`done`; outras entradas intocadas
- `TestDeliberationUnknownPersonaIsNoOp` — persona inexistente e Mesa zerada: no-op sem panic
- `TestObservePersonaAccumulatesAcrossConvocations` — 2 observações na mesma convocation (100→150, sem duplicar); 2ª convocation acumula sobre a 1ª (150+200=350); modelo atualizado
- `TestFinishTurnMarksDeliberatingDone` — `deliberating`→`done`; `waiting`/`done` preservados
- `TestMesaFullCycle` — ciclo completo do critério de sucesso com statuses e métricas finais conferidos
- `TestMesaJSONRoundTripPopulated` — bytes estáveis + `DeepEqual` de `Roles`/`Entries` e contadores
- `TestMesaJSONOmitEmpty` — Mesa zerada → `{}`; kickoff-only omite `convocations`/`tokens`; entry sem métricas omite `discipline`/`model`/`tokens`/`cost`
- `TestMesaConcurrentAccess` — 100 goroutines × 50 iterações misturando mutação e leitura sob `-race`, com contadores determinísticos conferidos ao fim (prova de serialização sem lost updates)

Os testes verificam comportamento real (valores, ordem, statuses, bytes JSON), não apenas ausência de panic.

## Segurança
N/A — tipo isolado de domínio, sem backend/API: sem I/O, sem rede, sem secrets, sem input externo não-confiável (o JSON consumido no futuro resume vem do snapshot próprio da sessão). A Mesa não carrega prompts nem conteúdo de mensagens, conforme a techspec.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | internal/squad/mesa.go | 25-35 | Campos exportados são legíveis/graváveis de fora do pacote sem lock; a Mesa não oferece leitura thread-safe externa (getter de cópia sob lock). A techspec projeta `Agent.Mesa()` como "cópia rasa sob lock", mas com `mu` unexported e nenhum método de cópia, a task 3.0 não conseguirá cumprir isso sem: (a) copiar por valor — `go vet` copylocks reprova e é racy, ou (b) adicionar um método à Mesa. Gap de design da techspec, não da 1.0 (o contrato desta task foi seguido à risca e ainda não há uso concorrente externo). | Na task 3.0, estender a Mesa com `func (m *Mesa) Clone() Mesa` (cópia profunda de `Roles`/`Entries` sob lock) ou garantir sincronização no Agent; documentar o desvio em relação à lista de interfaces |
| Baixa | internal/squad/mesa.go | 112 | `ObservePersona` sem `StartDeliberation` prévio na persona usa o baseline da convocation anterior (ou zero) — pode sobrescrever em vez de acumular se chamado fora de ordem. No fluxo do Agent o drain sempre ocorre dentro da convocation iniciada por `StartDeliberation`, então o contrato é respeitado por construção. | Manter o pareamento `StartDeliberation`→`ObservePersona` colado no drain da 3.0; opcionalmente um teste de contrato na integração |
| Informativa | internal/squad/mesa.go | 5-9 | Constantes de status exportadas não constam na lista de interfaces da techspec | Aceitável: literais exigidos pela task, forma idiomática, precedente no pacote, consumo pela TUI na 4.0 |
| Informativa | internal/squad/mesa.go | 52-57 | `ResetTurn` não limpa `baselines` | Sem efeito no fluxo real (baseline sempre recapturado por `StartDeliberation` antes do uso); leitura literal do requisito "sem tocar em Entries" |

## Pontos Positivos
- Aderência byte a byte às interfaces da techspec, com as únicas adições sendo as estritamente necessárias (`mu`, `baselines`, constantes) — todas invisíveis ao JSON
- Disciplina de concorrência exemplar: lock em todos os métodos, helper com sufixo `Locked` para o contrato interno, `go vet` copylocks limpo
- `Reset` copia defensivamente o slice de roles — higiene que evita aliasing com o kickoff do caller, testada
- Semântica de baseline correta e bem testada nos dois eixos (re-chamada na mesma convocation não duplica; nova convocation acumula)
- Testes vão além do mínimo exigido (reset sujo, cópia de slice, verificação determinística de contadores pós-concorrência)
- Escopo respeitado: nenhum outro pacote tocado, nenhum espelho antecipado no Agent (tasks 2.0/3.0)

## Recomendações
- Task 3.0: resolver explicitamente a leitura segura da Mesa para render/snapshot (método de cópia sob lock na Mesa ou lock próprio do Agent serializando todo acesso) — é o único ponto da techspec que não fecha com a superfície atual da Mesa
- Task 3.0: manter `StartDeliberation` colado a `convokePersona` e `ObservePersona` colado ao drain de `runSubagent`, preservando o contrato baseline→cumulativo
- Opcional: no teste de concorrência, adicionar leitura concorrente dos campos exportados via futuro getter de cópia quando existir (hoje a leitura concorrente ocorre via `entryIndexLocked` dentro dos métodos)

## Conclusão
APROVADO. A task 1.0 implementa exatamente o contrato da techspec (structs, tags, assinaturas, literais, semântica de baseline, transições), com concorrência segura por construção e sem exceção, escopo contido ao pacote `squad` e testes completos — todos os cenários exigidos pela task cobertos, mais edge cases além do mínimo. Os checks obrigatórios (`go build`, `go vet`, `gofmt -l`, `go test ./internal/squad/ -race`, `go test ./... -count=1`) estão verdes sem regressão. Os dois apontamentos de severidade baixa são notas de design para a task 3.0 (leitura thread-safe externa e pareamento de contrato no drain), não defeitos desta entrega.

## Checks executados
| Check | Resultado |
| --- | --- |
| `go build ./...` | ok |
| `go vet ./...` | ok |
| `gofmt -l .` | vazio |
| `go test ./internal/squad/ -race -count=1` | ok (35 testes) |
| `go test ./... -count=1` | ok (todos os pacotes, sem regressão) |
