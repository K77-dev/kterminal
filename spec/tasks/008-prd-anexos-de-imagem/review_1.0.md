# Relatório de Code Review - 008 Anexos de imagem: Task 1.0 (Content parts no `llm.Message`)

## Resumo
- Data: 2026-10-04
- Branch: 002-010-prds-kterminal
- Status: APROVADO
- Arquivos Modificados: 2 (`internal/llm/llm.go`, `internal/llm/llm_test.go`)
- Linhas Adicionadas: 262
- Linhas Removidas: 4

## Conformidade com Rules
| Rule | Status | Observações |
|------|--------|-------------|
| code-standards.md | OK | Nomenclatura exportada idiomática (`ImageURL`, `ContentPart`, `ContentParts`), gofmt limpo, sem comentários no código |
| architecture-ddd.md | N/A | Brownfield Go com `internal/` — a própria rule determina não impor DDD (conforme techspec) |
| tests.md / typescript.md / logging.md / database.md | N/A | Stack Go/stdlib; padrões do repositório (`testing`, AAA, golden) seguidos |
| Dependências | OK | Nenhuma dependência nova — apenas stdlib (`encoding/json`, `bytes` já importados) |

## Aderência à TechSpec
| Decisão Técnica | Implementado | Observações |
|-----------------|--------------|-------------|
| `ImageURL{URL string}` com `json:"url,omitempty"` | SIM | Idêntica à spec |
| `ContentPart{Type, Text, ImageURL *ImageURL}` com `text,omitempty` e `image_url,omitempty` | SIM | Idêntica à spec (incluindo ponteiro em `ImageURL`) |
| `Message.ContentParts []ContentPart` | SIM | Tag `json:"-"` (ver nota 2) |
| `MarshalJSON` com receiver por valor | SIM | `func (m Message) MarshalJSON() ([]byte, error)` conforme spec |
| `len(ContentParts) == 0` → JSON byte a byte igual ao atual | SIM | Golden capturado do output real do código pré-mudança e travado em teste (5 casos) |
| Com parts → `"content": [{"type":"text",...},{"type":"image_url","image_url":{"url":"data:<mime>;base64,..."}}]` | SIM | Testado byte a byte, ordem text→image preservada, múltiplas imagens na ordem |
| Interação techspec 004: "Load devolve parts intactas" (roundtrip) | SIM | Roundtrip marshal→unmarshal preserva `Role`, `Content`, `ContentParts`, `ToolCalls`, `ToolCallID` (ver nota 1) |
| `Content string` forte mantido (decisão 1: não usar `Content any`) | SIM | Callers existentes inalterados; 142 testes pré-existentes passam sem alteração |

### Notas de decisão
1. **`UnmarshalJSON` adicionado além da lista literal de requisitos** (que menciona apenas `MarshalJSON`): exigência derivada do requisito da task "Roundtrip marshal→unmarshal preserva campos (base para o snapshot da techspec 004)" e da techspec ("retomar sessão com imagem recupera as parts fielmente"; `session.Load` faz `json.Unmarshal` em `[]llm.Message`). Sem ele, unmarshal de `"content": [...]` em `Content string` falha e o teste especificado não passa com parts. Não é scope creep — é implementação necessária ao requisito. Segue a convenção de `json.Unmarshaler` (literal `null` = no-op, testado).
2. **Tag `json:"-"` em `ContentParts`** (não especificada na techspec): o campo nunca é chave JSON própria — é serializado como a chave `content` pelo marshaler custom. `-` garante que o alias do branch sem parts o exclua incondicionalmente, tornando a compatibilidade byte a byte independente de `omitempty`.

## Tasks Verificadas
| Task | Status | Observações |
|------|--------|-------------|
| 1.1 Definir `ContentPart` e `ImageURL` em `internal/llm/llm.go` | COMPLETA | Structs conforme spec |
| 1.2 Adicionar `ContentParts` ao `Message` e implementar `MarshalJSON` condicional | COMPLETA | Branch string via alias sem métodos (mesmas tags/ordem); branch array via struct espelhada |
| 1.3 Golden test + teste de parts + roundtrip | COMPLETA | 3 testes especificados + edge cases |

