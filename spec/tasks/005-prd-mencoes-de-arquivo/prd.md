# PRD — @menções de arquivo no prompt

## Visão Geral

No kterminal, para o agente trabalhar em um arquivo, ele precisa descobrir o caminho sozinho via glob/grep — gastando turnos (e tokens) só para localizar e ler o que o usuário já sabe onde está. Referenciar arquivos diretamente no prompt é a forma padrão de grounding em terminais agênticos (opencode, Claude Code).

Este PRD define a expansão de `@caminho` no prompt: o conteúdo do arquivo mencionado é anexado à mensagem enviada ao agente, com autocomplete de arquivos enquanto se digita. O usuário primário é o Jev (Typesafe), que quer respostas grounded nos arquivos certos sem tool calls de descoberta.

## Objetivos

- **Qualidade do agente**: o modelo recebe o conteúdo exato do arquivo na primeira chamada, sem tool calls de leitura.
- **Produtividade**: menos turnos desperdiçados em descoberta de caminhos.
- **Ergonomia**: autocomplete rápido enquanto digita, sem sair do teclado.
- **Previsibilidade**: menções inválidas ou em excesso têm comportamento definido e visível.

## Histórias de Usuário

- Como Jev, quero digitar `@main.go o que esse arquivo faz?` e receber uma resposta sobre o arquivo sem o agente precisar lê-lo via tool, para economizar turnos.
- Como Jev, quero um popup de sugestões de arquivos ao digitar `@`, para completar caminhos com Tab sem erro de digitação.
- Como Jev, quero que menções de arquivos inexistentes passem intactas ao agente, para que ele próprio decida o que fazer (ex.: criar o arquivo).
- Como Jev, quero ver o texto original do meu prompt no chat (sem o conteúdo expandido), para manter a leitura do histórico limpa.

## Funcionalidades Principais

### REQ-001 — Expansão de menções

Antes de enviar o prompt ao agente, a TUI processa o texto procurando tokens `@<caminho>`:

- Cada menção de arquivo existente é expandida: a mensagem enviada ao agente contém o texto original (com os tokens intactos) seguido dos blocos `--- Arquivo @caminho ---` com o conteúdo de cada arquivo (mesmo limite de 64k do tool `read`).
- Arquivo inexistente: o token permanece como texto — o agente decide o que fazer.
- O bloco do usuário no chat mostra só o texto original.
- A expansão acontece na TUI, antes do agente — o agente não conhece o conceito de menções.

#### Critérios de Aceite

- `@main.go o que esse arquivo faz?` → o modelo recebe o conteúdo e responde sobre ele sem tool call de leitura.
- Menção de arquivo inexistente passa o texto intacto ao agente.

---

### REQ-002 — Autocomplete com popup

Ao digitar `@`, a TUI abre um popup de sugestões entre o viewport e o prompt box:

- Sugestões via glob do prefixo digitado (ex.: `@internal/t` → arquivos de `internal/tui/`), máximo 10 resultados, ordenados.
- `Tab` completa com o primeiro resultado; `Enter` com popup aberto completa (não envia); `Esc` fecha o popup.
- Itens em cor de texto muted; item selecionado em cor primária.

#### Critérios de Aceite

- Popup lista arquivos reais do diretório corrente conforme o prefixo.
- Tab completa, Enter completa sem enviar, Esc fecha.

---

### REQ-003 — Proteções de expansão

- Menções dentro de blocos de código no texto (linha iniciada por 4 espaços ou ```) são ignoradas.
- Máximo de 5 arquivos expandidos por mensagem; excedentes ficam como texto com aviso inline em cor de warning ("max 5 file mentions").

#### Critérios de Aceite

- Menção dentro de code fence não é expandida.
- 6+ menções → 5 expandidas + aviso inline visível.

---

### REQ-004 — Transcript fiel

O transcript grava a mensagem já expandida (o que o modelo efetivamente viu) no evento `user`.

#### Critérios de Aceite

- Evento `user` no JSONL contém a mensagem expandida completa.

## Experiência do Usuário

- O fluxo de digitação não muda: `@` abre o popup naturalmente, sem comando ou atalho novo para lembrar.
- O chat permanece limpo: o usuário vê só o que digitou, nunca o conteúdo expandido dos arquivos.
- Feedback imediato para limites (aviso de máximo de menções) sem bloquear o envio.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- A expansão acontece na TUI antes do envio ao agente — o agente não conhece menções. Requisito não negociável.
- Limite de leitura por arquivo idêntico ao do tool `read` (64k).
- Popup posicionado entre viewport e prompt box, usando apenas a paleta de cores existente.

## Fora de Escopo

- Menções de diretório (expansão recursiva).
- Menções de URL ou recursos externos.
- Fuzzy matching no autocomplete.
- Expansão de menções em mensagens do agente ou em respostas.
- Anexo de arquivos binários.
