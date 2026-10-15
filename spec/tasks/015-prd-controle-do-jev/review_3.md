# Review — Task 3.0: main.go — wiring via `Wire()`, validação fatal de modelo no startup e `--doctor` com estado do Jev

**Veredito: APROVADO**

## Escopo do diff

Arquivos modificados (apenas os da task):

- `main.go` — wiring via `router.Wire()`, `SetPinned`, validação fatal de modelo no startup, `runDoctor` estendido
- `main_test.go` — 4 testes novos + 3 helpers de teste

Nenhum arquivo fora do escopo: `internal/agent`, `internal/tui`, `internal/jev`, `internal/config`, `internal/router` intocados por esta task (respeitada a execução paralela da task 4.0 em `internal/tui/`).

## Evidências do diff × requisitos

### 3.1 — Wiring via `Wire()` + `SetPinned`

- Bloco de main.go:82-90 (JevRouter condicional + HeuristicRouter) substituído por `primary, fallback := router.Wire(cfg.Typesafe.APIKey, cfg.Typesafe.JevEnabled(), cfg.Model, cat)` (main.go:90). Assinatura bate com a `Wire()` da task 2.0.
- Par passado ao `agent.New(llmClient, primary, fallback, ...)` como hoje (main.go:95).
- `ag.SetPinned(cfg.Model)` aplicado imediatamente após a construção do agente (main.go:96), seguindo o fluxo da techspec: config → `Wire()` → `SetRouters` (via `New`) + `SetPinned`. `SetPinned("")` é inócuo: `decide()` só short-circuita com `pinned != ""` (agent.go:1040) — vazio segue automático.
- `SetPinned`/`SetRouters` consumidos como estão; `internal/agent` sem mudanças. ✓

### 3.2 — Validação fatal de modelo no startup

- `validateModel(model, cat)` (main.go:259-267): vazio → nil (automático); fora do catálogo → `unknown model %q: not in the catalog` apontando o valor rejeitado.
- Chamada no startup (main.go:60-63) com o padrão exigido `kterminal: <err>` + exit 1 (espelho do padrão de main.go:60-63 original). Validação no ponto de wiring (main.go), não no parsing da config — conforme techspec ("config não conhece o catálogo").
- Posicionada após o early-return do `--doctor`: doctor continua utilizável como diagnóstico com config quebrada; o boot da TUI falha antes de criar sessão/agente.
- Teste: `TestValidateModel` — erro contém o valor rejeitado (`not-in-the-catalog`), vazio e default do catálogo passam.

### 3.3 — `runDoctor` com estado do Jev

- `doctorRoutingLines(cfg)` (main.go:269-282) impressa na seção de config (main.go:198), após `typesafe api key`: `jev enabled: yes/no` (derivado de `JevEnabled()`, cobrindo nil → yes retro-compat) e `model: <name>` ou `model: auto`.
- `doctorJevSection(cfg, baseURL)` (main.go:284-308): com Jev desabilitado imprime `jev: disabled` e retorna **antes** de construir o cliente Jev — zero chamadas HTTP ao endpoint Typesafe. Habilitado sem chave mantém `jev: NOT CONFIGURED`; habilitado com chave faz o probe como hoje (`jev: reachable/UNREACHABLE`), alimentando o exit code.
- Teste: `TestDoctorRoutingLines` cobre as 4 combinações (yes/no × auto/name).

### 3.4 — Testes de integração

- `TestRunDoctorJevDisabledSkipsProbe`: `enabled=false` **com api key presente** → saída contém `jev enabled: no`, `model: <name>` e `jev: disabled`; servidor httptest do endpoint Typesafe (witness) recebe **0 requisições**.
- `TestRunDoctorJevEnabledProbesEndpoint`: `enabled=true` + api key → saída contém `jev enabled: yes`, `model: auto` (1ª execução) e `model: <name>` (2ª execução com `cfg.Model` definido); witness recebe exatamente 1 requisição por probe — prova que o witness é significativo (o mesmo servidor que receberia o probe quando habilitado não recebe nada quando desabilitado).
- Nenhum teste toca rede real: gateway fica NOT CONFIGURED (sem HTTP) e o probe Jev aponta para o httptest.

