# igdecoder

Biblioteca Go para capturar reels, vídeos e stories de perfis do Instagram a partir de uma conta logada, transcrever o áudio localmente e entregar o resultado em chunks (`{perfil, hora, transcript}`) para onde você apontar.

Feita para embutir em serviços (AWS, GCP, Coolify, Docker), não para rodar numa máquina específica. Sem browser em runtime, sem serviços de terceiros pagos, e núcleo com **zero dependências** — só a stdlib do Go.

## Como funciona

O app web do Instagram é HTTP. Com um cookie `sessionid` válido, a lib fala direto com a API interna via rotas REST `v1` — que, ao contrário dos `doc_id` de GraphQL, o Instagram não rotaciona a cada duas semanas. Sem Playwright/Chrome: a imagem de container fica pequena e o consumo de memória, baixo.

A transcrição é uma interface (`Transcriber`) que você injeta. A lib traz um backend pronto para o whisper.cpp; troque por qualquer outro sem tocar no resto.

## Requisitos

Go 1.23+. O núcleo da lib não tem dependências além da stdlib.

Binários externos, usados apenas pelo pipeline de transcrição:
- `ffmpeg` no PATH (extração de áudio)
- `whisper-cli` do [whisper.cpp](https://github.com/ggml-org/whisper.cpp) + um modelo `ggml` (só se usar o backend embutido)

### Escolha do modelo de transcrição

| modelo | disco | RAM | velocidade | português |
|---|---|---|---|---|
| `tiny` | 75 MB | ~400 MB | ~30x tempo real | ruim |
| `base` | 142 MB | ~500 MB | ~15x | **erra o idioma** |
| `small` | 466 MB | ~1 GB | ~7x | aceitável |
| `large-v3-turbo-q5_0` | 870 MB | ~2 GB | ~8x | boa |

Em teste real, `base` transcreveu um reel em português como se fosse inglês. Para
conteúdo em pt-BR use `large-v3-turbo` **e** fixe o idioma, em vez de confiar na
autodetecção:

```go
whispercpp.New(whispercpp.Config{Model: "...", Language: "pt"})
```

## Uso como lib

```go
package main

import (
	"context"
	"time"

	"github.com/hugoluizmtb/igdecoder"
	"github.com/hugoluizmtb/igdecoder/transcriber/whispercpp"
)

func main() {
	ctx := context.Background()

	sess, _ := igdecoder.SessionFromEnv()
	cli, _ := igdecoder.New(sess)

	tr := whispercpp.New(whispercpp.Config{
		Model:    "/models/ggml-large-v3-turbo-q5_0.bin",
		Language: "pt",
	})
	sink := igdecoder.WebhookSink{
		URL:    "https://seu-endpoint/ingest",
		Secret: "seu-hmac-secret",
	}

	medias, _ := cli.Capture(ctx, "perfil_alvo", igdecoder.Filter{
		Kinds: []igdecoder.Kind{igdecoder.Reel, igdecoder.Story},
		Since: 7 * 24 * time.Hour,
		Limit: 20,
	})

	for _, m := range medias {
		doc, err := cli.Transcribe(ctx, m, tr)
		if err != nil {
			continue
		}
		for _, p := range igdecoder.Chunks(doc, m, igdecoder.ChunkOptions{MaxTokens: 900, IncludeCaption: true}) {
			_ = sink.Deliver(ctx, p)
		}
	}
}
```

## API

### Capturar de um perfil

```go
cli.Capture(ctx, "perfil", igdecoder.Filter{Kinds: []igdecoder.Kind{igdecoder.Reel}, Since: 7 * 24 * time.Hour, Limit: 20})
cli.Reels(ctx, "perfil", 10)
cli.Stories(ctx, "perfil")
cli.Posts(ctx, "perfil", 10)
cli.Profile(ctx, "perfil")
```

`Reels` e `Posts` leem a mesma rota (`/api/v1/feed/user/`) e filtram por tipo: um
reel chega no feed com `product_type: "clips"`. A rota `/api/v1/clips/user/`
responde `401 require_login` para sessões web e por isso não é usada.

### Buscar uma mídia por URL

```go
m, err := cli.Media(ctx, "https://www.instagram.com/reel/CylsWHNrUdB/")
```

Aceita permalink (`/reel/`, `/reels/`, `/p/`, `/tv/`) ou o shortcode puro. O
shortcode é convertido em media id localmente, sem requisição extra:

```go
sc, _ := igdecoder.ParseShortcode(url)              // "CylsWHNrUdB"
id, _ := igdecoder.ShortcodeToMediaID(sc)           // "3217172542446716737"
```

Devolve `ErrBadPermalink` se a entrada não contiver um shortcode válido.

> **Aviso:** `Media` usa `/api/v1/media/{id}/info/`, que é rota do app mobile.
> Em testes com uma sessão web ela redirecionou para o login. Use `Capture`
> quando o perfil for conhecido; `Media` pode não funcionar com todas as sessões.

### Transcrever

```go
doc, err := cli.Transcribe(ctx, m, transcriber)      // baixa, extrai áudio, transcreve
doc, err := cli.TranscribeFile(ctx, "video.mp4", tr) // arquivo local, sem tocar no Instagram
path, err := cli.Download(ctx, m, "/tmp")            // só baixa
```

## Sessão

A lib **não** obtém o cookie sozinha — você fornece, tipicamente de um secret manager ou variável de ambiente.

Para obter uma vez, manualmente: logue no `instagram.com`, abra DevTools → Application → Cookies e copie `sessionid`, `ds_user_id` e `csrftoken`.

```go
sess := igdecoder.Session{SessionID: "...", UserID: "...", CSRFToken: "..."}
sess, _ = igdecoder.SessionFromCookieString("sessionid=...; ds_user_id=...; csrftoken=...")
sess, _ = igdecoder.SessionFromEnv()
```

Variáveis de ambiente lidas por `SessionFromEnv`:

| var | obrigatória |
|-----|-------------|
| `IGDEC_SESSIONID` | sim |
| `IGDEC_USERID` | não |
| `IGDEC_CSRF` | não |
| `IGDEC_USERAGENT` | não |
| `IGDEC_APPID` | não |

## Payload entregue

Um por chunk; todos com o mesmo `document_id`.

```json
{
  "schema_version": "1",
  "document_id": "a1b2c3...",
  "media_id": "a1b2c3...",
  "idempotency_key": "a1b2c3...:0",
  "profile": {"username": "perfil_alvo", "user_id": "456"},
  "kind": "reel",
  "taken_at": "2026-08-10T14:30:00Z",
  "permalink": "https://www.instagram.com/reel/ABC/",
  "caption": "...",
  "language": "pt",
  "duration_s": 42.3,
  "metrics": {"views": 12000, "likes": 800},
  "chunk": {"index": 0, "total": 3, "text": "...", "start": 0, "end": 18200000000, "tokens": 890},
  "captured_at": "2026-08-13T20:00:00Z"
}
```

`chunk.start` e `chunk.end` são `time.Duration` serializados em **nanossegundos** (18200000000 = 18,2 s); `duration_s` é em segundos. Campos vazios são omitidos (`omitempty`).

O `WebhookSink` assina o corpo com HMAC-SHA256 no header `X-Signature-256` e manda `Idempotency-Key`.

## CLI de exemplo

```sh
export IGDEC_SESSIONID=...
go run ./cmd/igdecoder -kinds reel,story -limit 10 perfil_alvo
IGDEC_WHISPER_MODEL=/models/ggml-large-v3-turbo-q5_0.bin \
  go run ./cmd/igdecoder -webhook https://seu-endpoint perfil_alvo
```

Sem `-model`, o CLI apenas emite os metadados das mídias capturadas. Com modelo, roda o pipeline completo (download → áudio → transcrição → chunks → sink).

## Docker

O `Dockerfile` compila o binário, compila o whisper.cpp com clang (gcc 12 falha
nos intrínsecos NEON fp16 em arm64) e baixa um modelo:

```sh
docker build -t igdecoder .
docker build --build-arg WHISPER_MODEL=base -t igdecoder:base .   # build rápido p/ testes
docker run --rm -e IGDEC_SESSIONID=... -e IGDEC_WEBHOOK=https://... igdecoder -kinds reel,story perfil_alvo
```

`WHISPER_MODEL` define o modelo baixado e a variável `IGDEC_WHISPER_MODEL` da
imagem final. Padrão: `large-v3-turbo-q5_0`.

## Erros

Ramifique com `errors.Is`:

- `ErrLoginRequired` — sessão inválida/expirada; renove o cookie.
- `ErrRateLimited` — recue (veja `HTTPError.RetryAfter`).
- `ErrChallenge` — checkpoint na conta; **requer ação humana, não repita**.
- `ErrNotFound`, `ErrPrivate`, `ErrNoMedia`, `ErrNoAudioTrack`, `ErrUnexpectedShape`, `ErrBadPermalink`, `ErrNilTranscriber`.

O Instagram responde `401` com `require_login: true` tanto para sessão expirada
quanto para limite temporário ("Aguarde alguns minutos"). A lib distingue os dois
pela mensagem: o segundo vira `ErrRateLimited`, para você recuar em vez de trocar
o cookie sem necessidade. Checkpoint tem prioridade sobre ambos.

## Ritmo e risco de bloqueio

O limite temporário do Instagram é fácil de disparar: algumas dezenas de
requisições em poucos minutos bastam. `WithPacing` existe por isso e o padrão
(2–6s) é conservador de propósito.

```go
igdecoder.New(sess, igdecoder.WithPacing(4*time.Second, 9*time.Second))
```

Trate `ErrRateLimited` como ordem de parar, não como erro para repetir em laço.
Use uma conta dedicada, nunca a principal.

## Avisos

Automatizar acesso viola os Termos de Uso do Instagram e pode levar a bloqueio da conta. Use uma conta dedicada, respeite o ritmo (`WithPacing`) e a lei/consentimento aplicáveis aos dados coletados. Esta lib é uma ferramenta; o uso é responsabilidade de quem a integra.

## Testes

```sh
go test ./...
go test -cover ./...
```

Os testes de `capture` usam um servidor HTTP fake injetado via `WithHTTPClient`; os de áudio geram fixtures com `ffmpeg` e são pulados se o binário não existir.

## Licença

[MIT](LICENSE)
