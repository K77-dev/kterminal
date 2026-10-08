# Tarefa 6.0: Tool kspec_bootstrap com materialização da árvore kspec

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-007 — `/kspec-bootstrap` interoperável

## Dependências

- 2.0, 4.0

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Tool Go nativa `kspec_bootstrap` que materializa a estrutura kspec padrão no projeto corrente a partir do conteúdo embutido: `.agents/` (skills `kspec-*`, rules, templates), `spec/tasks/` e `VERSION`. Escrita determinística, sem custo de tokens — o agente permanece responsável pela análise do projeto, pela escolha de plataformas via `ask_user` e pela geração dos `*.bootstrap.md` a partir dos templates. Pós-bootstrap, as cópias do projeto passam à frente das embutidas (resolução projeto-first da task 2.0).

<skills>
### Conformidade com Skills Padrões

CLAUDE.md não possui tabela "Stack e skills recomendadas". Rules aplicáveis: `code-standards.md` (tools registradas no construtor do Registry), `tests.md`.
</skills>

<requirements>
- `Store.Bootstrap(dir string, force bool) error` materializa a árvore a partir do embed
- Recusa sobrescrever `.agents/` existente sem `force: true` (proteção de customizações locais)
- Tool `kspec_bootstrap` registrada no construtor do `Registry` (importa `internal/kspec`; não precisa de estado do Agent), `Mutating: true`, arg `force`
- A estrutura gerada é um projeto kspec padrão, reconhecível por Claude Code/Cursor/Codex
</requirements>

## Subtarefas

- [ ] 6.1 Implementar `Store.Bootstrap(dir, force)` em `internal/kspec/bootstrap.go`: cópia de `skills/kspec-*`, `templates/`, `rules/`, `VERSION` para `.agents/` + `spec/tasks/`, com guard de overwrite
- [ ] 6.2 Implementar a tool `kspec_bootstrap` em `internal/tools/bootstrap.go` e registrá-la no construtor de `NewRegistry`
- [ ] 6.3 Verificar a interoperabilidade: projeto bootstrapped expõe skills visíveis ao Claude Code (`.agents/skills/*/SKILL.md` com frontmatter válido)

## Detalhes de Implementação

Seguir a techspec, seção "Decisões Principais" (item 3 — divisão de responsabilidades entre tool nativa e agente) e "Verificações Técnicas → Segurança" (guard de overwrite e flag mutating). A tool escreve caminhos relativos ao cwd, como as tools de fs existentes.

## Critérios de Sucesso

- `kspec_bootstrap` materializa a árvore completa em um diretório vazio
- Chamada em projeto com `.agents/` existente sem `force` → erro claro; com `force` → sobrescreve
- Após bootstrap, `Source()` retorna "project" e `Resolve` usa as cópias do projeto
- Tool sujeita ao confirm de `--confirm` (mutante)

## Testes da Tarefa

- [ ] Testes de unidade
  - `Bootstrap` em dir vazio: árvore completa (skills, rules, templates, VERSION, spec/tasks)
  - Guard: dir com `.agents/` existente → erro sem `force`, sucesso com `force`
  - Contrato da tool: args, Mutating, output
- [ ] Testes de integração
  - Bootstrap em temp dir → `Source()` = "project" e `Resolve` retorna conteúdo do projeto
- [ ] Testes E2E (se aplicável)
  - N/A — validação no Claude Code entra no checklist manual da task 7.0

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/kspec/bootstrap.go` (novo)
- `internal/kspec/kspec_test.go`
- `internal/tools/bootstrap.go` (novo)
- `internal/tools/bootstrap_test.go` (novo)
- `internal/tools/tools.go` (registro no construtor)
