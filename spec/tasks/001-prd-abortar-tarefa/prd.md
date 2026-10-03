# PRD — Interromper tarefa em execução (Esc)

## Visão Geral

O kterminal é um terminal agêntico em Go (Bubble Tea) onde o Jev (Typesafe) escolhe o LLM a cada chamada, via um gateway LiteLLM OpenAI-compatible. Hoje, uma vez iniciado um turno agêntico (stream LLM, execução de tools, chamadas de roteamento), não existe forma de cancelá-lo: o usuário precisa aguardar a conclusão ou encerrar o app inteiro com `ctrl+c`, perdendo a sessão.

Esta funcionalidade permite interromper a tarefa corrente com `Esc` sem sair do app: aborta o stream LLM em curso, mata o tool em execução (inclusive processos `bash`) e retorna ao estado idle mantendo a conversa e o texto parcial já recebido. É valioso para o usuário principal (o Jev/operador do terminal) porque evita desperdício de tokens e tempo em respostas que já se mostraram inúteis, e devolve o controle sem custo de perda de sessão.

## Objetivos

- Permitir abortar o turno agêntico corrente com uma única tecla (`Esc`), sem encerrar o app.
- Latência de abort ≤ 100ms entre o `Esc` e a TUI voltar a aceitar input; processos `bash` em execução mortos no mesmo instante via contexto.
- Preservar integralmente a conversa após o abort: texto parcial visível no chat, transcript da sessão gravado, nova mensagem funciona normalmente.
- Nenhum erro espúrio apresentado ao usuário: cancelamentos de contexto nunca aparecem como `EventError`.

## Histórias de Usuário

- Como operador do kterminal, eu quero pressionar `Esc` durante o streaming de uma resposta para que o stream pare imediatamente e eu volte a digitar, sem perder o que já foi renderizado.
- Como operador, eu quero pressionar `Esc` enquanto um comando `bash` longo executa (ex.: `sleep 60`) para que o processo seja morto e o turno abortado, economizando tempo.
- Como operador com confirmação de tools ativada, eu quero pressionar `Esc` no prompt de confirmação de um tool mutante para que a tarefa inteira seja cancelada, em vez de responder tool por tool.
- Como operador, eu quero que o texto parcial recebido até o abort permaneça no chat com um marcador visual de interrupção, para que eu saiba distinguir resposta completa de resposta interrompida.
- Como operador, eu quero enviar uma nova mensagem logo após um abort para que o ciclo agêntico reinicie limpo, sem resíduos do turno cancelado.

Usuário secundário: quem revisa a sessão depois (transcript) — precisa distinguir turnos abortados de turnos concluídos no arquivo de sessão.

## Funcionalidades Principais

### REQ-001 — Cancelamento do turno agêntico via Esc

Introduz um mecanismo de cancelamento no `Agent`: cada execução de `Run()` cria um contexto cancelável derivado de `context.Background()`, propagado a todo o ciclo — `ChatStream`, execução de tools e chamadas de roteamento (Jev). Um método público `Cancel()` dispara o cancelamento. Ao cancelar, o loop emite o novo evento `EventTurnAborted` (com o texto parcial acumulado), grava no transcript (`session.Event{Type: "turn_aborted"}`) e encerra o loop. Erros decorrentes do cancelamento do contexto NÃO são emitidos como `EventError`.

Importante porque é o núcleo da funcionalidade: sem propagação de contexto até o HTTP do gateway e o `exec.CommandContext` do bash, o abort não seria efetivo.

Requisitos funcionais:
1. `Agent` expõe `Cancel()`; o contexto cancelável cobre todo o ciclo do turno.
2. O registry de tools propaga o contexto para os tools (o `bash` o respeita; tools de filesystem falham rápido naturalmente).
3. Cancelamento emite `EventTurnAborted` com o texto parcial e registra `turn_aborted` no transcript.
4. Nenhum `EventError` é emitido por cancelamento de contexto.
5. O texto parcial NÃO entra no histórico de mensagens enviado ao LLM no próximo turno (fica visível no chat e no transcript, mas é descartado do contexto do modelo).

#### Critérios de Aceite

- Esc durante stream: stream para, `EventTurnAborted` emitido, TUI volta a aceitar input, conversa preservada.
- Esc durante `bash` longo (ex.: `sleep 60`): processo morto via contexto.
- Nova mensagem após abort funciona normalmente (loop reinicia limpo, sem o parcial no contexto LLM).
- Nenhuma mensagem de erro exibida ao usuário em decorrência do cancelamento.

