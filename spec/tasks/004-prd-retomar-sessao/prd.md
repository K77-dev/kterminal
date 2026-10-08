# PRD — Retomar sessão (`kterminal --continue`)

## Visão Geral

Toda sessão do kterminal já é gravada como transcript JSONL em `~/.local/share/kterminal/sessions/`, mas ao fechar o app a conversa se perde — não há resume (decisão deliberada do v0.1). O usuário precisa recomeçar do zero e re-explicar contexto ao agente.

Este PRD define `kterminal --continue`, que reabre a última sessão com o histórico completo restaurado e pronto para continuar a conversa, e `kterminal --session <caminho>`, que abre uma sessão específica. O usuário primário é o Jev (Typesafe), que frequentemente interrompe o trabalho e quer retomá-lo depois sem perder contexto.

## Objetivos

- **Continuidade**: retomar exatamente de onde parou, com o agente ciente de toda a conversa anterior.
- **Zero perda**: um snapshot do estado completo da conversa é gravado a cada turno.
- **Robustez**: falhas de resume produzem erros claros e acionáveis (nunca silenciosos, nunca crash).
- **Produtividade**: eliminar o custo de re-explicar contexto a cada reinício do app.

## Histórias de Usuário

- Como Jev, quero executar `kterminal --continue` e retomar a última conversa com o histórico completo, para continuar o trabalho sem re-explicar nada.
- Como Jev, quero abrir uma sessão específica com `kterminal --session <caminho>`, para voltar a uma conversa anterior específica.
- Como Jev, quero ver o histórico restaurado no viewport ao iniciar, para relembrar o contexto de olho.
- Como Jev, quero um aviso amigável quando não há sessão anterior, para entender que estou começando fresh sem achar que algo quebrou.

## Funcionalidades Principais

### REQ-001 — Snapshot por turno

Ao final de cada turno (e também em abort/erro pós-stream), o agente grava no transcript um evento snapshot com o array completo de mensagens da conversa serializado. O snapshot é a fonte da verdade do estado da conversa — não um replay de eventos granulares.

#### Critérios de Aceite

- Snapshot gravado após cada turno concluído.
- O snapshot contém o array de mensagens completo e fiel ao estado do agente.

---

### REQ-002 — Carregamento de sessão

O carregamento lê o arquivo de trás para frente e usa o **último** snapshot do arquivo. Arquivo sem snapshot produz erro explícito ("sessão sem snapshot").

#### Critérios de Aceite

- O resume usa o snapshot mais recente do arquivo.
- Arquivo com eventos mas sem snapshot → erro explícito.

---

### REQ-003 — Flags CLI

- `--continue`: carrega a sessão mais recente (ordenada por nome de arquivo). Sem sessões existentes: aviso no chat ("no previous session — starting fresh") e o app segue normalmente.
- `--session <caminho>`: carrega o arquivo indicado. Arquivo inexistente → erro claro no stderr e exit 1.

#### Critérios de Aceite

- Fluxo: conversar, sair, `kterminal --continue` → pergunta nova responde com contexto anterior.
- `--session` com arquivo inexistente → erro claro no stderr, exit 1.
- `--continue` sem sessões anteriores → aviso no chat e app funcional.

---

### REQ-004 — Sessão retomada é a mesma sessão

A sessão retomada continua gravando no **mesmo arquivo** de transcript (append), mantendo o histórico completo num lugar só. Ao iniciar com sessão carregada, a TUI reconstrói as últimas mensagens no viewport (user → bloco de usuário, assistant → markdown + linha de conclusão, tool → linha de tool) e exibe no hint bar `resumed · N mensagens`.

#### Critérios de Aceite

- Mensagens enviadas após o resume são gravadas no mesmo arquivo JSONL original.
- O viewport mostra o histórico reconstruído ao iniciar.
- O hint bar indica sessão resumida e a contagem de mensagens.

## Experiência do Usuário

- O resume é imediato: abrir o app com `--continue` mostra o histórico e o prompt pronto, sem passos intermediários.
- A reconstrução visual do histórico usa os mesmos blocos do chat ao vivo — sem nova linguagem visual.
- O hint bar sinaliza o estado `resumed` de forma discreta.

## Restrições Técnicas de Alto Nível

- Go 1.27, módulo `kterminal`; sem comentários no código.
- **Não reconstruir mensagens a partir dos eventos granulares** — só o snapshot é fonte da verdade. Requisito não negociável.
- A sessão retomada continua no mesmo arquivo de transcript.
- Mensagens carregadas precisam ser revalidadas contra o gateway (modelos/candidatos podem ter mudado entre sessões).

## Fora de Escopo

- Listagem interativa de sessões (picker).
- Busca ou filtragem em sessões antigas.
- Exportação de sessões para outros formatos.
- Múltiplas sessões simultâneas no mesmo processo.
- Fork de sessão (retomar e salvar em arquivo novo).
