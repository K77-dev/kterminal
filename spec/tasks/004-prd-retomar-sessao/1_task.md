# Tarefa 1.0: Fundações de snapshot em `internal/session`

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Snapshot por turno (mecanismo de gravação/leitura)
- REQ-002 — Carregamento de sessão

## Dependências

- Nenhuma

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

Fundação isolada e testável do resume: `internal/session` ganha o campo `Messages json.RawMessage` no `Event`, o método `WriteSnapshot(messages []llm.Message)` (serializa o array completo como evento `snapshot`), as funções de carregamento `Load(path)` (lê de trás para frente e usa o **último** snapshot; sem snapshot → erro explícito "sessão sem snapshot") e `LoadLatest()` (sessão mais recente por nome de arquivo; sem arquivos → `ErrNoSessions`), e o `AppendWriter(path)` que abre arquivo existente em `O_APPEND` para a sessão retomada continuar no mesmo JSONL. O snapshot é a fonte da verdade — nunca replay de eventos granulares (requisito não negociável do PRD). Nesta task nada grava snapshot ainda — o agent chega na task 2.0.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`os`, `encoding/json`, `sort`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme seção "Conformidade com Skills Padrões" da techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement` (execução), `kspec-qa` (E2E deferido), `kspec-pr-review` (revisão semântica).
- Padrões do projeto: sem comentários no código; `session` importa `kterminal/internal/llm` apenas pelo tipo `Message` (sem ciclo).
</skills>

<requirements>
- `Event` ganha `Messages json.RawMessage` com `json:"messages,omitempty"`.
- `WriteSnapshot(messages []llm.Message) error` — `json.Marshal(messages)` → `Event{Type: "snapshot", Messages: raw}` → `Write` normal (ts preenchido).
- `Load(path) ([]llm.Message, error)` — itera as linhas de trás para frente; primeiro `Type == "snapshot"` vence; sem snapshot → `errors.New("sessão sem snapshot")`; linha inválida (crash anterior) é ignorada naturalmente no scan.
- `LoadLatest() (path string, messages []llm.Message, err error)` — `os.ReadDir(session.Dir())`, filtra `*.jsonl`, ordena por nome, pega o último; sem arquivos → `ErrNoSessions`.
- `AppendWriter(path) (*Writer, error)` — abre arquivo existente em `O_APPEND`, preserva o modo (0600); `Path()` retorna o caminho original.
- Snapshot serializa `[]llm.Message` incluindo `ToolCalls`/`ToolCallID` — o par tool_call↔tool result sobrevive ao resume.
- `Load` rejeita JSON malformado com erro explícito — nunca crash.
</requirements>

## Subtarefas

- [ ] 1.1 Adicionar `Messages json.RawMessage` ao `session.Event`
- [ ] 1.2 Implementar `WriteSnapshot`
- [ ] 1.3 Implementar `Load` (scan de trás para frente, último snapshot, erro explícito sem snapshot)
- [ ] 1.4 Implementar `LoadLatest` (ordenação por nome, `ErrNoSessions`)
- [ ] 1.5 Implementar `AppendWriter` (modo append sobre arquivo existente)
- [ ] 1.6 Escrever os testes 1-5 da techspec em `internal/session/session_test.go` (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinaturas e regras de carregamento: techspec, seção **Design de Implementação → Interfaces Principais** (subseção "Regras do carregamento").
- Modelo de dados e racional snapshot vs replay: techspec, seção **Design de Implementação → Modelos de Dados** e **Considerações Técnicas → Decisões Principais** (item 1).
- Degradação graciosa com linha parcial no fim do arquivo: techspec, seção **Verificações Técnicas → Segurança** e **Considerações Técnicas → Riscos Conhecidos**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste mandatório de roundtrip: 3 turnos simulados → `Load` retorna mensagens byte-idênticas (JSON marshalado), incluindo `ToolCalls`/`ToolCallID`.
- Dois snapshots → `Load` retorna o segundo; sem snapshot → erro explícito.
- `LoadLatest` pega o mais recente de três arquivos; diretório vazio → `ErrNoSessions`.
- `AppendWriter` preserva conteúdo anterior e o `Path()` é o original.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/session/session_test.go`, conforme techspec **Abordagem de Testes → Testes Unidade**, itens 1-5):
  - `TestSnapshotRoundtrip` (mandatório) — mensagens byte-idênticas, incluindo `ToolCalls`/`ToolCallID`.
  - `TestLoadUsesLastSnapshot` — dois snapshots → retorna o segundo.
  - `TestLoadWithoutSnapshot` — eventos sem snapshot → erro "sessão sem snapshot".
  - `TestLoadLatestPicksNewest` — mais recente de três; vazio → `ErrNoSessions`.
  - `TestAppendWriterAppends` — append preserva conteúdo; `Path()` original.
- [ ] Testes de integração — fora do escopo da task; o fluxo com agent é validado na task 2.0.
- [ ] Testes E2E — deferidos para `kspec-qa`.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/session/session.go` — `Messages` no `Event`, `WriteSnapshot`, `Load`, `LoadLatest`, `AppendWriter`
- `internal/session/session_test.go` — roundtrip, último snapshot, sem snapshot, `LoadLatest`, append
- `internal/llm/llm.go` — sem mudança (`Message` serializada como está)
