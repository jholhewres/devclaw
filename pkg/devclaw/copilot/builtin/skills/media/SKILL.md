---
name: media
description: "Handle images, audio, and document files for messaging channels"
trigger: on-demand
---

# Media Tools

Process and send media files (images, audio, documents) through messaging channels like WhatsApp.

## ⚠️ REGRAS CRÍTICAS

| Regra | Motivo |
|-------|--------|
| **SEMPRE** use `send_media` para enviar arquivos | Criar arquivo ≠ enviar |
| Use `type` para forçar tipo (image/audio/document) | Auto-detecta se omitido |
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
Agente: send_media(                           ✓
  file_path="/tmp/arquivo.pdf",
  type="document",
  caption="Lista de compras"
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
│describe_image │                    │  send_media   │
│transcribe_audio                    │ (unified tool)│
└───────┬───────┘                    └───────┬───────┘
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
| `send_media` | **SEND** any media to user | User needs file (image, audio, document) |

## Input Processing

### Image Analysis

Images sent by the user are automatically analyzed by the media enrichment pipeline.
The `describe_image` tool accepts `image_base64` (base64-encoded data), not file paths:

```bash
describe_image(image_base64="<base64 data>", prompt="What is in this image?")
# Output:
# The image shows a green fern plant in a white ceramic pot.
# The plant appears healthy with several fronds.
```

### Audio Transcription

Audio messages are automatically transcribed by the media enrichment pipeline.
The `transcribe_audio` tool accepts `audio_base64` (base64-encoded data):

```bash
transcribe_audio(audio_base64="<base64 data>", filename="voice.ogg")
# Output:
# "Olá, gostaria de saber sobre a entrega."
```

## Sending Media (OUTPUT)

### ⚠️ CRÍTICO: Sempre Envie!

Quando usuário pedir para enviar arquivo, USE `send_media`:

### Send Document (PDF, DOC, etc)

```bash
send_media(
  file_path="/tmp/report.pdf",
  type="document",
  caption="Relatório Mensal - Fevereiro 2026"
)
# Output: Document sent successfully
```

### Send Image

```bash
send_media(
  file_path="/tmp/chart.png",
  type="image",
  caption="Gráfico de vendas do mês"
)
# Output: Image sent successfully
```

### Send Audio

```bash
send_media(
  file_path="/tmp/response.mp3",
  type="audio",
  caption="Resposta em áudio"
)
# Output: Audio sent successfully
```

### Auto-detect Type (omit type param)

```bash
send_media(
  file_path="/tmp/photo.jpg",
  caption="Foto da entrega"
)
# type auto-detected as "image" from MIME type
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

# 3. ENVIAR (CRÍTICO!)
send_media(
  file_path="/tmp/lista.pdf",
  type="document",
  caption="Lista de Compras"
)

# 4. Confirmar
send_message("PDF enviado!")
```

### Generate and Send Chart
```bash
# 1. Gerar visualização
bash(command="python scripts/generate_chart.py --output /tmp/sales.png")

# 2. ENVIAR
send_media(
  file_path="/tmp/sales.png",
  type="image",
  caption="Vendas por categoria - Últimos 30 dias"
)
```

### User Sends Image → Analyze
```bash
# User sends photo → automatically enriched by pipeline
# The image description is added to the message context

# Agent sees the description and responds:
send_message("Essa é uma Samambaia! Ela gosta de luz indireta e rega quando o topo da terra estiver seco.")
```

### User Sends Voice → Transcribe
```bash
# User sends voice message → automatically transcribed by pipeline
# The transcription is added to the message context

# Agent sees the transcription and responds:
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
send_media(
  file_path="/tmp/lista.pdf",
  type="document",
  caption="Lista de Compras"
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

**Causa:** Não usou `send_media`.

**Solução:**
```bash
# ❌ Errado - apenas verifica
bash(command="ls /tmp/arquivo.pdf")

# ✓ Correto - envia
send_media(file_path="/tmp/arquivo.pdf", type="document", caption="...")
```

### "Unsupported format"

**Causa:** Formato não suportado pelo canal.

**Solução:**
```bash
# Converter formato
bash(command="ffmpeg -i input.wav output.mp3")
send_media(file_path="/tmp/output.mp3", type="audio")
```

### "File too large"

**Causa:** Excede limite do canal.

**Solução:**
```bash
# Comprimir
bash(command="gs -sDEVICE=pdfwrite -dPDFSETTINGS=/ebook -o small.pdf large.pdf")
send_media(file_path="/tmp/small.pdf", type="document", caption="...")
```

## Tips

- **Sempre use send_media**: Criar arquivo não é enviar
- **Adicione caption**: Dá contexto ao arquivo
- **Use type quando souber**: Evita erros de auto-detecção
- **Verifique tamanho**: Arquivos grandes podem falhar
- **Comprima se necessário**: Envio mais rápido

## Common Mistakes

| Erro | Correção |
|------|----------|
| Apenas criar arquivo, não enviar | Usar `send_media` |
| Sem caption | Adicionar descrição no caption |
| Caminho errado | Verificar com `ls` antes |
| Formato não suportado | Converter para formato aceito |
| Arquivo muito grande | Comprimir antes de enviar |
