# Review — Task 1.0: Tipo `squad.Mesa`

**Veredito: APROVADO**

## Escopo implementado

- `internal/squad/mesa.go` (novo): structs `MesaEntry` e `Mesa` com tags JSON exatas da techspec, constantes de status, mutex interno e os 8 métodos de transição (`Reset`, `ResetTurn`, `AddTokens`, `AddConvocation`, `StartDeliberation`, `FinishDeliberation`, `FinishTurn`, `ObservePersona`).
- `internal/squad/mesa_test.go` (novo): 12 testes cobrindo todos os casos exigidos pela task.
- Nenhum outro pacote foi alterado — `squad` permanece leaf (importa apenas `sync`), sem espelhos no Agent (escopo das tasks 2.0–3.0).

## Evidências do diff

### Conformidade com a techspec (Interfaces Principais)

- `MesaEntry`: campos `Name`/`Discipline`/`Status`/`Model`/`Tokens`/`Cost` com nomes, tipos e tags JSON idênticos ao snippet (`name` e `status` obrigatórios; `discipline`, `model`, `tokens`, `cost` com `omitempty`).
- `Mesa`: campos exportados `Roles`, `MaxConvocations`, `TokenBudget`, `Convocations`, `Tokens`, `Entries` com tags idênticas ao snippet (todos `omitempty`).
- Assinaturas dos 8 métodos idênticas à techspec, receivers em `*Mesa`.
- Status como literais em inglês: `waiting` | `deliberating` | `done` (constantes `StatusWaiting`/`StatusDeliberating`/`StatusDone`).
- Concorrência: todos os métodos travam `mu sync.Mutex` antes de qualquer acesso a campos; `entryIndexLocked` só é chamado com o lock segurado (sufixo `Locked` documenta o contrato, já que comentários são proibidos no código-base).
- `ObservePersona` soma os cumulativos do subagente ao baseline capturado em `StartDeliberation` (`entrada = baseline + cumulativo`), de modo que chamadas repetidas na mesma convocation não duplicam contagem e a entrada acumula entre convocations da mesma persona.
- `FinishTurn` converte `deliberating` → `done`; entradas `waiting` e `done` permanecem inalteradas.
- Persona inexistente (e Mesa zerada, sem `Reset`): `StartDeliberation`/`FinishDeliberation`/`ObservePersona` são no-op seguros, sem panic.

### Semântica de baseline (decisão de implementação)

- Baseline por persona em `baselines map[string]mesaBaseline` (campo unexported, invisível ao JSON), capturado em `StartDeliberation` a partir dos valores correntes da entrada — assim a segunda convocation da mesma persona acumula sobre a primeira, inclusive entre turnos (entries não são zeradas por `ResetTurn`).
- `ObservePersona` sem baseline capturado (ex.: Mesa restaurada de snapshot) trata baseline como zero — fallback seguro via leitura de mapa nil.

### Cobertura de testes (12 testes, todos exigidos pela task)

- `TestResetBuildsMesaFromKickoff` — 3 roles + disciplinas: ordem de convocação, status `waiting`, tetos preenchidos, contadores zerados.
- `TestResetRebuildsDirtyMesa` — `Reset` sobre Mesa suja reconstrói estado limpo.
- `TestResetCopiesRolesSlice` — `Reset` copia o slice de roles (sem aliasing com o kickoff).
- `TestResetTurnZeroesCountersKeepsEntries` — zera `Convocations`/`Tokens`, `Entries` intactas (`reflect.DeepEqual`).
- `TestDeliberationTransitions` — `waiting` → `deliberating` → `done`; outras entradas intocadas.
- `TestDeliberationUnknownPersonaIsNoOp` — persona inexistente e Mesa zerada: no-op sem panic.
- `TestObservePersonaAccumulatesAcrossConvocations` — 2 observações na mesma convocation (cumulativo, não soma); 2ª convocation acumula sobre a 1ª; modelo atualizado.
- `TestFinishTurnMarksDeliberatingDone` — `deliberating` → `done`; `waiting` e `done` preservados.
- `TestMesaFullCycle` — ciclo completo do critério de sucesso: `Reset` → `ResetTurn` → `AddConvocation`/`AddTokens` → `StartDeliberation` → `ObservePersona` (2×) → `FinishDeliberation` → `StartDeliberation` → `ObservePersona` → `FinishTurn`, com statuses e métricas finais corretos.
- `TestMesaJSONRoundTripPopulated` — Mesa populada serializa/desserializa idêntica (bytes estáveis + `DeepEqual` de `Roles`/`Entries`).
- `TestMesaJSONOmitEmpty` — Mesa zerada serializa como `{}`; Mesa só com kickoff omite `convocations`/`tokens`; entrada sem métricas omite `discipline`/`model`/`tokens`/`cost`.
- `TestMesaConcurrentAccess` — 100 goroutines × 50 iterações misturando mutação (`AddTokens`, `AddConvocation`, `ObservePersona`, `FinishTurn`) e leitura (`Start`/`Finish` em persona inexistente) sob `-race`; contadores determinísticos conferidos ao fim (prova que o mutex serializa sem lost updates).

## Checks executados

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok (sem copylocks — Mesa nunca é copiada por valor) |
| `gofmt -l .` | ✅ vazio |
| `go test ./internal/squad/ -race` | ✅ ok (12 testes novos + 24 existentes) |
| `go test ./...` | ✅ ok (todos os pacotes, sem regressão) |

## Notas e divergências documentadas

1. **Comentário do snippet da techspec omitido**: o snippet traz `// waiting | deliberating | done` em `MesaEntry.Status`; a regra do AGENTS.md (reiterada na task) proíbe comentários no código. Os literais estão nas constantes exportadas.
2. **Constantes de status exportadas** (`StatusWaiting`/`StatusDeliberating`/`StatusDone`): não constam na lista de interfaces da techspec, mas a task exige os literais exatos; constantes seguem o precedente do pacote (`SourceProject`/`SourceEmbedded`) e serão consumidas pela TUI na task 4.0. Nenhuma assinatura listada foi alterada.
3. **Campos unexported adicionais em `Mesa`** (`mu sync.Mutex`, `baselines`): o mutex é mandato da própria task ("mutex interno"); `baselines` é a única forma de implementar a semântica de baseline exigida. Ambos invisíveis ao JSON — a superfície serializada é exatamente a da techspec.
4. **`ResetTurn` não limpa baselines**: leitura literal do requisito ("zera Convocations/Tokens sem tocar em Entries"); baselines são sempre recapturadas por `StartDeliberation` antes de qualquer uso no fluxo do Agent, então baseline obsoleto nunca é lido.
5. **`Reset` copia defensivamente `k.Roles`**: higiene dentro da assinatura especificada, testada.

## Observação para tasks futuras

- A leitura segura da Mesa para render/snapshot (cópia sob lock) e o marshal em ponto seguro pertencem às tasks 3.0/2.0 (getter `Agent.Mesa()` com cópia; `writeSnapshot` em ponto quiescente). Nada foi pré-implementado aqui para evitar scope creep.
