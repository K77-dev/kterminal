# 05 — @menções de arquivo no prompt

## Contexto

kterminal é um terminal agêntico em Go (Bubble Tea). Para o agente trabalhar num arquivo, hoje ele precisa descobrir o caminho sozinho via glob/grep. Referenciar arquivos diretamente no prompt é a forma padrão de grounding em terminais agênticos (opencode, Claude Code).

Arquitetura relevante:

- `internal/tui/tui.go` — input `textinput.Model` single-line; `handleChatKey` trata Enter → `agent.Run(value)`.
- `internal/agent/agent.go` — `Run(userInput)` anexa a mensagem do usuário a `messages`.
- `internal/tools/fs.go` — `read` já lê arquivos com truncamento em 64k.

## Objetivo

Digitar `@caminho` no prompt anexa o conteúdo do arquivo à mensagem; autocomplete com lista de arquivos enquanto digita.

## Especificação

1. **Expansão**: antes de `agent.Run`, a TUI processa o texto do input procurando tokens `@<caminho>` (regex `@([^\s@]+)`). Para cada menção: ler o arquivo (mesmo limite de 64k do tool `read`); se não existir, deixar o token como está (o agente decide o que fazer). A mensagem enviada ao agente vira:
   ```
   <texto original com os tokens @caminho intactos>

   --- Arquivo @main.go ---
   <conteúdo>
   ```
   O bloco do usuário no chat continua mostrando só o texto original.
2. **Autocomplete**: ao digitar `@`, a TUI abre um popup de sugestões: glob do prefixo digitado (`@internal/t` → `internal/tui/*.go` etc.), máximo 10 resultados, ordenados. `Tab` completa com o primeiro resultado; `Enter` com popup aberto completa (não envia); `Esc` fecha o popup. Popup renderizado entre o viewport e o prompt box, cada item em `colTextMuted`, selecionado em `colPrimary`.
3. Ignorar menções dentro de blocos de código no texto (linha iniciada por 4 espaços ou ```).
4. Limite: no máximo 5 arquivos expandidos por mensagem; além disso, aviso inline (`colWarning`: "max 5 file mentions") e os excedentes ficam como texto.
5. Transcript: gravar a mensagem já expandida (o que o modelo viu) no evento `user`.

## Critérios de aceitação

- `@main.go o que esse arquivo faz?` → o modelo recebe o conteúdo e responde sobre ele sem tool call de leitura.
- Popup lista arquivos reais do cwd, Tab completa, Esc fecha.
- Menção de arquivo inexistente passa o texto intacto ao agente.
- 6+ menções → 5 expandidas + aviso.

## Verificação

```
go build ./... && go vet ./... && gofmt -l . && go test ./...
```

Testes: função de expansão (arquivo existe/não existe, múltiplas menções, limite de 5, menção dentro de code fence ignorada); teste de TUI simulando digitação de `@` + verificação de que o popup lista arquivos do diretório de teste (criar temp dir com arquivos).

## Restrições

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- A expansão acontece na TUI antes de `Run` — o agent não conhece menções.
