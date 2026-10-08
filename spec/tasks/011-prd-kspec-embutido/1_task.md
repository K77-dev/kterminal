# Tarefa 1.0: Vendoring e loader base do kspec

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Conteúdo kspec embutido (vendoring)
- REQ-002 — Loader com precedência projeto-first (parcial: embed, frontmatter e List)

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Criar a fundação do kspec embutido: o script de sincronização com o upstream, a árvore de conteúdo vendored em `internal/kspec/embed/` e o pacote Go `internal/kspec` que expõe esse conteúdo via `go:embed`, parseia o frontmatter das skills e lista as 10 skills `kspec-*`. Esta task não muda nenhum comportamento existente do kterminal — é fundação inerte.

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md` (sem comentários no código, padrões Go do código-base), `tests.md` (cobertura do pacote novo).
</skills>

<requirements>
- O script `scripts/sync-kspec.sh` é a ÚNICA via de entrada de conteúdo em `internal/kspec/embed/`
- Skills de terceiros (não `kspec-*`) e `skills-lock.json` ficam fora da árvore vendored
- Conteúdo vendored: `skills/kspec-*/SKILL.md`, `templates/*.md`, `rules/*.md`, `VERSION`
- Frontmatter parseado: `name`, `version`, `description`, `argument-hint`
</requirements>

## Subtarefas

- [ ] 1.1 Criar `scripts/sync-kspec.sh`: clona `K77-dev/kspec` (ref configurável via arg/env para reprodutibilidade), copia `.agents/skills/kspec-*/`, `.agents/templates/`, `.agents/rules/` e `VERSION` para `internal/kspec/embed/`, excluindo skills de terceiros e `skills-lock.json`; documentar no cabeçalho do script que edições de conteúdo acontecem no repo kspec
- [ ] 1.2 Executar o script e commitar a árvore vendored resultante
- [ ] 1.3 Criar o pacote `internal/kspec`: `//go:embed embed` como `embed.FS`, tipo `Skill{Name, Version, Description, ArgHint}`, parser de frontmatter YAML delimitado por `---` (usar `gopkg.in/yaml.v3`, já é dependência — ver `internal/catalog/catalog.go:11` como precedente de embed)
- [ ] 1.4 Implementar `Load() *Store` e `List() []Skill` sobre o conteúdo embutido (não há resolução projeto-first nesta task — apenas o conteúdo embutido)

## Detalhes de Implementação

Seguir a techspec, seções "Visão Geral dos Componentes" e "Interfaces Principais". O layout da árvore embed e o formato do frontmatter estão especificados lá. Nesta task o `Store` ainda não resolve projeto-first nem inlina templates — isso é a task 2.0.

## Critérios de Sucesso

- `go build ./...` compila com a árvore embutida no binário
- `List()` retorna exatamente as 10 skills `kspec-*` com name/version/description parseados
- Rodar `scripts/sync-kspec.sh` duas vezes produz árvore idêntica (verificável via `git status` limpo)
- Nenhum arquivo de skill de terceiros na árvore embed

## Testes da Tarefa

- [ ] Testes de unidade
  - Parse de frontmatter: caso válido completo, sem frontmatter (erro), campos opcionais ausentes (`argument-hint`)
  - `List()` retorna as 10 skills esperadas com campos preenchidos
  - `Version()` lê o `VERSION` embutido
- [ ] Testes de integração
  - Executar o sync script em cópia temporária e verificar layout resultante (skills, templates, rules, VERSION presentes; terceiros ausentes)
- [ ] Testes E2E (se aplicável)
  - N/A — fundação sem superfície de usuário

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `scripts/sync-kspec.sh` (novo)
- `internal/kspec/kspec.go` (novo)
- `internal/kspec/frontmatter.go` (novo)
- `internal/kspec/kspec_test.go` (novo)
- `internal/kspec/embed/**` (novo — conteúdo vendored)
- `internal/catalog/catalog.go` (referência — precedente de `go:embed`)
