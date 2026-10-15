# Tarefa 2.0: Router — `FixedRouter` determinístico e factory `Wire()`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Modelo fixo determinístico com Jev desabilitado (roteador que devolve sempre o mesmo modelo; `default_model` do catálogo como fallback)

## Dependências

- 1.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Criar em `internal/router` (router.go) o `FixedRouter` — roteador determinístico que devolve sempre o mesmo modelo — e a factory `Wire()`, única fonte de wiring do par (primary, fallback) a partir de (api_key, enabled, model, catálogo). `Wire()` substituirá o bloco duplicado entre main.go:82-90 e `rebuildAgent()` (tui.go:1261-1281) — a substituição em si acontece nas tasks 3.0 e 4.0; aqui entram apenas o tipo novo e a factory, com testes de unidade. `JevRouter` e `HeuristicRouter` não mudam; a heurística permanece só como fallback de erro com Jev habilitado.

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, código em inglês. `router` já importa `jev` e `catalog` — `Wire()` não cria ciclos (verificado no grafo Graphify, arestas EXTRACTED). Assinaturas seguem a interface `Router` existente (router.go:21).
</skills>

<requirements>
- Structs e assinaturas exatamente como na seção "Interfaces Principais" da techspec.md: `FixedRouter{Model, Default string}` e `func Wire(apiKey string, jevEnabled bool, fixedModel string, cat *catalog.Catalog) (router.Router, router.Router)`
- Ordem de decisão do `FixedRouter.Route()`: `Model` se presente nos candidatos → `Default` se presente → `candidates[0]`; Confiança 1, `Router: "fixed"`; sem candidatos → erro (contrato da interface)
- `Wire()` com Jev ativo (`jevEnabled && apiKey != ""`): primary `JevRouter`, fallback `HeuristicRouter{Default: cat.DefaultModel}` (comportamento atual)
- `Wire()` com Jev inativo: primary `FixedRouter{Model: fixedModel, Default: cat.DefaultModel}`, fallback `nil`
- `Wire()` nunca cria `JevRouter` com Jev desabilitado — garante zero chamadas ao endpoint Typesafe em toda a sessão, incluindo subagentes
- `Reason` do `FixedRouter` rastreável no transcript (ex: `"fixed: model not vision-capable"` na degradação por visão — ver "Riscos Conhecidos" da techspec)
</requirements>

## Subtarefas

- [x] 2.1 Implementar `FixedRouter` em `internal/router/router.go` com a ordem de decisão Model → Default → primeiro candidato e `Router: "fixed"`
- [x] 2.2 Implementar `Wire()` no mesmo pacote, decidindo o par (primary, fallback) pelas regras da techspec
- [x] 2.3 Testes de unidade do `FixedRouter`: modelo nos candidatos; degradação em cadeia com filtro de visão; sem candidatos → erro
- [x] 2.4 Testes de unidade do `Wire()`: chave+enabled → (Jev, Heuristic); enabled=false → (Fixed, nil); sem chave → (Fixed, nil); `fixedModel` vazio → Fixed no default do catálogo

## Detalhes de Implementação

Consulte techspec.md — seções "Interfaces Principais" (blocos `FixedRouter` e `Wire()`), "Arquitetura do Sistema" (componente `internal/router`) e "Pontos de Integração" (degradação determinística quando o modelo fixo é excluído dos candidatos por visão — o pin é pulado em agent.go:1041 e a mesma cadeia se aplica).

## Critérios de Sucesso

- `go test ./internal/router/` verde
- `FixedRouter` devolve o mesmo `Decision.Model` para qualquer `state` com o mesmo conjunto de candidatos (determinismo)
- `Wire()` com `jevEnabled=false` nunca retorna um `*JevRouter` (asserção de tipo no teste)
- Degradação Model → Default → `candidates[0]` respeita o filtro de visão dos candidatos de forma determinística

## Testes da Tarefa

- [x] Testes de unidade (`internal/router/`):
  - [x] `FixedRouter` com `Model` presente nos candidatos → escolhe `Model`, Confiança 1, `Router: "fixed"`
  - [x] `FixedRouter` com `Model` ausente → degrada para `Default`; `Default` ausente → `candidates[0]`
  - [x] `FixedRouter` com candidatos filtrados por visão (modelo fixo não-vision excluído) → cadeia determinística preservada
  - [x] `FixedRouter` sem candidatos → erro
  - [x] `Wire()`: (chave, enabled=true) → (Jev, Heuristic); (chave, enabled=false) → (Fixed, nil); (sem chave, qualquer enabled) → (Fixed, nil); `fixedModel=""` → `FixedRouter.Model` efetivo = default do catálogo
- [ ] Testes de integração: N/A (o turno completo com mock httptest do Jev acontece na task 3.0/7.0)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/router/router.go` (modificado — `FixedRouter` e `Wire` como componentes novos no arquivo existente)
- `internal/router/` testes de `FixedRouter`/`Wire` (novo)
- `internal/catalog/catalog.go` (referência — `DefaultModel`, `Available` consumidos como estão)
- `spec/tasks/015-prd-controle-do-jev/techspec.md` (referência)
