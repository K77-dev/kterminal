# Tech Spec — Controle do Jev: toggle de roteamento e modelo fixo

## Requisitos Atendidos

- REQ-001 — Toggle persistente do Jev (`[typesafe] enabled`, `/jev on|off`, env override, re-wiring sem restart)
- REQ-002 — Modelo fixo determinístico com Jev desabilitado (campo `model`, `default_model` como fallback, validação no startup)
- REQ-003 — Picker de modelos e `/model` persistente (popup no estilo `/mode`, `/model auto`, semântica de pin preservada)
- REQ-004 — Descoberta: `/config`, `--doctor`, `/help` e status bar

## Resumo Executivo

A feature introduz dois campos na config — `[typesafe] enabled` (tri-state, default true) e `model` (top-level, default vazio) — e um novo `FixedRouter` em `internal/router` que devolve sempre o mesmo modelo (o fixo configurado ou o `default_model` do catálogo). O wiring dos routers, hoje duplicado entre `main.go` e `rebuildAgent` na TUI, é centralizado em `router.Wire()`, que decide o par (primary, fallback) a partir de (api_key, enabled, model, catálogo). O `cfg.Model` é aplicado como pin existente (`SetPinned`) no agente principal e carregado dentro do `FixedRouter` para cobrir subagentes — que não herdam o pin do pai no código atual.

O agente (`internal/agent`) não precisa de mudanças: `SetPinned`/`SetRouters` já existem e subagentes já copiam `Router`/`Fallback`. A TUI ganha o comando `/jev`, o picker de modelos (padrão do popup `/mode`), dois inputs novos na tela `/config`, indicador de roteamento na status bar e entradas em `/help`/sugestões. O `--doctor` imprime o estado do Jev e o modelo fixo, e não faz probe do endpoint Typesafe quando o Jev está desabilitado.

## Arquitetura do Sistema

### Visão Geral dos Componentes

- **`internal/config`** — novos campos `Typesafe.Enabled *bool` (nil = true) e `Config.Model string`; accessors `JevEnabled()`; env overrides `KTERMINAL_TYPESAFE_ENABLED` e `KTERMINAL_MODEL`; `Save()` normaliza `Enabled` nil para true explícito.
- **`internal/router`** — novo `FixedRouter` (roteador determinístico) e `Wire()` (factory única do par primary/fallback). `JevRouter` e `HeuristicRouter` inalterados; Heuristic permanece só como fallback de erro com Jev habilitado.
- **`main.go`** — wiring via `router.Wire()`, validação de `cfg.Model` contra o catálogo no startup (erro fatal apontando o valor), `--doctor` com estado do Jev e modelo fixo.
- **`internal/tui`** — `/jev on|off|status`, `/model` persistente com picker, `rebuildAgent()` usando `Wire()` + `SetPinned(cfg.Model)`, tela `/config` com 6 inputs, status bar com indicador `jev`/`jev off`, `/help` e sugestões atualizadas.
- **`internal/agent`** — sem mudanças. `decide()` já short-circuita no pin; subagentes (`RunSyncPersona`) já copiam `Router`/`Fallback` e recebem pin de squad via parâmetro.
- **`internal/catalog`** — sem mudanças (`Get`, `DefaultModel` consumidos como estão).

Fluxo de dados: config (arquivo + env) → `Load()` resolve tri-state → `router.Wire()` produz (primary, fallback) → `agent.SetRouters` + `agent.SetPinned(cfg.Model)` → `decide()` por chamada. Alterações em runtime (`/jev`, `/model`, `/config`) seguem o mesmo caminho: mutar `cfg` em memória → `cfg.Save()` → `rebuildAgent()`.

## Design de Implementação

### Interfaces Principais

`FixedRouter` — determinístico, com degradação em cadeia para o caso de candidatos filtrados por visão:

```go
type FixedRouter struct {
	Model   string
	Default string
}

func (r *FixedRouter) Route(_ context.Context, _ string, candidates []catalog.Model) (Decision, error)
```

Ordem de decisão: `Model` se presente nos candidatos → `Default` se presente → `candidates[0]`. Confiança 1, `Router: "fixed"`. Sem candidatos → erro (contrato da interface).

`Wire()` — única fonte de wiring, usada por `main.go` e `rebuildAgent()`:

```go
func Wire(apiKey string, jevEnabled bool, fixedModel string, cat *catalog.Catalog) (router.Router, router.Router)
```

