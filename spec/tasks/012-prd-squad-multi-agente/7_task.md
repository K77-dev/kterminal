# Tarefa 7.0: Rules Go mínimas com frontmatter + dogfooding da primeira mesa real

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Convocação dinâmica e subset de rules
- REQ-009 — Ponte SDD e execução do plano

## Dependências

- 1.0 (pacote `internal/squad` — personas e loader)
- 3.0 (Agent core — `buildState` por papel, ativação de modo)
- 4.0 (Tools — `task` com `persona`, `squad_kickoff`, subset de rules)
- 6.0 (TUI — popup `/mode`, render por papel)

## Estimativa

- **Tamanho**: M
- **Horas estimadas**: 2-4h

## Visão Geral

Criar as rules Go mínimas com frontmatter declarando disciplinas (dogfooding — as personas do próprio kterminal consomem estas rules) e executar a primeira mesa real (arquiteto + backend + qa) resolvendo um problema do próprio kterminal, verificando: mesa anunciada antes do gasto, contribuições rotuladas por papel, convergência dentro dos tetos, fluxo SDD intacto após `/mode sdd`.

## Conformidade com Skills Padrões

- Go 1.27, sem comentários
- Rules Go do próprio código-base: sem comentários, eventos via canal, TUI com receivers por valor, `strings.Builder` por ponteiro, tools registradas no construtor, assets via `go:embed`
- Frontmatter de rule com `disciplines` para mapeamento rule → disciplina
- `.agents/rules/architecture-ddd.md` orienta projetos TypeScript/Java; o kterminal é Go flat com `internal/` — prevalecem os padrões do código-base (AGENTS.md)

## Requisitos

- Rules Go mínimas com frontmatter declarando disciplinas (`disciplines: [backend, architecture, ...]`)
- Rules cobrem: build/vet/test, sem comentários, eventos via canal, TUI receivers por valor, `strings.Builder` por ponteiro, tools no construtor, assets via `go:embed`
- Rules sem frontmatter ficam fora do subset de persona (comportamento intencional — fim do bundle-all)
- Dogfooding: primeira mesa real (arquiteto + backend + qa) resolve um problema do próprio kterminal usando as rules Go
- Verificação: mesa anunciada antes do gasto, contribuições rotuladas, convergência dentro dos tetos
- Fluxo SDD intacto após `/mode sdd`

## Subtarefas

- [ ] 7.1 Criar `.agents/rules/go.md` com frontmatter `disciplines: [backend, architecture, database, qa, test]` e corpo com as rules Go mínimas (build/vet/test, sem comentários, eventos via canal, TUI receivers por valor, strings.Builder por ponteiro, tools no construtor, assets via go:embed)
- [ ] 7.2 Verificar que o loader de rules do kspec (`kspec.Store.Rules()`) carrega a rule com frontmatter
- [ ] 7.3 Verificar que o subset de rules por disciplina (tarefa 4.0) filtra corretamente — persona `architect` recebe rules com `disciplines: [architecture]`
- [ ] 7.4 Executar a primeira mesa real de dogfooding: `/mode` → `squad` → pedido de um problema do próprio kterminal (ex: "refactor the compaction logic to preserve agent labels") → arquiteto + backend + qa deliberam → convergência → plano
- [ ] 7.5 Documentar o resultado do dogfooding: mesa anunciada antes do gasto, contribuições rotuladas por papel, convergência dentro dos tetos, fluxo SDD intacto após `/mode sdd`

## Detalhes de Implementação

Consulte "Riscos Conhecidos" e "Conformidade com Skills Padrões" na `techspec.md`:

- Rules sem frontmatter ficam fora do subset de persona: comportamento intencional (fim do bundle-all), mas requer escrever as rules Go mínimas com frontmatter desde o início
- Dogfooding: a primeira mesa real (arquiteto + backend + qa) resolve um problema do próprio kterminal usando as rules Go

O problema do próprio kterminal para a mesa de dogfooding pode ser: "refactor the compaction logic to preserve agent labels in the transcript" ou similar — um problema real que beneficia de múltiplas perspectivas (arquiteto para design, backend para implementação, qa para testes).

## Critérios de Sucesso

- `.agents/rules/go.md` existe com frontmatter `disciplines` e corpo com as rules Go mínimas
- O loader carrega a rule; o subset por disciplina filtra corretamente
- A primeira mesa real converge sem intervenção dentro dos tetos (8 convocações / 200k tokens)
- Contribuições são rotuladas por papel no transcript
- Mesa é anunciada antes do gasto
- Fluxo SDD funciona normalmente após `/mode sdd`

## Testes da Tarefa

- [ ] Testes de unidade — verificar que o loader de rules carrega `.agents/rules/go.md` com frontmatter e o subset por disciplina filtra corretamente
- [ ] Testes de integração — não aplicável (dogfooding é manual)
- [ ] Testes E2E (dogfooding documentado):
  - Mesa anunciada antes do gasto
  - Contribuições rotuladas por papel
  - Convergência dentro dos tetos
  - Fluxo SDD intacto após `/mode sdd`

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `.agents/rules/go.md` (novo — rules Go mínimas com frontmatter)
- Documentação do dogfooding (pode ser um comentário no PR ou um arquivo de registro)
