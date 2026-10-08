# Relatório de Code Review — Task 2.0: Campos calculados e `Criteria()` alternável no catálogo

## Resumo

- **Data**: 2026-10-04
- **Branch**: `002-010-prds-kterminal`
- **Status**: **APROVADO**
- **Arquivos Modificados**: 2 (`internal/catalog/catalog.go` modificado; `internal/catalog/catalog_test.go` novo)
- **Linhas Adicionadas**: 73 (8 em `catalog.go` + 65 em `catalog_test.go`)
- **Linhas Removidas**: 2
- **Reviewer**: kspec-review-runner (auto-review do task-runner)

## Veredito

**APROVADO** — a implementação atende integralmente a Task 2.0 e a REQ-003 (parcela do catálogo): `catalog.Model` ganha `MeasuredTPS float64` e `MeasuredSamples int` sem tags YAML, e `Criteria()` alterna o texto de speed conforme `MeasuredTPS > 0`, exatamente como a Tech Spec prescreve. O caminho estimate é **byte-idêntico** à saída anterior (provado programaticamente), o `models.yaml` não foi tocado, `catalog` não importa `telemetry` e o contrato com o Jev (um string por candidato) permanece inalterado. Os testes 8-9 da techspec existem e provam o que a task exige; um terceiro teste prova o critério de sucesso "parse do `models.yaml` permanece idêntico". 117/117 testes passando, todos os checks limpos.

## Interpretação Documentada

O prompt da task mencionou "catálogo de modelos (internal/llm)", porém o `2_task.md`, a Tech Spec (visão de componentes, modelos de dados e tabela de arquivos) e o código existente apontam unanimemente para `internal/catalog` — onde `Model` e `Criteria()` de fato vivem. Task e Tech Spec concordam entre si (sem conflito que exigisse abort); a implementação seguiu os arquivos de spec. O pacote `internal/llm` não contém o catálogo e não foi tocado.

## Conformidade com Rules

| Rule | Status | Observações |
|------|--------|-------------|
| Sem comentários no código (padrão do projeto) | OK | Nenhum comentário adicionado; único `//` do arquivo é o directive pré-existente `//go:embed` |
| Sem dependências novas | OK | `go list -deps ./internal/catalog`: apenas stdlib + `gopkg.in/yaml.v3` (já existente); zero ocorrências de `telemetry` |
| Direção de dependência: `catalog` não importa `telemetry` | OK | `go list -deps ./internal/catalog \| grep -c telemetry` = 0 — injeção permanece pull no agent (task 3.0) |
| Formatação/lint | OK | `gofmt -l .` sem output; `go vet ./...` sem achados |
| Nomenclatura Go | OK | `MeasuredTPS`/`MeasuredSamples` exatamente como na techspec; exported por serem lidos pelo agent |
| Padrões de teste do repositório (`testing` puro, `t.Fatalf`, sem framework) | OK | Mesmo estilo de `internal/telemetry/telemetry_test.go` |
| Tratamento de erro | N/A | Nenhum novo caminho de erro (campos calculados + formatação) |
| Logging | N/A | Techspec: sem logs novos |
| Rules DDD/TS/Vitest | N/A | Brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec |

## Verificação de Segurança

| Item | Status | Observações |
|------|--------|-------------|
| Sem secrets/PII | OK | Campos carregam apenas números em memória; nada persistido por este pacote |
| Endpoints/CORS/SQL/auth/headers | N/A | Pacote local sem rede nem backend |
| Inputs malformados | OK | Parse do YAML inalterado (`yaml.Unmarshal` com os mesmos campos taggeados); campos sem tags não participam do parse do `models.yaml` real |

## Aderência à TechSpec

