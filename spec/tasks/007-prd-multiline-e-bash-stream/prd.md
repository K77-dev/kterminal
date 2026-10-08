# PRD — Input multi-linha + stream do output do bash

## Visão Geral

O kterminal tem duas fricções de UX que este PRD resolve em conjunto:

1. **Prompt single-line**: prompts longos ficam ilegíveis numa linha só.
2. **Bash bloqueante**: o tool `bash` espera o comando terminar para mostrar algo — um `go test ./...` demorado exibe nada até o fim, deixando o usuário no escuro.

Solução: prompt multi-linha (`shift+enter` quebra linha, `enter` envia) e stream do output do bash ao vivo no chat enquanto o comando roda.

## Objetivos

- **Legibilidade**: prompts longos digitados com quebras de linha naturais.
- **Feedback imediato**: output de comandos longos aparece linha a linha enquanto roda.
- **Produtividade**: histórico de prompts navegável com setas, como em shells convencionais.
- **Histórico enxuto**: o transcript continua gravando só o resultado consolidado.

## Histórias de Usuário

- Como Jev, quero quebrar linha com `shift+enter` enquanto escrevo um prompt longo, para organizá-lo em parágrafos legíveis.
- Como Jev, quero ver o output de um comando demorado aparecendo linha a linha, para saber que está progredindo (e o que está fazendo).
- Como Jev, quero recuperar prompts anteriores com ↑ quando o input está vazio, para reenviar variações sem redigitar.
- Como Jev, quero que comandos silenciosos longos continuem mostrando o spinner normalmente, sem bloco de output vazio.

## Funcionalidades Principais

### REQ-001 — Prompt multi-linha

O input passa a ser multi-linha:

- `shift+enter` quebra linha; `enter` envia (comportamento inverso ao padrão do componente, configurado explicitamente).
- Altura automática de 1 a 8 linhas conforme o conteúdo.
- O prompt box cresce com o conteúdo; o viewport nunca fica menor que 3 linhas.
- Estilo visual do cursor e do texto preservado (fundo/caixa iguais ao atual).

#### Critérios de Aceite

- `shift+enter` cria segunda linha no prompt; o envio preserva as quebras.
- O viewport nunca encolhe abaixo de 3 linhas com o prompt no máximo de altura.

---

### REQ-002 — Histórico de prompts

Setas ↑/↓ com o input vazio navegam os últimos 20 prompts enviados (mantidos em memória na TUI).

#### Critérios de Aceite

- ↑ com input vazio recupera o prompt anterior; ↓ avança na navegação.

---

### REQ-003 — Stream do output do bash

O tool `bash` emite cada linha de output (stdout e stderr) no momento em que é produzida, via callback de streaming — mantendo a execução síncrona consolidada como wrapper.

#### Critérios de Aceite

- Comando `for i in $(seq 1 10); do echo $i; sleep 0.3; done` mostra os números aparecendo um a um.
- A ordem e a contagem de linhas do stream são fidedignas ao output real.

---

### REQ-004 — Bloco de output ao vivo na TUI

- Cada linha de output acumula num bloco ao vivo sob a linha da tool `bash`, em cor de texto muted.
- Eventos de linha são agrupados com throttle (no máximo 1 evento a cada 50ms) para não inundar o loop da TUI.
- O bloco ao vivo mostra as últimas 15 linhas visíveis com contador de omitidas (`… +N lines`).
- O resultado final (tool result) substitui o bloco ao vivo pelo resultado consolidado, com o truncamento normal de hoje.

#### Critérios de Aceite

- O bloco ao vivo contém as linhas conforme saem; o resultado final o substitui.
- Comando silencioso longo mostra o spinner normalmente, sem bloco vazio.

---

### REQ-005 — Transcript enxuto

O transcript grava só o resultado final do comando (não cada linha streamada), para não inflar o JSONL.

#### Critérios de Aceite

- O JSONL não cresce com eventos por linha de output.

## Experiência do Usuário

- A transição single → multi-linha é imperceptível: quem digita como antes não nota mudança até usar `shift+enter`.
- O stream ao vivo transforma a espera de comandos longos em acompanhamento ativo.
- O bloco ao vivo é discreto (muted) e cede lugar ao resultado consolidado — o histórico final fica idêntico ao de hoje.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- Componente de textarea da mesma família de dependências atual (bubbles) — sem dependência nova de outro ecossistema.
- Bug conhecido a evitar: `strings.Builder` nunca por valor dentro do Model do Bubble Tea.
- Throttle de eventos com buffer dimensionado para não travar o agente em comandos verbosos.

## Fora de Escopo

- Renderização ANSI colorida do output streamado.
- Streaming de output de outros tools (só `bash`).
- Edição de texto colado multi-linha (tratamento especial de paste).
- Histórico de prompts persistido entre sessões.
- Interatividade com o comando em execução (stdin).
