# Review — Task 7.0: Verificação final — suite completa e métricas do PRD via transcript JSONL

**Veredito: APROVADO**

## Escopo revisado

Somente testes, conforme delimitado na task: `internal/agent/agent_test.go` (teste de integração 7.2 + helper) e `internal/tui/commands_test.go` (checagens de persistência/retro-compatibilidade 7.3). **Nenhum arquivo de produção modificado** — a task é de fechamento/verificação. O working tree contém também as alterações ainda não commitadas das tasks 1.0–6.0 (branch da feature em andamento); esta review cobre exclusivamente os deltas da 7.0.

## Evidências do diff

### 7.1 — Verificação completa

Cadeia completa executada e verde (ver tabela de checks). `gofmt -l .` verificado isoladamente por listar arquivos com exit 0 — lista vazia confirmada.

### 7.2 — Teste de integração ponta-a-ponta (`agent_test.go`)

- Helper `countingJev`: mock httptest do endpoint `POST /v1/systemone` que incrementa um contador `atomic.Int32` — testemunha de requisições ao Typesafe.
- `TestJevDisabledFullSessionZeroRequestsFixedRoutes`:
  - **Cenário**: agente construído com `JevRouter` apontando para o mock (estado pré-toggle, tornando o mock uma testemunha alcançável) e então re-wirado com `SetRouters(router.Wire("test-key", false, "glm-5.3", cat))` + `SetPinned("glm-5.3")` — espelhando exatamente o `rebuildAgent()` pós-`/jev off` + `/model`. Asserções de pré-condição: `*router.FixedRouter` como primary e fallback nil.
  - **Turno completo com tool loop**: `ag.Run("run echo hi")` → gateway responde tool call `bash` (executado de verdade, precedente `TestAgentToolLoopReroutes`) e depois a resposta final — 2 chamadas LLM, 2 eventos `route` no agente principal, ambos `pin`/`glm-5.3`.
  - **Subagente via `RunSyncPersona`**: persona `architect` resolvida do squad store, `pinned=""` (sem squad pin) → roteia pelo `FixedRouter` herdado do pai: evento `route` depth 1 com `Router == "fixed"`, `Model == "glm-5.3"`, `Agent == "architect"`.
  - **Métricas do PRD** (detalhadas na seção seguinte): zero requisições ao mock, transcript com 100% das rotas no modelo fixo, nenhum router `heuristic`.
  - **Prompts com keywords de heurística de propósito**: "run echo hi" (keyword routine "run") e "review the design" (keyword hard "design") — se a heurística fosse roteador principal, as decisões divergeriam do modelo fixo e o switch de router no transcript falharia. Provam "nenhuma decisão por keyword com Jev desabilitado" no caminho negativo.

### 7.3 — Persistência e retro-compatibilidade (`commands_test.go`)

Análise de lacunas contra o já coberto pelas tasks 4.0–6.0:

- **Já coberto**: `/jev off` persiste `enabled=false` (`TestJevOffPersistsAndOpensPicker`); `/model <name>` persiste `model` (`TestModelCommandPersistsPinAndRewires`); `/model auto` limpa (`TestModelAutoClearsPersistedModel`); fixture legada unitária (`TestLoadLegacyConfigDefaults`); fixture legada na tela `/config` (`TestConfigSaveMaterializesLegacyFields`); toggle em runtime com mock contando hits (`TestJevOffStopsJevRequestsInNextTurn`); doctor sem probe (`TestRunDoctorJevDisabledSkipsProbe`); startup com modelo inválido (`TestValidateModel`).
- **Lacuna 1 — fechada**: persistência **combinada** `/jev off` + `/model <name>` (nenhum teste anterior recarregava os DOIS campos da mesma sessão). Novo `TestJevOffThenModelCommandPersistsBothFields`: `/jev off` → esc no picker → `/model glm-5.3` → re-`Load` confirma `enabled=false` **e** `model="glm-5.3"`, pin aplicado, `*router.FixedRouter` com fallback nil.
- **Lacuna 2 — fechada**: fixture legada **de ponta a ponta** (a cobertura existente parava no `Load`/materialização, sem exercitar o roteamento). Novo `TestLegacyConfigKeepsJevActiveEndToEnd`: arquivo TOML legado (sem `enabled`/`model`, com `typesafe.api_key`) → `config.Load()` → `JevEnabled()==true` e `Model==""` → `router.Wire` devolve `*JevRouter` + `*HeuristicRouter` → turno real pela TUI roteia via `jev` para a escolha do mock (`glm-5.2`) e o mock confirma a requisição (hits ≥ 1). Comportamento atual preservado de ponta a ponta.

## Métricas do PRD medidas

