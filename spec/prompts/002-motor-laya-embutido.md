# Motor Laya Embutido

## Descrição

Infraestrutura de inferência da **Laya** (https://github.com/NandhaKishorM/laya) rodando **in-process dentro do binário Go do kterminal** — sem servidor externo, sem Python em runtime. A Laya é um decision engine System 1 não-autoregressivo: responde perguntas tipadas (`choice`, `score`, `noul`) sobre um state em uma única forward pass (~33ms) com probabilidades calibradas. Este módulo entrega o motor puro: download de artefatos, carga de checkpoints, tokenização, inferência e resposta — sem qualquer acoplamento com o roteamento do kterminal.

## Requisitos Funcionais

- [RF-01] Inferência via bindings Go para ONNX Runtime (ex.: `github.com/yalue/onnxruntime_go`), consumindo os checkpoints ONNX oficiais da Laya (opset 18, pesos em external-data sidecar). Tokenização (HuggingFace `tokenizers`, mesmo `tokenizer.json` dos checkpoints) e calibration/temperaturas reimplementados em Go puro. Sem cgo/Rust.
- [RF-02] Suporte aos **três checkpoints**: `multilingual` (mmBERT-base, 322M, 1024-token, 100+ idiomas), `english` (ModernBERT-large, 421M, 512-token) e `typed-decisions` (ModernBERT-large, 421M, 1024-token, fine-tuned para decisões tipadas).
- [RF-03] Roteamento interno entre checkpoints replicando o `Router` da Laya: detecção de idioma do state → inglês vai para `english`, demais idiomas para `multilingual`; decisões tipadas podem rotear para `typed-decisions` (equivalente ao `LAYA_AUTO_TASK`).
- [RF-03] Download no primeiro uso, **bloqueante com progresso visível na TUI**: checkpoints + lib nativa do ONNX Runtime (`.so`/`.dylib`/`.dll` por plataforma) baixados juntos para o cache. Usuário não instala nada manualmente.
- [RF-04] Cache em `~/.config/kterminal/laya/` (junto da config do kterminal), com verificação de integridade (hash/size) e re-download em caso de corrupção.
- [RF-05] API do motor: função `Predict(state string, questions map[string]Question) (answers, error)` com tipos `Question{Type, Instructions, Criteria}` e resposta com `Choice`, `Confidence`, `Probabilities` — espelhando o contrato do cliente Jev (`jev.Client.Decide`) para que o módulo 003 seja fino.
- [RF-06] Verificação de sanidade no startup com engine=laya: artefatos ausentes/corrompidos → mensagem clara instruindo o download (o download bloqueante do RF-03 cobre o primeiro uso; o RF-06 cobre deleção manual posterior).
- [RF-07] Isolamento por trás de uma porta (interface pequena) — o resto do kterminal só vê a API do RF-05, nunca ONNX/tokenizer/detalhes de download.

## Dependências

- Nenhuma de outros módulos (infraestrutura pura). É pré-requisito do 003.

## Observações

- **Risco técnico principal da feature**: a reimplementação Go do pipeline (tokenização + calibration) deve reproduzir o comportamento do SDK oficial — usar os goldens do `laya-rust` (testes verificados contra o Python) como referência de correção.
- Checkpoints têm centenas de MB: o download precisa ser resumível e mostrar progresso; falha de rede → erro claro, sem estado corrompido no cache.
- A lib do ONNX Runtime é baixada junto dos checkpoints (mesmo fluxo), não empacotada no binário.
- Fora de escopo: interface `Router` do kterminal (003), seleção de engine (001), telemetria (004), fine-tuning, GPU/execution providers além de CPU.