- Jev ativo (`jevEnabled && apiKey != ""`): primary `JevRouter`, fallback `HeuristicRouter{Default: cat.DefaultModel}` (comportamento atual).
- Jev inativo: primary `FixedRouter{Model: fixedModel, Default: cat.DefaultModel}`, fallback `nil`.

Config:

```go
type Typesafe struct {
	APIKey  string `toml:"api_key"`
	Enabled *bool  `toml:"enabled"`
}

func (t Typesafe) JevEnabled() bool
```

`JevEnabled()` retorna true quando `Enabled == nil` (retro-compatibilidade). `Config.Model string \`toml:"model"\`` top-level. Validação do modelo contra o catálogo fica nos pontos de wiring (startup e salvamento), não no parsing — `config` não conhece o catálogo.

### Comandos TUI

| Comando | Comportamento |
| --- | --- |
| `/jev` | Exibe estado: on/off, modelo fixo vigente (`cfg.Model` ou default do catálogo) |
| `/jev on` | Sem api_key → `errorBox`, nada persiste. Com chave → `Enabled=true`, save, rebuild |
| `/jev off` | `Enabled=false`, save, rebuild, abre o picker (REQ-003) |
| `/model` | Abre o picker (comportamento novo) |
| `/model <name>` | Valida contra catálogo → `cfg.Model=name`, save, `SetPinned(name)`, rebuild |
| `/model auto` | `cfg.Model=""`, save, `SetPinned("")`, rebuild |

Picker: estados `modelOpen`/`modelSelected` + `openModelPopup()`/`selectModel()`/`modelPopupView()`, espelhando o popup `/mode` (commands.go:120-170). Itens: entrada `auto` + modelos do catálogo com resumo `$in/$out · ~tok/s`; setas navegam, enter seleciona (persiste + rebuild), esc cancela sem alterar. Pre-seleciona o pin corrente quando definido.

### Pontos de Persistência

- `saveConfig()` (tui.go:1245): passa a mapear 6 inputs — acrescenta `jev enabled (yes/no)` e `model` — e valida o modelo antes de salvar; inválido → `errorBox` e mantém a tela de config. Salvo válido → `rebuildAgent()` (que agora também aplica `SetPinned(cfg.Model)`).
- `rebuildAgent()` (tui.go:1261): substitui o bloco de wiring duplicado por `router.Wire(m.cfg.Typesafe.APIKey, m.cfg.Typesafe.JevEnabled(), m.cfg.Model, m.catalog)` + `SetRouters` + `SetPinned`.
- Status bar (`hintBar`, tui.go:1412): indicador de roteamento derivado de `m.cfg` — `jev` quando `JevEnabled() && api_key != ""`, `jev off` quando desabilitado; `pinned <model>` existente cobre o modelo fixo (tui.go:1446).
- `--doctor` (main.go:181): imprime `jev enabled: yes/no` na seção de config e `model: <name>` ou `model: auto`; seção Jev imprime `jev: disabled` sem probe quando desabilitado.
- Sugestões: `/jev` entra em `nativeCommands` (commands.go:18); `/help` ganha linha do `/jev` e atualiza a descrição de `/model`.

### Endpoints de API

Não aplicável — sem endpoints novos. O cliente Jev (`internal/jev`) e o protocolo Typesafe não mudam; o cliente LLM não muda.

## Pontos de Integração

- **Jev (Typesafe)**: integração existente; a única mudança é quando o cliente é instanciado — `Wire()` nunca cria `JevRouter` com Jev desabilitado, garantindo zero chamadas ao endpoint em toda a sessão (incluindo subagentes, que herdam o `Router` do pai no momento da criação).
- **Gateway LLM**: inalterado; `FixedRouter` só escolhe nomes que estão nos candidatos (`Catalog.Available(remote)`).
- **Modos de falha**: modelo fixo ausente no gateway → já excluído dos candidatos pelo `Available`, `FixedRouter` degrada para `Default`/primeiro candidato de forma determinística; modelo fixo sem visão + mensagem com imagens → pin é pulado (agent.go:1041) e a mesma cadeia determinística se aplica.

## Verificações Técnicas

### Segurança

Sem superfície nova: `enabled` e `model` são valores não-sensíveis; `model` é validado contra o catálogo (denylist natural) antes de qualquer efeito; api_key continua mascarada no `/config` e no `--doctor` (`display()`). Env overrides seguem o padrão existente de prevalência sobre o arquivo.

### Arquitetura

