# Tarefa 1.0: Config — campos `Typesafe.Enabled`/`Config.Model`, accessors, env overrides e `Save()` normalizado

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Toggle persistente do Jev (campo `[typesafe] enabled`, default true retro-compatível, env override `KTERMINAL_TYPESAFE_ENABLED`)
- REQ-002 — Modelo fixo determinístico (campo top-level `model`, env override `KTERMINAL_MODEL`)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Estender `internal/config` com os dois campos novos da feature: `Typesafe.Enabled *bool` (tri-state: nil = true, para retro-compatibilidade com configs legadas sem o campo) e `Config.Model string` top-level (vazio = automático). Incluir o accessor `JevEnabled()`, os env overrides `KTERMINAL_TYPESAFE_ENABLED` e `KTERMINAL_MODEL` no `Load()`, e a normalização nil → true no `Save()`. É a fundação da feature — nenhum outro pacote muda nesta task. A validação de `model` contra o catálogo **não** fica aqui: `config` é pacote folha e não conhece o catálogo (validação nos pontos de wiring, tasks 3.0 e 6.0).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês, `config` permanece folha (não importa `catalog`), env overrides seguem o padrão de prevalência dos knobs existentes em `Load()` (config.go:101-127).
</skills>

<requirements>
- Structs e assinaturas exatamente como na seção "Interfaces Principais" da techspec.md: `Typesafe{APIKey string; Enabled *bool}` com tag `toml:"enabled"`, `func (t Typesafe) JevEnabled() bool`, `Config.Model string` com tag `toml:"model"`
- `JevEnabled()` retorna true quando `Enabled == nil` (retro-compatibilidade: config legada sem o campo preserva comportamento atual)
- Env override `KTERMINAL_TYPESAFE_ENABLED` lido com `strconv.ParseBool`; valor inválido → `Load()` retorna erro
- Env override `KTERMINAL_MODEL` sobrescreve o valor do arquivo quando não vazio
- `Save()` normaliza `Enabled == nil` para `true` explícito antes do marshal (risco "Marshal de `*bool` nil" da techspec)
- Env override prevalece sobre o arquivo, como nos knobs existentes (restrição do PRD)
</requirements>

## Subtarefas

- [x] 1.1 Adicionar campo `Enabled *bool` (tag `toml:"enabled"`) ao struct `Typesafe` (config.go:32) e o accessor `JevEnabled()` com semântica nil → true
- [x] 1.2 Adicionar campo `Model string` (tag `toml:"model"`) top-level ao struct `Config` (config.go:43)
- [x] 1.3 Adicionar env overrides no `Load()` (config.go:94): `KTERMINAL_TYPESAFE_ENABLED` (ParseBool, inválido → erro de Load) e `KTERMINAL_MODEL`
- [x] 1.4 Normalizar `Enabled` nil → true explícito no `Save()` (config.go:175) antes do marshal TOML
- [x] 1.5 Testes de unidade em `config_test.go` cobrindo defaults, tri-state, env overrides, round-trip de `Save()` e fixture de config legada

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (bloco Config), "Arquitetura do Sistema" (componente `internal/config`) e "Riscos Conhecidos" (marshal de `*bool` nil). Precedentes de env override no mesmo arquivo: `KTERMINAL_TYPESAFE_API_KEY` (config.go:127) e os knobs de `LLM`.

## Critérios de Sucesso

- `go test ./internal/config/` verde
- Config legada sem os campos novos → `JevEnabled()` == true e `Model` == "" (comportamento atual preservado)
- `enabled = false` no TOML → `JevEnabled()` == false
- `KTERMINAL_TYPESAFE_ENABLED=false` vence o valor do arquivo; valor não-parseável → `Load()` falha com erro claro
- Round-trip `Load()` → `Save()` → `Load()` materializa `enabled = true` explícito sem mudança semântica

## Testes da Tarefa

- [x] Testes de unidade (`config_test.go`):
  - [x] `Enabled` nil → `JevEnabled()` true; explícito true → true; explícito false → false
  - [x] Env `KTERMINAL_TYPESAFE_ENABLED` (valores `true`/`false`/`1`/`0`) prevalece sobre o arquivo; valor inválido → erro de `Load()`
  - [x] Env `KTERMINAL_MODEL` prevalece sobre o arquivo; vazio → valor do arquivo
  - [x] Round-trip `Save()`: `Enabled` nil normalizado para true explícito; re-`Load()` preserva semântica
  - [x] Fixture de config legada (sem `enabled` e sem `model`) → defaults retro-compatíveis
- [x] Testes de integração: N/A (pacote isolado; integração acontece nas tasks 3.0–6.0)
- [x] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/config/config.go` (modificado — structs, `Load()`, `Save()`)
- `internal/config/config_test.go` (modificado)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência — seções citadas acima)