| Decisão Técnica (techspec) | Implementado | Evidência |
|-----------------|--------------|-------------|
| `Model` ganha `MeasuredTPS float64`, `MeasuredSamples int` **sem tags YAML** | SIM | `catalog.go:23-24` — sem tags YAML nem JSON, idêntico ao bloco Modelos de Dados |
| Campos calculados nunca parseados do `models.yaml`; populados em memória pelo agent | SIM | `models.yaml` sem mudança (git status limpo na pasta); `TestParseIgnoresMeasuredFields` prova zero-values pós-`Load()` para todos os modelos |
| `Criteria()`: `MeasuredTPS > 0` → `Speed: ~%.0f tok/s (measured over %d calls)` | SIM | `catalog.go:93-95`; saída real verificada: `Speed: ~92 tok/s (measured over 7 calls).` com `MeasuredTPS=92.4, MeasuredSamples=7` |
| Senão, o texto atual do estimate (`~%.0f tok/s`) | SIM | `catalog.go:92`; **byte-identidade programaticamente provada**: saída antiga (format string original) == `Criteria()` novo para glm-5.2 → `true` |
| `models.yaml` — sem mudança, `tps_estimate` permanece como fallback | SIM | Arquivo intocado; `TestCriteriaUsesEstimateByDefault` prova o fallback com estimate 70 |
| `catalog` não importa `telemetry` (injeção é pull no agent) | SIM | `go list -deps`: 0 dependências de telemetry |
| Contrato com o Jev inalterado (`Criteria()` continua um string por candidato) | SIM | Assinatura `func (m Model) Criteria() string` intacta; `router.go:45` (`criteria[m.Name] = m.Criteria()`) não modificado e continua compilando sem mudança |
| Fallback "nunca piora a informação do Jev" (PRD, não negociável) | SIM | `MeasuredTPS = 0` (default pós-parse e abaixo do limiar de `Store.Get`) → texto estimate idêntico ao de antes da feature |

## Tasks Verificadas

| Task | Status | Observações |
|------|--------|-------------|
| 2.1 Adicionar `MeasuredTPS`/`MeasuredSamples` ao `catalog.Model` (sem tags YAML) | COMPLETA | `catalog.go:23-24` |
| 2.2 Alternar o texto de speed no `Criteria()` conforme `MeasuredTPS > 0` | COMPLETA | `catalog.go:92-95` |
| 2.3 Escrever os testes 8-9 da techspec | COMPLETA | `catalog_test.go` — 2 mandatórios + 1 para critério de sucesso explícito |

## Testes

- **Total de Testes**: 117 (114 pré-existentes + 3 novos)
- **Passando**: 117
- **Falhando**: 0
- **Coverage**: `internal/catalog` 57.6% (antes: 0% — pacote não tinha testes); ambos os branches do `Criteria()` alternável exercitados; restante = caminhos de erro de parse e `Available` pré-existentes, fora do escopo da task

### Testes mandatórios (techspec, itens 8-9) — ambos PASS

| Teste | O que prova |
|-------|-------------|
| `TestCriteriaUsesEstimateByDefault` | Carrega o `models.yaml` real (embedded), pega glm-5.2 (estimate 70) → texto contém `~70 tok/s` e **não** contém "measured"; valida também o estimate esperado (70) antes de formatar |
| `TestCriteriaUsesMeasuredWhenAvailable` | Cópia por valor do modelo do catálogo (mesmo mecanismo da injeção da task 3.0) + `MeasuredTPS: 92.4, MeasuredSamples: 7` → texto contém `measured over 7 calls` e `~92 tok/s`, e **não** cita o estimate `~70 tok/s` |

### Teste adicional (critério de sucesso explícito da task)

| Teste | Requisito que cobre |
|-------|---------------------|
| `TestParseIgnoresMeasuredFields` | "O parse do `models.yaml` permanece idêntico (campos calculados ignorados)" — percorre **todos** os modelos carregados e exige `MeasuredTPS == 0 && MeasuredSamples == 0` pós-`Load()` |

### Verificação obrigatória executada

