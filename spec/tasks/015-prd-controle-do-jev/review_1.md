# Review — Task 1.0: Config — campos `Typesafe.Enabled`/`Config.Model`, accessors, env overrides e `Save()` normalizado

**Veredito: APROVADO**

## Evidências do diff

Arquivos modificados (somente `internal/config`, conforme o escopo):

- `internal/config/config.go` (+25/-1)
  - `Typesafe` ganha `Enabled *bool` com tag `toml:"enabled"` — exatamente a struct da techspec (`Typesafe{APIKey string; Enabled *bool}`).
  - `func (t Typesafe) JevEnabled() bool` — receiver por valor, semântica nil → true (retro-compatível).
  - `Config` ganha `Model string` com tag `toml:"model"` top-level. Declarado como **primeiro campo** do struct: em TOML, escalar top-level precisa preceder os headers de tabela — se fosse o último campo, o marshal emitiria `model = "..."` após `[agent]` e a chave viraria `agent.model` na re-leitura. A spec não prescreve posição; a escolha é necessária para o round-trip funcionar (coberto por `TestSaveRoundTripModel`).
  - `Load()`: env override `KTERMINAL_TYPESAFE_ENABLED` via `strconv.ParseBool` — valor inválido → `Load()` retorna `fmt.Errorf("invalid typesafe.enabled %q: %w", v, err)`, seguindo o padrão de `invalid llm.max_retries %q: %w`. Env override `KTERMINAL_MODEL` sobrescreve o arquivo quando não vazio. Ambos agrupados após o knob `TYPESAFE_API_KEY`, no padrão de prevalência dos knobs existentes (env vence arquivo).
  - `Save()`: normaliza `Enabled == nil` para `&true` antes do `toml.Marshal` (risco "Marshal de `*bool` nil" da techspec). A normalização muta também o cfg em memória — semanticamente transparente (nil ≡ true).
- `internal/config/config_test.go` (+275)
  - `TestJevEnabledAccessorTriState` — tri-state puro: nil → true, true → true, false → false.
  - `TestJevEnabledNilDefaultsTrue` — config sem `enabled` → `Enabled == nil` e `JevEnabled() == true`.
  - `TestJevEnabledExplicitTOML` — `enabled = true`/`enabled = false` no TOML → semântica correta com ponteiro materializado.
  - `TestLoadTypesafeEnabledEnvOverridesFile` — tabela com `false`/`true`/`1`/`0` prevalecendo sobre o arquivo (4 subtests).
  - `TestLoadTypesafeEnabledEnvInvalidRejected` — `KTERMINAL_TYPESAFE_ENABLED=maybe` → erro de `Load()` contendo `typesafe.enabled` e o valor.
  - `TestLoadModelEnvOverridesFile` — `KTERMINAL_MODEL` prevalece; vazio → valor do arquivo.
  - `TestSaveNormalizesNilEnabledToExplicitTrue` — round-trip `Load() → Save() → Load()`: arquivo materializa `enabled = true`, reload volta explícito true, `JevEnabled()` preservado.
  - `TestSaveRoundTripPreservesExplicitFalse` — `enabled = false` sobrevive ao save/reload (normalização não clobbers false).
  - `TestSaveRoundTripModel` — `Model` persiste no arquivo top-level e no reload.
  - `TestLoadLegacyConfigDefaults` — fixture de config legada (llm/typesafe/squad, sem enabled/model) → `Model == ""`, `Enabled == nil`, `JevEnabled() == true`, `api_key` intacta.

Conformidade com os padrões do repo (AGENTS.md):

- Sem comentários no código — confirmado no diff.
- Código em inglês — confirmado.
- `config` permanece pacote folha — imports inalterados (`fmt`, `os`, `path/filepath`, `strconv`, `time`, `BurntSushi/toml`); nenhuma importação de `catalog`; validação de modelo fica nas tasks 3.0/6.0, como manda a techspec.
- Sem scope creep — nenhum outro pacote tocado.

## Resultado dos checks

| Check | Resultado |
| --- | --- |
| `go build ./...` | ✅ OK |
| `go vet ./...` | ✅ OK |
| `gofmt -l .` | ✅ lista vazia |
| `go test ./internal/config/` | ✅ ok — 29/29 testes (18 pré-existentes + 11 novos), 0.28s |
| `go test ./...` (extra, além do escopo) | ✅ todos os pacotes ok |

## Ressalvas (não bloqueantes)

1. **Posição de `Model` no struct**: primeiro campo de `Config` (não último) — decisão de implementação necessária para o marshal produzir TOML válido; a techspec não prescreve ordem de campos.
2. **`Save()` muta o receiver**: a normalização nil → true também materializa em memória. Semanticamente transparente (nil ≡ true) e é a leitura direta do requisito "normaliza antes do marshal".
3. **Referência imprecisa na task**: o precedente citado como `KTERMINAL_TYPESAFE_API_KEY` (config.go:127) é na verdade `TYPESAFE_API_KEY` (sem prefixo, linha 126 no arquivo original). O padrão de prevalência citado é exatamente o seguido; nenhum conflito com a techspec.
4. **Observação de ambiente**: durante a verificação houve duas falhas transitórias do toolchain — um SIGBUS dentro do compilador Go 1.27.1 ao compilar `internal/tui` (pacote não modificado por esta task) e um hang de `go test -v`. Ambas não reproduziram em re-execução e não têm relação com o diff; registradas apenas como ruído de ambiente.

## Subtarefas

- [x] 1.1 Campo `Enabled *bool` + accessor `JevEnabled()` (nil → true)
- [x] 1.2 Campo `Model string` top-level
- [x] 1.3 Env overrides `KTERMINAL_TYPESAFE_ENABLED` (ParseBool, inválido → erro) e `KTERMINAL_MODEL`
- [x] 1.4 Normalização nil → true no `Save()`
- [x] 1.5 Testes de unidade (defaults, tri-state, env, round-trip, fixture legada)
