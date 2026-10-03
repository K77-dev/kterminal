# 08 — Anexos de imagem no prompt

## Contexto

kterminal é um terminal agêntico em Go onde o Jev escolhe o LLM a cada chamada. Vários modelos do gateway aceitam imagens (o `glm-5.3` do usuário tem `attachment: true` na config do opencode), mas o kterminal só troca texto.

Arquitetura relevante:

- `internal/llm/llm.go` — `Message.Content` é `string`; o request usa o formato OpenAI-compatible.
- `internal/catalog/models.yaml` — metadados por modelo (preços, TPS, contexto, tags).
- `internal/router/router.go` — `Route(ctx, state, candidates)`; o Jev escolhe entre os candidatos.
- `internal/tui/tui.go` — input e comandos slash.

## Objetivo

Anexar imagens ao prompt (`/image <caminho>` e colar caminho de imagem) e fazer o Jev rotear apenas para modelos com visão.

## Especificação

1. **Formato de mensagem**: `llm.Message` ganha suporte a content parts — `ContentParts []ContentPart` com `ContentPart{Type: "text"|"image_url", Text string, ImageURL *ImageURL{URL string}}` e `MarshalJSON` customizado: sem parts → `"content": "string"` (compatibilidade total); com parts → array OpenAI (`{"type":"image_url","image_url":{"url":"data:image/png;base64,..."}}`).
2. **Catálogo**: campo `vision: true|false` por modelo no `models.yaml` (glm-5.3: true; os demais: false até confirmação). `catalog.Model` ganha `Vision bool`.
3. **Comando `/image <caminho>`**: na TUI, anexa a imagem à próxima mensagem (lista de anexos pendentes visível acima do prompt como chips `🖼 screenshot.png ×` em `colSecondary`; `/image` sem argumentos lista anexos; `/unimage <n>` remove). Validar existência e extensão (png, jpg, jpeg, gif, webp); ler e encodar base64 (limite 5MB).
4. **Envio**: no Enter, se há anexos, a mensagem do usuário vira content parts: `[text do prompt, image_url × N]`. O bloco do usuário no chat mostra o texto + chips dos arquivos.
5. **Roteamento**: `Agent` sinaliza ao router que a chamada tem imagens; o `JevRouter`/`HeuristicRouter` filtram candidatos para `Vision == true` antes de montar o Choice (se nenhum modelo tiver visão → `EventError` claro: "no vision-capable model available"). O state enviado ao Jev menciona "this step includes image attachments".
6. **Transcript**: gravar anexos como metadados (nome, tamanho) — nunca o base64.

## Critérios de aceitação

- Mensagem com imagem chega ao gateway como array de content parts (verificar no mock).
- Sem anexos, o request permanece byte-a-byte igual ao atual (string content).
- Com anexo, o Jev só recebe modelos com `vision: true` como critérios.
- Nenhum modelo de visão no gateway → erro amigável, anexos preservados para o usuário remover.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: `MarshalJSON` da Message (string vs parts, golden JSON); router filtra candidatos por `Vision`; fluxo de agente com mock: mensagem com parts passa pelo `ChatStream` e o request decodificado no servidor de teste contém o array; limite de 5MB rejeita arquivo grande.

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Não enviar base64 ao Jev nem ao transcript — só ao gateway LLM.