## Decisões de design (necessárias para os testes exigidos)

1. **`runDoctor` ganhou o parâmetro `jevBaseURL string` e passou a retornar `int`** (em vez de chamar `os.Exit(1)` internamente). Necessário para os testes de integração exigidos pela task: o padrão "httptest como testemunha" exige injetar o endpoint, e `os.Exit` dentro de `runDoctor` mataria o processo de teste. Comportamento preservado em main.go:52-58 — `os.Exit(code)` apenas quando `code != 0`; sucesso retorna como antes (exit 0). Em produção o endpoint é `jev.DefaultBaseURL` (inalterado).
2. **`client.BaseURL = baseURL`** em `doctorJevSection` usa campo público existente de `jev.Client` — sem mudanças em `internal/jev` (restrição do PRD respeitada).

## Observações (não bloqueantes)

1. A mensagem `jev: NOT CONFIGURED — routing falls back to heuristics` foi mantida (fora do escopo listado da task), mas ficou semanticamente defasada com a REQ-002: sem api key o roteador principal agora é o `FixedRouter` no default do catálogo, não a heurística. Recomendo ajuste em follow-up (1 linha em `doctorJevSection`).
2. O caminho de `exit 1` do startup em si não é unit-testável sem extrair um `run()` de `main()` — mesma situação das demais error paths de `main()` (config.Load, resolveSession). A lógica de erro (`validateModel`) está testada, incluindo a mensagem com o valor rejeitado.
3. `--doctor` não valida `cfg.Model` (imprime o valor cru): deliberado — doctor é ferramenta de diagnóstico e deve rodar mesmo com config quebrada; a validação fatal é do caminho de startup, conforme a task.

## Critérios de sucesso

- `go build ./... && go vet ./... && go test ./...` verde ✓
- Startup com `model` fora do catálogo → boot falha com erro apontando o valor ✓ (main.go:60-63 + `TestValidateModel`)
- `--doctor` reflete `enabled` e o modelo fixo ✓ (`TestDoctorRoutingLines` + testes de integração)
- `--doctor` com `enabled=false` → nenhuma requisição HTTP ao endpoint Typesafe ✓ (`TestRunDoctorJevDisabledSkipsProbe`, witness com 0 requisições)
- Startup com config legada (sem campos novos) → par de routers idêntico ao comportamento atual ✓ (`Wire(key, JevEnabled()=true, "", cat)` → Jev+Heuristic; coberto pelos testes de `Wire()` da task 2.0 — `internal/router/wire_test.go`)

## Checks executados

| Comando | Resultado |
| --- | --- |
| `go build ./...` | ✅ sem erros |
| `go vet ./...` | ✅ sem apontamentos |
| `gofmt -l .` | ✅ vazio (nenhum arquivo listado) |
| `go test ./... -count=1` | ✅ todas as packages ok |
| `go test . -race` (pacote main, testes novos + existentes) | ✅ sem data races |

Nota de paralelismo: durante a primeira execução da suite completa houve falha de build transitória em `internal/tui/commands_test.go` (import não usado — arquivo em edição pela task 4.0 paralela). Re-executado após aguardar: `internal/tui` passou isoladamente e a suite completa final passou 100%, confirmando ruído da task paralela.

## Convenções

- Sem comentários no código ✓
- Código em inglês ✓
- `strings.Builder` sempre por ponteiro ✓ (`var b strings.Builder` + `&b` nos helpers novos)
- Erros de startup → stderr + exit 1 apontando o valor rejeitado ✓
- Validação nos pontos de wiring, não no parsing ✓