- `router` já importa `jev` e `catalog` — `Wire()` não cria ciclos.
- Direção de dependência preservada: `config` permanece folha (não conhece catálogo); validação de modelo fica nos pontos de wiring.
- Mudança intencional de comportamento (PRD REQ-002): usuários sem api_key passam do `HeuristicRouter` por keywords para `FixedRouter` no default do catálogo — determinismo substitui a heurística como roteador principal.
- Ponto de atenção: subagentes não herdam o pin do pai (fato do código atual, agent.go:383-402); `FixedRouter` carrega `cfg.Model` e cobre esse caso com Jev desabilitado. Com Jev habilitado, subagentes continuam roteando via Jev — comportamento atual, fora de escopo.

### Infraestrutura

Binário único, sem dependências novas. Rollback = remover os campos da config (retro-compat garante comportamento anterior). `Save()` reescreve o TOML completo com `enabled` explícito — configs legadas ganham o campo materializado no primeiro save, sem efeito semântico.

## Abordagem de Testes

### Testes Unidade

- `config_test.go`: `Enabled` nil → `JevEnabled()` true; explícito false; env `KTERMINAL_TYPESAFE_ENABLED` (ParseBool, valor inválido → erro de Load); env `KTERMINAL_MODEL`; round-trip `Save()` normaliza nil → true; fixture de config legada sem os campos preserva comportamento.
- `router` (novo): `FixedRouter` — modelo nos candidatos; degradação Model→Default→primeiro com filtro de visão; sem candidatos → erro. `Wire()` — chave+enabled → (Jev, Heuristic); enabled=false → (Fixed, nil); sem chave → (Fixed, nil); `fixedModel` vazio → Fixed no default.

### Testes de Integração

- `agent_test.go` (padrão existente com httptest): com Jev desabilitado, servidor mock do Jev não recebe nenhuma requisição em turno completo com tool loop; subagentes (`RunSyncPersona`) roteiam para o modelo fixo; `Decision.Router == "fixed"`/`"pin"` conforme o caminho.
- `tui_test.go`/`commands_test.go`: `/jev on` sem chave → erro e arquivo inalterado; `/jev off` → config salva + `modelOpen`; `/model <name>` persiste no arquivo (re-`Load` e conferir); `/model auto` limpa; picker esc não altera nada, enter persiste e re-wira; status bar distingue `jev`/`jev off`/`pinned`; `/help` lista `/jev`; sugestões incluem `/jev`; `/config` com modelo inválido → erro e permanece na tela.
- `main_test.go`: startup com `model` fora do catálogo → exit com erro apontando o valor; `runDoctor` com `enabled=false` imprime `jev: disabled` sem requisição HTTP (httptest como testemunha).

### Testes de E2E

Não aplicável TestSprite (TUI Go sem frontend web). A verificação E2E da métrica do PRD — "modelo fixo idêntico em 100% das rotas do transcript" — é feita inspecionando o JSONL de sessão: todas as entradas de rota com o mesmo `Model` e `Router ∈ {pin, fixed}` com Jev desabilitado.

## Sequenciamento de Desenvolvimento

1. **Config** — campos, accessors, env overrides, `Save()` normalizado (fundação; testável isolada).
2. **Router** — `FixedRouter` + `Wire()` com testes de unidade (sem depender da TUI).
3. **Wiring + doctor** — `main.go` usa `Wire()`, validação fatal de modelo no startup, seções novas do `--doctor`.
4. **TUI comandos** — `rebuildAgent()` via `Wire()`; `/jev`; `/model` persistente; picker de modelos.
5. **TUI descoberta** — tela `/config` (6 inputs + validação), status bar, `/help`, sugestões.
6. **Verificação final** — `go build ./... && go vet ./... && gofmt -l . && go test ./...` + checagem das métricas do PRD via transcript.

### Dependências Técnicas

Nenhuma bloqueante. Sem serviços externos novos; o mock htttest existente cobre Jev e gateway.

## Monitoramento e Observabilidade

### Error Tracking

CLI local, sem Sentry. Erros de startup (modelo inválido) → stderr + exit 1, apontando o valor rejeitado. Erros de comando na TUI → `errorBox` (padrão existente). Erros de roteamento com Jev on → cadeia de fallback atual preservada.

### Logging Estruturado

O transcript JSONL de sessão já registra a `Decision` de cada rota (`Model`, `Router`, `Reason`) — é a fonte de verdade para as métricas do PRD. `FixedRouter` usa `Router: "fixed"` e o pin `Router: "pin"`, mantendo o transcript auditável. Nenhum dado sensível novo (nomes de modelo e bool não são PII; api_key nunca vai ao transcript).

### Health Checks

