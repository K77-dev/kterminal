# Tarefa 2.0: Capacidade de visão no catálogo

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Catálogo com capacidade de visão

## Dependências

- Nenhuma (independente da task 1.0)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

O catálogo aprende qual modelo enxerga: `models.yaml` ganha `vision: true|false` por modelo (`glm-5.3`: true; demais: false até confirmação) e `catalog.Model` ganha o campo `Vision bool` com tag YAML. O default honesto é `false` — errar para false degrada para texto (seguro); errar para true quebraria chamadas (decisão 5 da techspec). Tarefa pequena e isolada — pode rodar em paralelo com a 1.0.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib + YAML existente.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa`, `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; parse YAML aditivo (campos existentes intocados).
</skills>

<requirements>
- `models.yaml`: `vision` por modelo — `glm-5.3: true`; demais `false` (default honesto).
- `Model.Vision bool` com tag `yaml:"vision"`.
- O catálogo expõe a capacidade de visão por modelo, vinda do YAML.
- Nenhum outro campo do YAML muda.
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `vision` por modelo no `models.yaml` (`glm-5.3`: true; demais: false)
- [ ] 2.2 Adicionar `Vision bool` ao `catalog.Model` com tag YAML
- [ ] 2.3 Escrever `TestVisionParsedFromYAML` (ver Testes da Tarefa)

## Detalhes de Implementação

- Schema YAML e struct: techspec, seção **Design de Implementação → Modelos de Dados**.
- Decisão "`vision: false` como default honesto": techspec, seção **Considerações Técnicas → Decisões Principais** (item 5).

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: `glm-5.3` → `Vision: true`; demais modelos → false.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/catalog`):
  - `TestVisionParsedFromYAML` — `glm-5.3` → `Vision: true`; demais → false (techspec, item 4).
- [ ] Testes de integração — fora do escopo da task; a filtragem por visão é validada na task 4.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/catalog/models.yaml` — `vision` por modelo (glm-5.3: true)
- `internal/catalog/catalog.go` — `Model.Vision`
- `internal/catalog/catalog_test.go` — parse de visão
