# Tarefa 1.0: Pacote `internal/squad` — loader de personas, assets embutidos e validação de kickoff

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Maestro prompt-driven
- REQ-003 — Catálogo de personas

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Criar o pacote `internal/squad` como fundação do squad mode. Este pacote contém: (1) o loader de personas com `go:embed` de 7 personas default + override de `.agents/agents/<nome>/AGENT.md`, (2) o prompt de maestro embutido, (3) os tipos `Persona`, `Kickoff`, `Budget` e a validação de tetos. O pacote espelha o padrão de `internal/kspec` (embed + frontmatter + override de projeto) sem acoplar os dois domínios.

## Conformidade com Skills Padrões

- Go 1.27, módulo único `kterminal`, binário auto-contido
- Assets embutidos via `go:embed` (precedentes: `models.yaml`, `markdown.json`, `internal/kspec/embed`)
- Código em inglês, sem comentários
- Frontmatter parseado com `splitFrontmatter` existente (delimitadores estritos)
- Nomes de persona validados por `validName` (sem path traversal) — reusar a lógica de `kspec.validName`

## Requisitos

- 7 personas default embarcadas no binário via `go:embed`: architect, backend, frontend, database, ux, qa, test
- Prompt de maestro embarcado via `go:embed` (`internal/squad/embed/maestro.md`)
- Frontmatter estruturado parseado: `name`, `discipline`, `model-tags`, `rules`
- Persona do projeto (`.agents/agents/<nome>/AGENT.md`) com mesmo nome sobrescreve a embutida
- Persona do projeto inédita é utilizável sem configuração adicional
- Tipo `Kickoff` com validação: papéis existem no catálogo de personas, tetos clampados aos máximos do config
- `MaestroPrompt()` retorna o conteúdo do asset embutido
- Nenhum dependência de outros pacotes internos (exceto padrões da stdlib e yaml.v3)

## Subtarefas

- [ ] 1.1 Criar estrutura de diretórios `internal/squad/embed/personas/{architect,backend,frontend,database,ux,qa,test}/AGENT.md` com frontmatter e prompt de cada persona
- [ ] 1.2 Criar `internal/squad/embed/maestro.md` com o prompt do maestro (instruções para convocar personas via `task`, sintetizar contribuições, declarar convergência)
- [ ] 1.3 Implementar `internal/squad/frontmatter.go` — parse de frontmatter YAML (name, discipline, model-tags, rules) reusing o padrão de `kspec.splitFrontmatter`
- [ ] 1.4 Implementar `internal/squad/squad.go` — tipos `Persona`, `Store`, `Load()`, `List()`, `Resolve(name)`, `MaestroPrompt()`, override de projeto sobre embutida, persona inédita de projeto
- [ ] 1.5 Implementar tipo `Kickoff` e função de validação (`ValidateKickoff`) — papéis existem no catálogo, `max_convocations` e `token_budget` clampados aos máximos do config
- [ ] 1.6 Implementar `validName` para nomes de persona (sem path traversal), espelhando `kspec.validName`

## Detalhes de Implementação

Consulte a seção "Design de Implementação → Interfaces Principais" e "Modelos de Dados" da `techspec.md` para as assinaturas exatas:

- `Persona` struct: `Name`, `Discipline`, `ModelTags []string`, `Rules []string`, `Prompt string`
- `Store` struct: `Load() *Store`, `List() []Persona`, `Resolve(name string) (Persona, error)`, `MaestroPrompt() string`
- `Kickoff` struct: `Roles []string`, `MaxConvocations int`, `TokenBudget int64`, `ExitCriterion string`

Frontmatter da persona (`.agents/agents/<nome>/AGENT.md` e assets embutidos):

```yaml
---
name: architect
discipline: architecture
model-tags: [reasoning]
rules: [go, code-standards]
---
<prompt do papel>
```

O loader segue o mesmo padrão de `kspec.Store`: `go:embed embed`, `fs.Sub(embedded, "embed")`, override de projeto lendo `.agents/agents/<nome>/AGENT.md` do CWD.

## Critérios de Sucesso

- `squad.Load()` retorna 7 personas com frontmatter parseado corretamente
- `Resolve("architect")` retorna a persona com `Name: "architect"`, `Discipline: "architecture"`, `ModelTags: ["reasoning"]`
- Persona de projeto com mesmo nome sobrescreve a embutida
- Persona de projeto inédita aparece em `List()`
- `MaestroPrompt()` retorna string não vazia
- `ValidateKickoff` rejeita papéis inexistentes e clampa tetos acima do máximo

## Testes da Tarefa

- [ ] Testes de unidade — `internal/squad/squad_test.go`:
  - Parse de frontmatter (name/discipline/model-tags/rules) para cada uma das 7 personas
  - Override de persona de projeto sobre embutida (mesmo nome)
  - Persona inédita de projeto aparece em `List()`
  - `MaestroPrompt()` não vazio
  - `Resolve` de persona inexistente retorna erro
  - `validName` rejeita path traversal (`../`, `/`, `\`)
  - `ValidateKickoff`: papéis inexistentes rejeitados, tetos clampados, valores ausentes caem em defaults
- [ ] Testes de integração — não aplicável (pacote de fundação sem dependências externas)

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/squad/squad.go` (novo)
- `internal/squad/frontmatter.go` (novo)
- `internal/squad/squad_test.go` (novo)
- `internal/squad/embed/maestro.md` (novo)
- `internal/squad/embed/personas/architect/AGENT.md` (novo)
- `internal/squad/embed/personas/backend/AGENT.md` (novo)
- `internal/squad/embed/personas/frontend/AGENT.md` (novo)
- `internal/squad/embed/personas/database/AGENT.md` (novo)
- `internal/squad/embed/personas/ux/AGENT.md` (novo)
- `internal/squad/embed/personas/qa/AGENT.md` (novo)
- `internal/squad/embed/personas/test/AGENT.md` (novo)
- Referência: `internal/kspec/kspec.go` (padrão de loader com embed + override)
- Referência: `internal/kspec/frontmatter.go` (padrão de parse de frontmatter)
