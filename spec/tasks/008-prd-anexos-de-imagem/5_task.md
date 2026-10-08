# Tarefa 5.0: Verificação final integrada e checagem de critérios de aceite

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-001 — Mensagens com content parts
- REQ-002 — Catálogo com capacidade de visão
- REQ-003 — Comandos de anexo
- REQ-004 — Roteamento por visão
- REQ-005 — Transcript sem base64

## Dependências

- 4.0 (feature completa: llm, catálogo, TUI e agent implementados e testados)

## Estimativa

- **Tamanho**: P
- **Horas estimadas**: 1-2h

## Visão Geral

Fechamento da feature: rodar a verificação obrigatória do projeto completa, checar cruzadamente os critérios de aceite do PRD contra o que foi implementado e confirmar a conformidade com os padrões do código — com atenção especial ao requisito não negociável de privacidade (base64 só ao gateway). Esta task não escreve código novo — se qualquer verificação falhar, o ajuste volta para a task responsável (1.0, 2.0, 3.0 ou 4.0). Ao final, a funcionalidade está pronta para o `kspec-qa` (E2E) e para o `kspec-pr-review`.

<skills>
### Conformidade com Skills Padrões

- Verificação obrigatória do projeto (PRD): `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Skills do fluxo kspec aplicáveis: `kspec-qa` (próximo passo — E2E: `/image` + pergunta → resposta descreve a imagem; compatibilidade sem anexos; erro amigável), `kspec-pr-review` (revisão semântica contra PRD/Tech Spec/tasks).
- Padrões do projeto a conferir: sem comentários no código; eventos via canal; receivers por valor na TUI; base64 nunca no transcript nem no state do Jev.
</skills>

<requirements>
- `go build ./...`, `go vet ./...`, `gofmt -l .` (saída vazia) e `go test ./...` todos verdes em uma única execução encadeada.
- Checklist dos critérios de aceite do PRD percorrido item a item, com evidência (teste que o cobre ou deferimento explícito ao QA).
- Conformidade com os padrões de código do projeto confirmada por inspeção do diff.
- Nenhum requisito do PRD deixado de lado; itens fora de escopo do PRD não implementados (colar do clipboard, captura de screenshot, PDF/vídeo/áudio, OCR local, redimensionamento automático, visão em mensagens do agente).
</requirements>

## Subtarefas

- [ ] 5.1 Executar `go build ./... && go vet ./... && gofmt -l . && go test ./...` e confirmar tudo verde
- [ ] 5.2 Percorrer o checklist de critérios de aceite do PRD (ver Critérios de Sucesso) e registrar a evidência de cada item
- [ ] 5.3 Inspecionar o diff completo contra os padrões do projeto (sem comentários; sem `base64,` em nenhum caminho de transcript/state)
- [ ] 5.4 Confirmar prontidão para `kspec-qa`: listar os cenários E2E da techspec que o QA deve executar

## Detalhes de Implementação

- Sem código novo. Verificação obrigatória definida no PRD (seção **Restrições Técnicas de Alto Nível**) e na techspec (seção **Infraestrutura**).
- Cenários E2E para o QA estão na techspec, seção **Abordagem de Testes → Testes de E2E**.
- Se um critério de aceite falhar, não corrigir nesta task — reabrir a task responsável (1.0 llm, 2.0 catálogo, 3.0 TUI, 4.0 agent) e corrigir lá.

## Critérios de Sucesso

- Verificação encadeada completa passa sem erros.
- Checklist de aceite com evidência por item:
  - Mensagem com imagem chega ao gateway como array de content parts (`TestRequestContainsContentParts`).
  - Sem anexos, o request é idêntico ao atual (`TestMessageMarshalStringContentGolden`).
  - O catálogo expõe visão por modelo vinda do YAML (`TestVisionParsedFromYAML`).
  - Arquivo inexistente/extensão inválida → erro claro, nada anexado; > 5MB → rejeitado (`TestImageRejectsInvalid`).
  - Chips visíveis; `/unimage` remove o anexo correto (`TestImageCommandAttaches` + `TestUnimageRemovesAttachment`).
  - Com anexo, o Jev só recebe modelos `vision: true` (`TestVisionFilterRoutesOnlyVisionModels`).
  - Nenhum modelo de visão → erro amigável, anexos preservados (`TestNoVisionModelPreservesAttachments`).
  - O JSONL contém metadados dos anexos, sem base64 (`TestTranscriptHasNoBase64`).
- Diff conforme os padrões do projeto.

## Testes da Tarefa

- [ ] Testes de unidade — todos os suites das tasks 1.0, 2.0, 3.0 e 4.0 passando (`go test ./...`).
- [ ] Testes de integração — cobertos pelos testes de agent com mocks (task 4.0).
- [ ] Testes E2E — nenhum novo nesta task; execução deferida ao `kspec-qa` com a lista de cenários montada na subtarefa 5.4.

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/llm/llm.go`, `internal/llm/llm_test.go` — diff da task 1.0
- `internal/catalog/models.yaml`, `internal/catalog/catalog.go` — diff da task 2.0
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — diff da task 3.0
- `internal/agent/agent.go`, `internal/session/session.go` — diff da task 4.0
- `spec/tasks/008-prd-anexos-de-imagem/prd.md` — critérios de aceite (fonte do checklist)
- `spec/tasks/008-prd-anexos-de-imagem/techspec.md` — cenários E2E para o QA
