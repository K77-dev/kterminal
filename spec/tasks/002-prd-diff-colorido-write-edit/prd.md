# PRD — Diff colorido para write/edit

## Visão Geral

No kterminal, o agente edita arquivos através das tools `write` e `edit`, todas auto-aprovadas (YOLO) por padrão, com modo `--confirm` opcional. Hoje, o resultado de uma edição aparece no chat como uma única linha truncada — é impossível saber o que realmente mudou sem abrir o arquivo manualmente. Isso mina a confiança no agente e cria fricção em cada edição.

Este PRD define a exibição de **diff unificado colorido** para toda edição de arquivo: depois da execução (sempre) e antes da aprovação (modo `--confirm`). O usuário primário é o Jev (Typesafe), desenvolvedor que usa o kterminal diariamente no seu próprio fluxo de trabalho.

## Objetivos

- **Transparência total**: toda edição de arquivo é visível como diff colorido no momento em que acontece.
- **Confiança na aprovação**: no modo `--confirm`, a decisão de aprovar/rejeitar é tomada com o diff completo à frente.
- **Produtividade**: eliminar idas manuais ao arquivo só para verificar o que o agente mudou.
- **Rastreabilidade**: o diff fica gravado no transcript da sessão para auditoria posterior.

## Histórias de Usuário

- Como Jev, quero ver um diff colorido de toda edição assim que o agente a executa, para saber exatamente o que mudou sem abrir o arquivo.
- Como Jev no modo `--confirm`, quero ver o diff completo antes de responder y/n, para decidir a aprovação com informação completa.
- Como Jev, quero o diff gravado no transcript da sessão, para auditar o histórico de mudanças depois.
- Como Jev, quero diffs truncados de forma legível quando o arquivo é grande, para não poluir o chat com centenas de linhas.

## Funcionalidades Principais

### REQ-001 — Diff pós-execução

Toda execução bem-sucedida das tools `write` e `edit` produz um diff de linhas (unificado) entre o conteúdo anterior e o novo do arquivo, exibido como bloco colorido no chat:

- `edit` em arquivo existente: diff entre conteúdo anterior e editado.
- `write` em arquivo existente: diff entre conteúdo anterior e o novo.
- `write` em arquivo novo: todas as linhas como adições (`+`).
- Edições que não mudam nada não renderizam bloco de diff vazio.

#### Critérios de Aceite

- Edit em arquivo existente mostra remoções e adições coloridas.
- Write em arquivo novo mostra todo o conteúdo como adição.
- Diff de arquivo sem mudanças não renderiza bloco vazio.

---

### REQ-002 — Diff antes da aprovação (modo `--confirm`)

No modo `--confirm`, a tela de confirmação exibe o diff completo da edição pendente antes da pergunta y/n, permitindo decisão informada.

#### Critérios de Aceite

- Modo `--confirm` exibe o diff antes da aprovação.
- O diff na confirmação reflete exatamente a edição que será aplicada.

---

### REQ-003 — Diff no transcript

O diff é gravado no evento JSONL do transcript como linhas já formatadas, permitindo reconstruir o que cada edição mudou sem reler os arquivos.

#### Critérios de Aceite

- Eventos de tool result de `write`/`edit` no transcript carregam as linhas do diff.

## Experiência do Usuário

- O diff renderiza como bloco no chat com as cores do tema opencode: linhas `+` em verde (`diffAdded: #4fd6be`), linhas `-` em vermelho (`diffRemoved: #c53b53`), linhas de contexto em cor de texto muted — estas duas cores novas precisam ser adicionadas à paleta do tema.
- Cada linha é prefixada por `+`, `-` ou espaço, como em um diff unificado convencional.
- Diffs longos são truncados no meio com indicador `… N more lines …`, mantendo no máximo ~40 linhas visíveis.
- O bloco de diff aparece imediatamente após a linha da tool, sem interação adicional.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- **Sem novas dependências**: o diff de linhas é implementado à mão (LCS clássico é suficiente — arquivos de código são pequenos o suficiente).
- O resultado das tools precisa carregar o diff além do output textual, propagando do registry até os eventos do agente e a TUI.
- Paleta do tema original do opencode como referência para as novas cores.

## Fora de Escopo

- Diff word-level (palavra a palavra dentro da linha).
- Syntax highlight dentro das linhas do diff.
- Diff de arquivos binários.
- Diff de mudanças feitas fora das tools `write`/`edit` (ex.: via `bash`).
- Edição interativa do diff pelo usuário (aceitar/rejeitar hunks individuais).
