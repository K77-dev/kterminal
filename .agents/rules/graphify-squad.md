---
name: graphify-squad
description: Consultar o knowledge graph do projeto (Graphify) antes de amostrar código com read/glob/grep.
disciplines: [architecture, backend, frontend, database, qa, test]
---

# Graphify: grafo de conhecimento do projeto

Se `graphify-out/graph.json` existir na raiz do projeto, consulte o grafo como ferramenta primária de mapeamento de código, **antes** de amostrar arquivos com `read`/`glob`/`grep`:

- **Personas do squad** (tool `graphify`): `{"command": "query", "question": "..."}` para contexto amplo, `{"command": "path", "source": "A", "target": "B"}` para impacto entre componentes, `{"command": "explain", "node": "X"}` para a explicação de um nó.
- **Agente principal** (com `bash`): CLI `graphify query "..."` / `graphify path "A" "B"` / `graphify explain "X"`.

Diretrizes:

- Tratar edges `EXTRACTED` como fato; `INFERRED`/`AMBIGUOUS` como hipótese — confirmar via `read` antes de afirmar.
- Se a tool/CLI responder que não há grafo ou está indisponível, seguir com `read`/`glob`/`grep` normalmente — nunca bloquear o fluxo por isso.
- Citar os `file:line` retornados pelo grafo ao fundamentar análises e propostas.
