---
name: media
description: "Handle images, audio, and document files for messaging channels"
trigger: automatic
---

# Media Tools

Process and send media files (images, audio, documents) through messaging channels like WhatsApp.

## ⚠️ REGRAS CRÍTICAS

| Regra | Motivo |
|-------|--------|
| **SEMPRE** use `send_document` para enviar PDFs | Criar arquivo ≠ enviar |
| **SEMPRE** use `send_image` para enviar imagens | Não apenas criar |
| Use `caption` para contexto | Ajuda usuário entender o arquivo |
| Verifique se arquivo existe antes de enviar | Evita erros |

### ❌ Errado
```
Usuário: "Me envia o PDF"
Agente: bash(command="ls /tmp/arquivo.pdf")  ❌
Agente: "O PDF está em /tmp/arquivo.pdf"     ❌ (não enviou!)
```

### ✓ Correto
```
Usuário: "Me envia o PDF"
Agente: send_document(                        ✓
  document_path="/tmp/arquivo.pdf",
  caption="Lista de compras",
  filename="lista.pdf"
)
Agente: "Enviado!"                            ✓
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Agent Context                          │
└──────────────────────────┬──────────────────────────────────┘
                           │
        ┌──────────────────┴──────────────────┐
        │                                     │
        ▼                                     ▼
┌───────────────┐                    ┌───────────────┐
│  INPUT MEDIA  │                    │ OUTPUT MEDIA  │
├───────────────┤                    ├───────────────┤
│describe_image │                    │  send_image   │
│transcribe_audio                    │  send_audio   │
└───────┬───────┘                    │ send_document │
        │                            └───────┬───────┘
        │                                    │
        ▼                                    ▼
┌───────────────┐                    ┌───────────────┐
│  AI Analysis  │                    │   Messaging   │
│  (Vision/STT) │                    │   Channel     │
└───────────────┘                    └───────────────┘
```

## Tools

| Tool | Action | Use When |
|------|--------|----------|
| `describe_image` | Analyze image with AI vision | User sends image |
| `transcribe_audio` | Convert audio to text | User sends voice message |
| `send_image` | **SEND** image to user | User needs visual response |
| `send_audio` | **SEND** audio file | User needs audio response |
| `send_document` | **SEND** PDF/document | User needs document |

## Input Processing

### Image Analysis

```bash
describe_image(image_path="/tmp/photo.jpg")
# Output:
# The image shows a green fern plant in a white ceramic pot.
# The plant appears healthy with several fronds.
```

### Audio Transcription

```bash
transcribe_audio(audio_path="/tmp/voice.ogg")
# Output:
# "Olá, gostaria de saber sobre a entrega."
```

## Sending Media (OUTPUT)

### ⚠️ CRÍTICO: Sempre Envie!

Quando usuário pedir para enviar arquivo, USE a ferramenta de envio:

### Send Document (PDF, DOC, etc)

```bash
send_document(
  document_path="/tmp/report.pdf",
  caption="Relatório Mensal - Fevereiro 2026",
  filename="relatorio-fev-2026.pdf"
)
# Output: Document sent successfully
```

### Send Image

```bash
send_image(
  image_path="/tmp/chart.png",
  caption="Gráfico de vendas do mês"
)
# Output: Image sent successfully
```

### Send Audio

```bash
send_audio(
  audio_path="/tmp/response.mp3",
  caption="Resposta em áudio"
)
# Output: Audio sent successfully
```

## Supported Formats

| Type | Formats | Max Size |
|------|---------|----------|
| Images | PNG, JPG, JPEG, GIF, WEBP | 5MB |
| Audio | MP3, OGG, WAV, M4A | 16MB |
| Documents | PDF, DOC, DOCX, XLS, XLSX | 100MB |

## Common Patterns

### Generate and Send PDF
```bash
# 1. Gerar PDF
bash(command="python3 scripts/create_pdf.py --output /tmp/lista.pdf")

# 2. Verificar se criou
bash(command="ls -lh /tmp/lista.pdf")

# 3. ENVIAR (não apenas informar que existe!)
send_document(
  document_path="/tmp/lista.pdf",
  caption="Lista de Compras",
  filename="lista.pdf"
)

# 4. Confirmar
send_message("PDF enviado!")
```

