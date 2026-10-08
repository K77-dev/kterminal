# Tarefa 3.0: Comandos `/image`/`/unimage` e chips de anexos na TUI

<critical>Ler os arquivos de prd.md e techspec.md desta pasta, se você não ler esses arquivos sua tarefa será invalidada</critical>

## Requisitos Atendidos

- REQ-003 — Comandos de anexo

## Dependências

- 1.0 (`llm.ContentPart` disponível para construir as parts no envio)

## Estimativa

- **Tamanho**: G
- **Horas estimadas**: 4-8h

## Visão Geral

A gestão de anexos pendentes na TUI: `/image <caminho>` valida existência, extensão (whitelist png/jpg/jpeg/gif/webp → mime) e tamanho (≤ 5MB via `os.Stat` antes da leitura), lê o arquivo e codifica base64 como data URI; erros aparecem inline em `colError` sem abortar o prompt. `/image` sem argumentos lista os pendentes; `/unimage <n>` remove pelo índice (1-based). Os anexos pendentes aparecem como chips `🖼 nome ×` em `colSecondary` acima do prompt. O envio com anexos constrói as parts (texto + imagens) e entrega ao agent pelo novo método da task 4.0 — aqui a TUI já prepara tudo e chama o método (stub/wiring fino; a semântica de roteamento é a task 4.0).

<skills>
### Conformidade com Skills Padrões

- Stack Go 1.27, módulo `kterminal` — stdlib (`encoding/base64`, `os`).
- Rules de DDD/TS/Vitest não aplicáveis (brownfield Go, conforme techspec).
- Skills do fluxo kspec aplicáveis: `kspec-implement`, `kspec-qa` (fluxo E2E de anexo), `kspec-pr-review`.
- Padrões do projeto: sem comentários no código; receivers por valor na TUI; paleta existente (chips em `colSecondary`).
</skills>

<requirements>
- `attachment{name, size, dataURI}` no Model; `pendingAttachments []attachment`.
- `/image <path>`: valida extensão (map png/jpg/jpeg/gif/webp → mime), `os.Stat` ≤ 5MB, lê, `base64.StdEncoding` → `data:<mime>;base64,<...>`.
- Arquivo inexistente, extensão inválida ou > 5MB → erro inline em `colError`, nada anexado, prompt não abortado.
- `/image` sem args → lista anexos pendentes no chat.
- `/unimage <n>` → remove pelo índice (1-based).
- Chips: linha acima do prompt box, `🖼 nome ×` em `colSecondary`.
- Enter com anexos → envia texto + anexos pelo método do agent com parts; pendentes limpos (exceto no erro de roteamento da task 4.0, que os preserva).
- Bloco do usuário no chat: texto + chips dos anexos enviados.
</requirements>

## Subtarefas

- [ ] 3.1 Adicionar `pendingAttachments` ao Model e implementar `/image <path>` com validações (extensão, 5MB, existência) e codificação base64
- [ ] 3.2 Implementar `/image` sem args (lista) e `/unimage <n>` (remoção 1-based)
- [ ] 3.3 Renderizar chips `🖼 nome ×` acima do prompt e no bloco do usuário enviado
- [ ] 3.4 Ligar o envio com anexos ao método do agent (parts construídas na ordem texto + imagens)
- [ ] 3.5 Escrever os testes 5-9 da techspec (ver Testes da Tarefa)

## Detalhes de Implementação

- Struct `attachment`, validações e chips: techspec, seção **Design de Implementação** (subseção "TUI").
- Segurança (whitelist de extensões, 5MB antes da leitura, mime correto): techspec, seção **Verificações Técnicas → Segurança**.
- Erros de anexo são inline na TUI — sem `EventError`: techspec, seção **Monitoramento e Observabilidade → Error Tracking**.

## Critérios de Sucesso

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` passam.
- Teste prova: `/image shot.png` válido → chip visível, anexo pendente com data URI.
- Teste prova: inexistente / `.txt` / 6MB → erro inline, nada anexado.
- Teste prova: 2 anexos + `/unimage 1` → segundo permanece.
- Teste prova: Enter com anexos → método do agent recebe parts com `image_url`; bloco do chat mostra texto + chips; pendentes limpos.
- Teste prova: `/image` sem args → lista no chat.

## Testes da Tarefa

- [ ] Testes de unidade (`internal/tui/tui_test.go`, temp dir — techspec **Abordagem de Testes → Testes Unidade**, itens 5-9):
  - `TestImageCommandAttaches` — arquivo válido → chip visível, data URI no pendente.
  - `TestImageRejectsInvalid` — inexistente / `.txt` / 6MB → erro inline, nada anexado.
  - `TestUnimageRemovesAttachment` — 2 anexos + `/unimage 1` → segundo permanece.
  - `TestSendWithAttachmentsBuildsParts` — Enter → parts com `image_url`; chat com texto + chips; pendentes limpos.
  - `TestImageWithoutArgsLists` — `/image` → lista no chat.
- [ ] Testes de integração — o contrato TUI→agent com parts é validado em `TestSendWithAttachmentsBuildsParts` (captura).
- [ ] Testes E2E — deferidos para `kspec-qa` (`/image screenshot.png` + pergunta → resposta descreve a imagem).

<critical>SEMPRE CRIE E EXECUTE OS TESTES DA TAREFA ANTES DE CONSIDERÁ-LA FINALIZADA</critical>

## Arquivos relevantes

- `internal/tui/tui.go` — `/image`, `/unimage`, chips, envio com anexos
- `internal/tui/tui_test.go` — comandos, validações, chips, parts construídas
- `internal/llm/llm.go` — `ContentPart` consumido (task 1.0)
