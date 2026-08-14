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

O `Dockerfile` compila o binário, compila o whisper.cpp e baixa um modelo:

```sh
docker build -t igdecoder .
docker run --rm -e IGDEC_SESSIONID=... -e IGDEC_WEBHOOK=https://... igdecoder -kinds reel,story perfil_alvo
```

## Erros

Ramifique com `errors.Is`:

- `ErrLoginRequired` — sessão inválida/expirada; renove o cookie.
- `ErrRateLimited` — recue (veja `HTTPError.RetryAfter`).
- `ErrChallenge` — checkpoint na conta; requer ação humana.
- `ErrNotFound`, `ErrPrivate`, `ErrNoMedia`, `ErrUnexpectedShape`.

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