```
go build ./...            OK
go vet ./...              OK (sem achados)
gofmt -l .                OK (sem output)
go test ./... -count=1    ok kterminal, internal/agent, internal/catalog, internal/llm,
                          internal/session, internal/telemetry, internal/tools, internal/tui
                          — 117 PASS, 0 FAIL
go test -race -count=1 ./internal/catalog/ ./internal/telemetry/   OK (sem data races)
```

Os 114 testes pré-existentes continuam passando (nada quebrou).

## Problemas Encontrados

| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| Baixa | `catalog.go` | 23-24 | yaml.v3 casa campos sem tag pelo nome lowercase — um hipotético `measuredtps:` no YAML popularia o campo | Inalcançável na prática: `models.yaml` é embedded, não contém essas chaves e a task veta mudanças nele. Spec prescreve literalmente "sem tags YAML" (3×); adicionar `yaml:"-"` desviaria da spec. Manter como está; task 3.0 deve popular os campos apenas em memória |
| Baixa | `catalog.go` | 93-94 | `MeasuredTPS > 0` com `MeasuredSamples == 0` renderizaria "measured over 0 calls" | Estado inconsistente inalcançável pelo fluxo desenhado: `Store.Get` retorna `(0, n)` abaixo do limiar e `(ewma, n ≥ 5)` acima; a injeção da techspec (task 3.0) guarda `if tp > 0` e atribui ambos juntos. Regra literal `MeasuredTPS > 0` da spec mantida |
| Baixa | `catalog.go` | 23-24 | Campos sem tags JSON — se `Model` fosse marshalado, serializaria como `MeasuredTPS`/`MeasuredSamples` | Nenhum código marshaliza `Model` hoje (grep: uso apenas como valor em router/agent e via `Criteria()` string); bloco Modelos de Dados da techspec os mostra sem tags. Sem ação |

## Pontos Positivos

- Mudança cirúrgica: 8 linhas adicionadas / 2 removidas em um único arquivo de produção; nenhum comportamento existente alterado.
- Caminho estimate **byte-idêntico** ao anterior — provado programaticamente (comparação da saída da format string antiga com o `Criteria()` novo), não apenas por inspeção: o requisito "nunca piorar a informação do Jev" fica demonstrado no caminho de fallback.
- Testes usam o `models.yaml` real via `Load()` (não fixtures sintéticas), provando o parse de produção; o teste measured usa cópia por valor do modelo do catálogo, exercitando o mesmo mecanismo de injeção que a task 3.0 empregará.
- Testes negativos fortes: ausência de "measured" no fallback e ausência do estimate quando há medido — provam a alternância nos dois sentidos.
- Escopo respeitado: nada da task 3.0 (injeção no agent) ou 4.0 (doctor) antecipado; `telemetry` intocado.

## Recomendações

1. **Task 3.0**: ao injetar, seguir o snippet da techspec (`if tp, n := a.Telemetry.Get(m.Name); tp > 0 { m.MeasuredTPS, m.MeasuredSamples = tp, n }`) — atribui os dois campos juntos, mantendo o estado consistente que o `Criteria()` assume.
2. **Task 3.0**: preservar o guard `Usage.CompletionTokens > 0` antes de `Record` (recomendação herdada da review 1.0).
3. Se um dia `Model` passar a ser serializado em JSON, decidir tags para os campos calculados (hoje não há serializador).

## Conclusão

A Task 2.0 entrega exatamente o que a Tech Spec desenha para o catálogo: campos calculados `MeasuredTPS`/`MeasuredSamples` sem tags (nunca lidos do YAML) e um `Criteria()` que alterna o texto de speed com TPS medido e contagem de chamadas quando há confiança, caindo no estimate byte-idêntico caso contrário — o fallback "nunca piora" do PRD preservado e provado. Dependências limpas (`catalog` folha quanto a `telemetry`), contrato do Jev intacto, `models.yaml` intocado. 117/117 testes, build/vet/gofmt/`-race` limpos. **APROVADO** — base pronta para a task 3.0 (coleta pós-chamada e injeção no Agent).