`--doctor` é o health check do CLI: passa a reportar `jev enabled`, o modelo fixo efetivo e — com Jev desabilitado — o estado sem probe do endpoint.

### Métricas de Negócio

Telemetria de TPS (`internal/telemetry`) continua medindo por modelo independentemente do roteador (fora de escopo mudar). As métricas de sucesso do PRD são verificáveis por: zero requisições ao endpoint Typesafe (assert em testes de integração), 100% das rotas do transcript com o modelo fixo (inspeção do JSONL), persistência entre restarts (re-`Load` da config).

### Alertas

Não aplicável (CLI local sem canal de notificação).

## Considerações Técnicas

### Decisões Principais

1. **FixedRouter + pin** (confirmado em clarificação): `cfg.Model` alimenta o pin do agente principal (semântica atual de "modelo fixo vence com Jev on") e o `FixedRouter` cobre o caso sem modelo e os subagentes. Alternativa rejeitada: tudo via FixedRouter — criaria duas fontes de verdade para o mesmo conceito.
2. **`router.Wire()` centraliza o wiring** (confirmado): elimina a duplicação main/TUI que a feature ampliaria. Alternativa rejeitada: novo pacote `internal/wiring` — peso desnecessário.
3. **`Enabled *bool` + `JevEnabled()`** (confirmado): tri-state idiomático em Go/TOML para default true retro-compatível. Alternativa rejeitada: unmarshal duplo com map de presença — mesmo efeito, mais código.
4. **Doctor sem probe quando desabilitado** (confirmado): coerente com "zero chamadas ao Jev".
5. **TUI deriva estado de `m.cfg`** (confirmado): env overrides já estão dobrados no `Load()`; sem nova superfície no agent.
6. **Propagação de pin para subagentes rejeitada**: mudaria a semântica do squad mode (pins de persona); `FixedRouter` resolve o requisito sem tocar nisso.

### Riscos Conhecidos

- **Marshal de `*bool` nil**: `Save()` deve normalizar nil → true antes do marshal — coberto por teste de round-trip.
- **Env override × toggle em runtime**: `KTERMINAL_TYPESAFE_ENABLED=false` + `/jev on` na sessão → vale para a sessão (cfg em memória), mas o env vence no próximo `Load` (padrão existente de prevalência, restrição do PRD). Documentar no `/help` não é necessário; comportamento é o dos knobs existentes.
- **Degradação de visão**: modelo fixo não-vision com anexos de imagem roteia para default/primeiro candidato vision — determinístico, mas pode surpreender; o `Reason` no transcript (`"fixed: model not vision-capable"`) torna o evento rastreável.
- **Mudança de comportamento para usuários sem chave**: heurística por keywords deixa de ser roteador principal — intencional (REQ-002), mas vale nota de release.

### Conformidade com Skills Padrões

As rules de `.agents/rules/` são orientadas ao ecossistema kspec TS/Java; para o código Go prevalecem os padrões do repo (AGENTS.md): sem comentários no código, eventos do agente via canal, receivers por valor na TUI, `strings.Builder` por ponteiro, validação nos pontos de wiring. As rules de testes TS (TestSprite) não se aplicam — os testes seguem o padrão `go test` + htttest do código-base.

### Arquivos relevantes e dependentes

- `internal/config/config.go` — campos `Enabled *bool`, `Model`, accessors, env overrides, `Save()` (modificado)
- `internal/config/config_test.go` — defaults, env, round-trip (modificado)
- `internal/router/router.go` — `FixedRouter` (novo componente no arquivo existente)
- `internal/router/` testes de `FixedRouter`/`Wire` (novo)
- `main.go` — wiring via `Wire()`, validação de startup, `runDoctor` (modificado)
- `main_test.go` — startup inválido, doctor sem probe (modificado)
- `internal/tui/tui.go` — `rebuildAgent()`, `/model`, `saveConfig()`, `newConfigInputs()`, `hintBar()`, `/help` (modificado)
- `internal/tui/commands.go` — `/jev`, picker de modelos, `nativeCommands` (modificado)
- `internal/tui/commands_test.go`, `internal/tui/tui_test.go` — testes de comando, picker, status bar (modificados)
- `internal/agent/agent.go` — **sem mudanças** (`SetPinned`, `SetRouters`, `decide()`, `RunSyncPersona` consumidos como estão)
- `internal/catalog/catalog.go` — sem mudanças (`Get`, `DefaultModel`, `Available`)
- `internal/jev/jev.go` — sem mudanças (restrição do PRD)
- `spec/tasks/015-prd-controle-do-jev/prd.md` — fonte dos requisitos
