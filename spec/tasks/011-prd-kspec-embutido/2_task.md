# Tarefa 2.0: Resolução projeto-first com templates inlined, Rules, Version e Source

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-002 — Loader com precedência projeto-first

## Dependências

- 1.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Completar o loader `internal/kspec` com a resolução projeto-first: para cada componente (skill, template, rule), `.agents/` do diretório corrente vence o conteúdo embutido. `Resolve(name)` devolve o SKILL.md com os templates `@.agents/templates/*.md` referenciados inlined. `Rules()`, `Version()` e `Source()` fecham a API do Store.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md`, `tests.md`.
</skills>

<requirements>
- Resolução por componente: `.agents/skills/<name>/SKILL.md`, `.agents/templates/*.md`, `.agents/rules/*.md` do cwd vencem os embutidos
- Inlining de templates referenciados no SKILL.md, cada um resolvido projeto-first
- `Source()` retorna "project" quando existe `.agents/skills/kspec-*/SKILL.md` no cwd, senão "embedded"
- `Version()` devolve o frontmatter da skill `kspec-version` do projeto (fonte project) ou o `VERSION` embutido (fonte embedded)
</requirements>

## Subtarefas

- [ ] 2.1 Implementar `Resolve(name string) (string, error)`: lê SKILL.md do projeto primeiro, fallback embed; detecta referências `@.agents/templates/*.md` no conteúdo e substitui cada uma pelo template resolvido (projeto-first), inline
- [ ] 2.2 Implementar `Rules() []Rule`: rules do projeto quando existirem, senão as 13 embutidas
- [ ] 2.3 Implementar `Version() string` e `Source() string` conforme requisitos acima
- [ ] 2.4 Cobrir com testes os dois modos (projeto com/sem `.agents/`) usando temp dirs

## Detalhes de Implementação

Seguir a techspec, seção "Interfaces Principais" (assinaturas do Store) e "Decisões Principais" (item 9 — o inlining não edita o conteúdo vendored, apenas o conteúdo resolvido devolvido por `Resolve`). O parser de frontmatter e `List()` da task 1.0 permanecem válidos: `List()` também deve refletir projeto-first nesta task (skills do projeto aparecem na lista quando existirem).

## Critérios de Sucesso

- Em projeto sem `.agents/`, `Resolve`/`Rules` resolvem o conteúdo embutido
- Em projeto com `.agents/`, as cópias do projeto prevalecem em skills, templates e rules
- `Resolve("kspec-techspec")` retorna conteúdo com o template techspec inlined
- `Source()`/`Version()` corretos nos dois modos

## Testes da Tarefa

- [ ] Testes de unidade
  - Precedência: temp dir com `.agents/` customizado vence embed; sem `.agents/`, embed resolve
  - Inlining: template referenciado aparece no conteúdo resolvido; template inexistente não quebra (referência mantida ou erro claro)
  - `Version`/`Source` nos dois modos
- [ ] Testes de integração
  - Fluxo completo sobre temp dir: `List` → `Resolve` → conteúdo contém frontmatter removido + templates inlined
- [ ] Testes E2E (se aplicável)
  - N/A — sem superfície de usuário ainda

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/kspec/kspec.go`
- `internal/kspec/kspec_test.go`
- `internal/kspec/embed/**` (conteúdo da task 1.0)