| Métrica (PRD/techspec) | Valor medido | Evidência |
| --- | --- | --- |
| Requisições ao endpoint Typesafe com `enabled=false` (sessão inteira, incluindo subagente) | **0** | `jevHits.Load() == 0` após turno com tool loop (2 chamadas) + `RunSyncPersona` (1 chamada); o mock estava inicialmente wired como `JevRouter` — qualquer vazamento (re-wiring falho, router stale herdado) incrementaria o contador |
| Entradas de rota no transcript JSONL da sessão de teste | **3** (2 agente principal depth 0 + 1 subagente depth 1) | `transcriptLines` filtrando `type == "route"` |
| % das entradas de rota no mesmo modelo | **100%** (`glm-5.3` nas 3) | assert por entrada + contagem `routes == 3` |
| Distribuição de `Router` no transcript | **pin: 2, fixed: 1, heuristic: 0, jev: 0** | switch com case `pin`/`fixed` e default fatal — qualquer router fora do conjunto (incluindo `heuristic`) falha o teste |
| Modelo efetivo nas chamadas ao gateway | **3/3 com `model == "glm-5.3"`** no corpo da requisição | `chatCall.Model` capturado pelo `taskFlowGateway` |
| Escolha de modelo intacta após restart | **preservada** | re-`Load` após `/jev off` + `/model glm-5.3` → `enabled=false` + `model="glm-5.3"` |
| Retro-compatibilidade (config legada) | **preservada** | `JevEnabled()==true` com api key; turno roteia via `jev` com requisição confirmada ao mock |

## Conformidade com padrões (AGENTS.md / techspec)

- **Sem comentários no código**: nenhuma adições contêm comentários. ✓
- **Código em inglês**: identificadores e mensagens. ✓
- **Sem comportamento novo / sem código de produção alterado**: diff da 7.0 toca apenas `*_test.go`. ✓
- **Padrão go test + httptest**: helpers seguem os precedentes (`mockJev`, `taskFlowGateway`, `countingJev` da TUI espelhado como helper local do pacote agent). ✓
- **Sem TestSprite**: E2E via inspeção do transcript JSONL, conforme a seção "Testes E2E" da techspec. ✓
- **Sem scope creep**: 3 testes + 1 helper + 1 import — nada além do que a task pede. ✓

## Observações (não-bloqueantes)

1. **Decisão de design no teste 7.2**: o agente nasce com `JevRouter` wired ao mock e é re-wirado via `Wire(...)` disabled (espelho do `rebuildAgent` pós-`/jev off`), em vez de construído direto no estado disabled. Motivo: manter o mock como testemunha alcançável — se `SetRouters` não pegasse, ou se subagentes herdassem router stale, o contador dispararia. O caminho de boot-fresh (Wire já com disabled, como no `main.go`) permanece coberto por `wire_test.go` (`Wire` nunca retorna `JevRouter` com disabled) e pelos testes TUI.
2. **Flake único de infraestrutura**: na primeira execução do baseline (antes de qualquer mudança desta task), a suite completa crashou uma vez com dump de registradores (runtime fault), sem pacote identificado no trecho capturado. Não reproduziu em 4 execuções completas subsequentes nem nos runs com `-race`. Anterior às mudanças da 7.0; registrado como observação, não como defeito da feature.

## Checks executados

| Comando | Resultado |
| --- | --- |
| `go build ./...` | ✅ ok |
| `go vet ./...` | ✅ ok |
| `gofmt -l .` | ✅ vazio (nenhum arquivo listado; verificado isoladamente) |
| `go test ./... -count=1` | ✅ ok — 12 pacotes com testes verdes (14 no total; `clipboard` e `jev` sem arquivos de teste) |
| `go test ./internal/agent -run TestJevDisabledFullSessionZeroRequestsFixedRoutes -count=5 -race` | ✅ ok (estável, sem data races) |
| `go test ./internal/tui -run 'TestJevOffThenModelCommandPersistsBothFields\|TestLegacyConfigKeepsJevActiveEndToEnd' -count=5 -race` | ✅ ok (estável, sem data races) |

## Conclusão

As três métricas de sucesso do PRD estão materializadas em testes que falham se regredirem: zero chamadas ao Typesafe com `enabled=false` (mock contador como testemunha, incluindo subagente), 100% das rotas do transcript no modelo fixo com `Router ∈ {pin, fixed}` (nenhuma decisão por keyword), e persistência entre restarts (re-`Load` preserva `enabled=false` + `model`). Retro-compatibilidade confirmada de ponta a ponta com config legada. Suite completa verde. Task 7.0 **aprovada** — feature "Controle do Jev" encerrada.