### Generate and Send Chart
```bash
# 1. Gerar visualização
bash(command="python scripts/generate_chart.py --output /tmp/sales.png")

# 2. ENVIAR
send_image(
  image_path="/tmp/sales.png",
  caption="Vendas por categoria - Últimos 30 dias"
)
```

### User Sends Image → Analyze
```bash
# User sends photo

# 1. Analyze
describe_image(image_path="/tmp/plant.jpg")

# 2. Respond based on analysis
send_message("Essa é uma Samambaia! Ela gosta de luz indireta e rega quando o topo da terra estiver seco.")
```

### User Sends Voice → Transcribe
```bash
# User sends voice message

# 1. Transcribe
transcribe_audio(audio_path="/tmp/voice.ogg")

# 2. Process and respond
send_message("Entendi! Vou verificar isso para você.")
```

## Workflow Completo: Criar e Enviar PDF

```bash
# Usuário: "Gera um PDF dessa lista e me envia"

# PASSO 1: Criar o PDF
bash(command="python3 << 'PYEOF'
from fpdf import FPDF
pdf = FPDF()
pdf.add_page()
pdf.set_font('Arial', 'B', 16)
pdf.cell(0, 10, 'Lista de Compras', 0, 1, 'C')
pdf.set_font('Arial', '', 12)
items = ['Papel higiênico', 'Peito de frango', 'Ovos', 'Tomates', 'Coentro']
for item in items:
    pdf.cell(0, 10, f'• {item}', 0, 1)
pdf.output('/tmp/lista.pdf')
PYEOF")

# PASSO 2: Verificar criação (opcional)
bash(command="ls -lh /tmp/lista.pdf")
# Output: -rw-r--r-- 1 user user 1.7K Feb 24 14:36 /tmp/lista.pdf

# PASSO 3: ENVIAR (CRÍTICO!)
send_document(
  document_path="/tmp/lista.pdf",
  caption="Lista de Compras",
  filename="lista-compras.pdf"
)
# Output: Document sent successfully

# PASSO 4: Confirmar
send_message("PDF enviado! 📄")
```

## Troubleshooting

### "File not found"

**Causa:** Arquivo não existe no caminho especificado.

**Debug:**
```bash
# Verificar se arquivo existe
bash(command="ls -la /tmp/arquivo.pdf")
```

### Arquivo não chegou no WhatsApp

**Causa:** Não usou `send_document`/`send_image`.

**Solução:**
```bash
# ❌ Errado - apenas verifica
bash(command="ls /tmp/arquivo.pdf")

# ✓ Correto - envia
send_document(document_path="/tmp/arquivo.pdf", caption="...")
```

### "Unsupported format"

**Causa:** Formato não suportado pelo canal.

**Solução:**
```bash
# Converter formato
bash(command="ffmpeg -i input.wav output.mp3")
send_audio(audio_path="/tmp/output.mp3")
```

### "File too large"

**Causa:** Excede limite do canal.

**Solução:**
```bash
# Comprimir
bash(command="gs -sDEVICE=pdfwrite -dPDFSETTINGS=/ebook -o small.pdf large.pdf")
send_document(document_path="/tmp/small.pdf", caption="...")
```

## Tips

- **Sempre use send_* tools**: Criar arquivo não é enviar
- **Adicione caption**: Dá contexto ao arquivo
- **Use filename claro**: Facilita identificação
- **Verifique tamanho**: Arquivos grandes podem falhar
- **Comprima se necessário**: Envio mais rápido

## Common Mistakes

| Erro | Correção |
|------|----------|
| Apenas criar arquivo, não enviar | Usar `send_document`/`send_image` |
| Sem caption | Adicionar descrição no caption |
| Caminho errado | Verificar com `ls` antes |
| Formato não suportado | Converter para formato aceito |
| Arquivo muito grande | Comprimir antes de enviar |
