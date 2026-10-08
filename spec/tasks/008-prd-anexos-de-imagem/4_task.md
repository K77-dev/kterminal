# Tarefa 4.0: `RunWithAttachments`, roteamento por visão e transcript sem base64

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-004 — Roteamento por visão
- REQ-005 — Transcript sem base64

## Dependências

- 1.0 (content parts no `llm.Message`)
- 2.0 (`Vision` no catálogo)
- 3.0 (TUI enviando texto + parts)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

O agent assume a roteagem por visão: `RunWithAttachments(text, parts, meta)` recebe a mensagem com parts (`Run` vira wrapper com parts nil — callers existentes inalterados). Em `decide()`, quando a mensagem tem imagens, os candidatos são filtrados para `Vision == true` **antes** de consultar o Jev (routers permanecem genéricos — a filtragem é do agent, dono do contexto da chamada); o state do Jev ganha o sufixo "This step includes image attachments.". Sem modelos com visão → `EventError` amigável "no vision-capable model available" **sem consumir a mensagem** (anexos preservados na TUI para o usuário remover). O transcript grava `attachments: [{name, size}]` no evento `user` — metadados apenas, **nunca base64** (requisito não negociável de privacidade, com teste automático de não-vazamento). O snapshot (techspec 004) serializa parts de imagem como placeholder `"[omitted]"` — imagens são efêmeras.

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib.
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (fluxo E2E de anexo + roteamento), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; eventos via canal; base64 só ao gateway (nunca ao Jev nem ao transcript).
</skills>

<requirements>
- `RunWithAttachments(text string, parts []llm.ContentPart, meta []session.AttachmentMeta)`; `Run` vira wrapper com parts nil.
- Filtragem em `decide` quando a mensagem tem imagens: `candidates = filter(candidates, m.Vision)` — routers intocados.
- State do Jev com anexos: sufixo `"This step includes image attachments."` no `buildState`.
- Sem candidatos de visão → `emitError("no vision-capable model available")` sem anexar a user message ao histórico (turno não consumido; anexos preservados na TUI).
- Pin manual (`/model`) em modelo sem visão com anexos ativos: pin é ignorado, segue a filtragem normal.
- `session.Event` ganha `Attachments []AttachmentMeta{name, size}` (`json:"attachments,omitempty"`) — gravado no evento `user`.
- `WriteSnapshot` serializa parts de imagem como placeholder `{"type":"image_url","image_url":{"url":"[omitted]"}}` — sem base64 no JSONL.
- O base64 existe só no request HTTP ao gateway — nunca no state do Jev, nunca no transcript.
</requirements>

## Subtarefas

- [ ] 4.1 Implementar `RunWithAttachments` com `Run` como wrapper
- [ ] 4.2 Filtrar candidatos por `Vision` em `decide` quando há imagens; sufixo no state do Jev
- [ ] 4.3 Emitir erro amigável sem consumir a mensagem quando não há modelos com visão
- [ ] 4.4 Adicionar `AttachmentMeta` ao `session.Event` e gravar metadados no evento `user`
- [ ] 4.5 Serializar parts de imagem como placeholder no `WriteSnapshot`
- [ ] 4.6 Escrever os testes 10-13 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Assinaturas, filtragem e regra do erro sem consumo: techspec, seção **Design de Implementação** (subseção "Agent").
- Decisões "filtragem no agent, routers intocados", "turno não consumido no erro", "data URI inline": techspec, seção **Considerações Técnicas → Decisões Principais** (itens 2, 3, 4).
- Mitigação do snapshot com placeholder: techspec, seção **Considerações Técnicas → Riscos Conhecidos** (item "Snapshot com base64 no JSONL").
- Privacidade (base64 só ao gateway): techspec, seção **Verificações Técnicas → Segurança**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: mensagem com imagem → mock Jev recebe só candidatos `Vision == true`; state contém "image attachments".
- Teste prova: gateway sem modelos de visão → `EventError` amigável; `messages` não ganhou a user message.
- Teste prova: mock gateway decodifica o request — `messages[0].content` é array com `image_url` (data URI).
- Teste prova: evento `user` no JSONL contém `attachments: [{name, size}]` e nenhum `base64,` no arquivo.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/agent/agent_test.go`, mock gateway + mock Jev — techspec **Abordagem de Testes → Testes Unidade**, itens 10-13):
  - `TestVisionFilterRoutesOnlyVisionModels` — só candidatos com visão; state menciona anexos.
  - `TestNoVisionModelPreservesAttachments` — `EventError` amigável; turno não consumido.
  - `TestRequestContainsContentParts` — request com `content` array e `image_url` (data URI).
  - `TestTranscriptHasNoBase64` — `attachments: [{name, size}]` no JSONL; nenhum `base64,` no arquivo.
- [ ] Testes de integração — cobertos pelos testes de agent (TUI→agent→gateway→transcript com mocks).
- [ ] Testes E2E — deferidos para `kspec-qa` (resposta descreve a imagem; compatibilidade sem anexos; erro amigável com anexos preservados).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/agent/agent.go` — `RunWithAttachments`, filtragem por visão, state, erro amigável
- `internal/agent/agent_test.go` — filtro, preservação de anexos, request com parts, transcript sem base64
- `internal/session/session.go` — `AttachmentMeta` no evento `user`; placeholder no snapshot
- `internal/tui/tui.go` — envio com anexos consumindo `RunWithAttachments` (task 3.0)