## Testes
- Total de Testes: 145 (142 pré-existentes + 3 novos)
- Passando: 145
- Falhando: 0
- Coverage: não medida (sem ferramenta de coverage configurada no projeto); compensada por asserções byte a byte e edge cases
- Extra: `go test -race ./internal/llm/` passa

### Cobertura dos testes da task (techspec — Testes Unidade, itens 1-3)
| Teste | Status | Cobertura |
|-------|--------|-----------|
| `TestMessageMarshalStringContentGolden` | PASS | 5 casos byte a byte: user simples, content vazio, escapes (`\n`, `\"`), assistant com `tool_calls`, tool com `tool_call_id` |
| `TestMessageMarshalContentParts` | PASS | text+image na ordem (data URI), múltiplas imagens na ordem, parts sobrepõem `Content` string, comportamento `omitempty` das tags |
| `TestMessageUnmarshalRoundtrip` | PASS | Roundtrip de 4 formas de mensagem (string, parts, tool_calls, tool_call_id) via `reflect.DeepEqual`; edge cases: content ausente, `content: null`, mensagem `null` (no-op), content numérico → erro, part inválida → erro |

## Verificação de Segurança
- Checklist de backend/API (CORS, auth, SQL, rate limiting): N/A — mudança é serialização de contrato de mensagem, sem endpoints.
- Sem secrets/keys no código: OK.
- UnmarshalJSON rejeita `content` de tipo inválido (número/objeto) e parts malformados — erros propagados, não silenciados: OK.
- Privacidade (REQ-005): o marshaler apenas serializa; não grava logs nem transcript. O risco de base64 no snapshot (documentado na techspec) é mitigação da task 4.0 (placeholder), fora do escopo aqui.

## Problemas Encontrados
| Severidade | Arquivo | Linha | Descrição | Sugestão |
|------------|---------|-------|-----------|----------|
| — | — | — | Nenhum problema bloqueante encontrado | — |

## Pontos Positivos
- Golden capturado do output real do código pré-mudança (via teste temporário descartado), não por assumption — prova objetiva de compatibilidade byte a byte (REQ-001 duro).
- Compatibilidade estruturalmente garantida: branch sem parts delega ao `encoding/json` via alias com tags/ordem idênticas; branch com parts espelha a mesma ordem de campos (`role`, `content`, `tool_calls`, `tool_call_id`).
- Roundtrip cobre exatamente o mecanismo usado pelo snapshot da feature 004 (`json.Marshal`/`json.Unmarshal` em `[]llm.Message`), sem alterar `session.go`.
- Convenção de `json.Unmarshaler` (null = no-op) respeitada e travada por teste.
- Edge cases além do caminho feliz: content ausente/null, tipos inválidos, `omitempty`, override de `Content` por parts.

## Recomendações
- Task 4.0: implementar o placeholder de snapshot (`{"type":"image_url","image_url":{"url":"[omitted]"}}` no `WriteSnapshot`) conforme mitigação de risco da techspec — o base64 em `ContentParts` fluirá para o JSONL de snapshot até lá (limitação já documentada na techspec).
- Task 4.0: validar o request ao gateway com content parts (teste de integração 12 da techspec).

## Conclusão
Implementação aderente à techspec e à task: structs idênticas à spec, `MarshalJSON` condicional com compatibilidade byte a byte provada por golden test, formato de parts OpenAI-compatible testado byte a byte, e roundtrip completo (base do snapshot 004) com `UnmarshalJSON` — adição necessária ao requisito de roundtrip, documentada nas notas. Todos os checks passam (build, vet, gofmt, 145/145 testes, race). Nenhum problema bloqueante. **APROVADO.**