---

### REQ-002 — Tecla Esc na TUI e estados

Mapeia a tecla `Esc` no estado de chat: quando o agente está ocupado (`busy`), `Esc` chama `Cancel()`; quando não está ocupado, não faz nada (comportamento atual preservado). Quando o agente aguarda confirmação de tool mutante (estado `confirm`), `Esc` cancela a tarefa inteira: responde o canal de confirmação como recusa e aborta o turno. `ctrl+c` continua encerrando o app em todos os estados.

Importante porque define o contrato de interação preciso, evitando ambiguidade entre cancelar tarefa e sair do app.

Requisitos funcionais:
1. `Esc` com agente ocupado cancela o turno corrente.
2. `Esc` com agente ocioso não tem efeito.
3. `Esc` no prompt de confirmação recusa o tool pendente e cancela o turno inteiro.
4. `ctrl+c` mantém o comportamento atual: encerra o app.

#### Critérios de Aceite

- `Esc` com `busy=false` não altera nenhum estado.
- `Esc` no estado `confirm` resulta em turno abortado (não apenas tool recusado com loop continuando).
- `ctrl+c` encerra o app como hoje.

---

### REQ-003 — Renderização do turno abortado e feedback visual

O texto parcial streamado permanece no chat, renderizado como markdown (mesmo tratamento do `EventTurnDone`), acrescido de um marcador visual de interrupção (ex.: rótulo "interrompido") que o diferencia de uma resposta completa. Após o abort, o status volta a idle (`busy = false`) e a TUI aceita input. Enquanto ocupado, a hint bar exibe `esc to interrupt` no lado direito.

Importante porque o usuário precisa confiar no que vê: distinguir resposta completa de parcial, e saber que pode interromper.

Requisitos funcionais:
1. Texto parcial renderizado como markdown com marcador visual de interrupção.
2. Status retorna a idle após o abort; input habilitado.
3. Hint bar mostra `esc to interrupt` (lado direito) enquanto `busy`.

#### Critérios de Aceite

- Turno abortado exibe marcador visual distinto de turno concluído.
- Hint bar alterna corretamente entre ocupado (`esc to interrupt`) e ocioso.
- Latência entre `Esc` e a TUI aceitar input ≤ 100ms.

## Experiência do Usuário

Persona primária: o operador (Jev/Typesafe) interagindo com o terminal agêntico em seu fluxo de desenvolvimento.

Fluxo principal: usuário envia mensagem → agente inicia turno (stream/tools) → hint bar mostra `esc to interrupt` → usuário pressiona `Esc` → stream/tool aborta → texto parcial permanece no chat com marcador "interrompido" → TUI volta a idle → usuário digita nova mensagem normalmente.

Fluxo de confirmação: agente pede confirmação de tool mutante → usuário pressiona `Esc` → tool recusado + turno abortado → mesmo desfecho do fluxo principal.

Casos extremos: `Esc` quando ocioso (sem efeito); `Esc` repetido durante o mesmo turno (idempotente — segundo `Esc` não causa erro); abort no meio de chamada de roteamento (Jev) também cancela.

Acessibilidade: o marcador de interrupção deve ser textual (não apenas cor), legível no conteúdo plano copiável da TUI. O hint `esc to interrupt` segue o padrão visual existente da hint bar.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`. Sem comentários no código.
- Seguir os padrões existentes: eventos via canal, TUI com receivers por valor, `strings.Builder` sempre por ponteiro.
- Integração com gateway LiteLLM OpenAI-compatible existente (`ChatStream` já aceita contexto; HTTP aborta com cancelamento).
- Verificação obrigatória: `go build ./... && go vet ./... && gofmt -l . && go test ./...`.
- Teste novo em `internal/agent/agent_test.go`: mock de gateway cujo handler bloqueia até o contexto do request morrer; chamar `Run` e depois `Cancel`; assertar `EventTurnAborted` e ausência de goroutine vazando (com timeout).
- Detalhes de implementação (estrutura do cancelável, propagação no registry, ordem de emissão de eventos) pertencem à Tech Spec.

## Fora de Escopo

- Pausar e retomar a tarefa (só abortar; retomar de onde parou é futuro).
- Cancelar seletivamente um tool específico mantendo o turno vivo.
- Desfazer/rollback de efeitos de tools já executados antes do abort.
- Customização da tecla de interrupção (remapeamento de keybindings).
- Reenvio automático ou edição do turno abortado a partir do transcript.
