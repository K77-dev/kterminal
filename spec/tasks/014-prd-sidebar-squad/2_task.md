# Tarefa 2.0: Session — campos aditivos `Event.Mesa`/`Snapshot.Mesa` e assinatura de `WriteSnapshot`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-006 — Persistência e resume (campos aditivos retrocompatíveis)

## Dependências

- 1.0 (tipo `squad.Mesa`)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Estender `internal/session` para carregar a Mesa no transcript e no snapshot: (1) campo `Mesa *squad.Mesa` (aditivo, `omitempty`) em `session.Event`, presente apenas na linha `snapshot` do JSONL; (2) campo `Mesa *squad.Mesa` em `Snapshot`; (3) `WriteSnapshot` ganha parâmetro `mesa *squad.Mesa`. A mudança de assinatura quebra o compile das chamadas existentes — o compile error lista exatamente os call sites que a task 3.0 vai ajustar (padrão já usado na feature 012 com `Skill`/`Mode`).

<skills>
### Conformidade com Skills Padrões

Rules de `.agents/rules/` são TS/Java; para o Go do kterminal valem os padrões do código-base (AGENTS.md): sem comentários, snapshot aditivo com `omitempty` (precedentes: `Skill`, `Mode`), `session` importa tipos de domínio leaf (mesmo padrão do import de `llm` — sem ciclo).
</skills>

<requirements>
- `session.Event` ganha `Mesa *squad.Mesa \`json:"mesa,omitempty"\``
- `session.Snapshot` ganha `Mesa *squad.Mesa`
- `WriteSnapshot(messages []llm.Message, skill, mode string, mesa *squad.Mesa)`
- `Load` devolve a Mesa do último snapshot; `Mesa == nil` é estado legítimo ("mesa não iniciada")
- Snapshots antigos (sem `mesa`) desserializam sem erro com `Mesa == nil`
- A Mesa nunca carrega prompts nem conteúdo de mensagens (apenas roles, tetos, consumo e entradas)
</requirements>

## Subtarefas

- [ ] 2.1 Adicionar `Mesa *squad.Mesa` em `session.Event` com tag `json:"mesa,omitempty"`
- [ ] 2.2 Adicionar `Mesa *squad.Mesa` em `Snapshot` e estender a assinatura de `WriteSnapshot`
- [ ] 2.3 Ajustar call sites de `WriteSnapshot` para compilar (passar `nil` — o Agent só passará a Mesa real na task 3.0)
- [ ] 2.4 Garantir que `Load`/`LoadLatest` propagam a Mesa do snapshot
- [ ] 2.5 Testes: round-trip `WriteSnapshot`→`Load` com Mesa; snapshot antigo sem `mesa` carrega com `Mesa == nil`

## Detalhes de Implementação

Consulte techspec.md — seção "Modelos de Dados". A linha `snapshot` do JSONL ganha `mesa` (roles, tetos, consumo, entradas) — é o log estruturado do estado da mesa, rastreável no resume. Binários antigos ignoram o campo desconhecido (rollback seguro).

## Critérios de Sucesso

- `go build ./...` compila (call sites ajustados com `nil`)
- Snapshot com Mesa populada persiste e `Load` devolve valores idênticos
- JSONL de sessão antiga (sem `mesa`) carrega sem erro, `Mesa == nil`
- `go test ./internal/session/` verde

## Testes da Tarefa

- [ ] Testes de unidade (`session_test.go`):
  - Round-trip `WriteSnapshot`→`Load` com Mesa populada (roles, entradas, contadores) — deep equal
  - `WriteSnapshot` com `mesa == nil` não emite o campo no JSON (omitempty)
  - Snapshot pré-feature (JSON sem `mesa`) carrega com `Mesa == nil`
- [ ] Testes de integração: resume lendo JSONL real com linha `snapshot` contendo `mesa` (a validação ponta-a-ponta com Agent fica na 6.0)
- [ ] Testes E2E: N/A

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/session/session.go` (modificado — Event.Mesa, Snapshot.Mesa, WriteSnapshot, Load)
- `internal/session/session_test.go` (modificado)
- `internal/agent/agent.go` (ajuste mecânico de call site — passa `nil`; wiring real na 3.0)
