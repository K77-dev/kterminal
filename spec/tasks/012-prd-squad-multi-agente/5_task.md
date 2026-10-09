# Tarefa 5.0: Session — campo `Agent` no `Event`, `Mode` no `Snapshot`, restore no resume

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-008 — Eventos, transcript e identidade visual por papel

## Dependências

- 3.0 (Agent core — `Event.Agent` e ativação de modo)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: < 2h

## Visão Geral

Modificar `internal/session` para persistir a identidade do agente e o modo ativo: (1) campo `Agent string` (aditivo, `omitempty`) em `session.Event` para o transcript JSONL, (2) campo `Mode string` em `Snapshot` para persistência e restore no resume.

## Conformidade com Skills Padrões

- Go 1.27, sem comentários
- JSONL é o log — cada evento de persona carrega `agent` (papel), `depth`, `model`, `router`, `cost`, `tps`
- Precedente: `Skill string` já existe em `Event` com `omitempty` e é restaurado no resume

## Requisitos

- `session.Event` ganha `Agent string \`json:"agent,omitempty"\``
- `Snapshot` ganha `Mode string`
- `WriteSnapshot` aceita e persiste o modo ativo
- `Load` restaura o modo do snapshot
- `LoadLatest` propaga o modo restaurado
- Compatível com JSONL existente (campo `agent` ausente em sessões antigas → string vazia)

## Subtarefas

- [ ] 5.1 Adicionar `Agent string` em `session.Event` com tag `json:"agent,omitempty"`
- [ ] 5.2 Adicionar `Mode string` em `Snapshot`
- [ ] 5.3 Modificar `WriteSnapshot` para aceitar e gravar o modo ativo
- [ ] 5.4 Modificar `Load` para restaurar o modo do snapshot
- [ ] 5.5 Garantir compatibilidade retroativa: JSONL sem `agent` ou `mode` não quebra

## Detalhes de Implementação

Consulte "Modelos de Dados" na `techspec.md`:

- `session.Event` ganha `Agent string \`json:"agent,omitempty"\``
- `Snapshot` ganha `Mode string`

O `WriteSnapshot` atual (`session.go:90`) aceita `messages` e `skill`. Estender para aceitar `mode` — ou adicionar um campo na `Event` do snapshot. O `Load` (`session.go:108`) lê o último snapshot e retorna `Snapshot{Messages, Skill}`. Estender para incluir `Mode`.

O `main.go` já restaura `resumed.Skill` via `ag.RestoreSkill(resumed.Skill)`. Adicionar restauração de modo análoga: se `resumed.Mode == "squad"`, chamar `ag.ActivateMode("squad")`.

## Critérios de Sucesso

- Eventos de persona carregam o campo `agent` com o papel no JSONL
- O JSONL registra `agent` em cada evento de persona
- Snapshot persiste o modo; resume restaura `squad` se era o ativo ao salvar
- JSONL antigo (sem `agent`/`mode`) carrega sem erro

## Testes da Tarefa

- [ ] Testes de unidade (`session_test.go`):
  - Event com `Agent: "architect"` serializa `"agent":"architect"` no JSONL
  - Event sem `Agent` não inclui o campo (omitempty)
  - Snapshot com `Mode: "squad"` é persistido e restaurado por `Load`
  - JSONL antigo (sem `agent`/`mode`) carrega sem erro (compatibilidade retroativa)
- [ ] Testes de integração — resume com snapshot `Mode: "squad"` restaura o modo

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/session/session.go` (modificado — Event.Agent, Snapshot.Mode, WriteSnapshot, Load)
- `internal/session/session_test.go` (modificado)
- `main.go` (modificado — restore de modo no resume)
