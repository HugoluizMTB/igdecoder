package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hugoluizmtb/igdecoder"
	"github.com/hugoluizmtb/igdecoder/transcriber/whispercpp"
)

func main() {
	kindsArg := flag.String("kinds", "reel,story", "tipos: reel,story,video,image,highlight")
	since := flag.Duration("since", 7*24*time.Hour, "idade máxima das mídias")
	limit := flag.Int("limit", 10, "máximo de itens por tipo")
	model := flag.String("model", os.Getenv("IGDEC_WHISPER_MODEL"), "caminho do modelo ggml do whisper.cpp")
	whisperBin := flag.String("whisper", envOr("IGDEC_WHISPER_BIN", "whisper-cli"), "binário do whisper.cpp")
	lang := flag.String("lang", os.Getenv("IGDEC_LANG"), "idioma (vazio = autodetecção)")
	webhook := flag.String("webhook", os.Getenv("IGDEC_WEBHOOK"), "URL do webhook de destino")
	maxTokens := flag.Int("max-tokens", 900, "tokens por chunk")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "uso: igdecoder [flags] <username> [username...]")
		flag.PrintDefaults()
		os.Exit(2)
	}

	sess, err := igdecoder.SessionFromEnv()
	if err != nil {
		fatal(err)
	}
	cli, err := igdecoder.New(sess)
	if err != nil {
		fatal(err)
	}

	var tr igdecoder.Transcriber
	if *model != "" {
		tr = whispercpp.New(whispercpp.Config{Binary: *whisperBin, Model: *model, Language: *lang})
	}

	sink := selectSink(*webhook)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	filter := igdecoder.Filter{Kinds: parseKinds(*kindsArg), Since: *since, Limit: *limit}
	enc := json.NewEncoder(os.Stdout)

	for _, user := range flag.Args() {
		medias, err := cli.Capture(ctx, user, filter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "capture %s: %v\n", user, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "%s: %d mídias capturadas\n", user, len(medias))

		for _, m := range medias {
			if tr == nil {
				_ = enc.Encode(m)
				continue
			}
			if !m.IsVideo() {
				continue
			}
			doc, err := cli.Transcribe(ctx, m, tr)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  transcribe %s: %v\n", m.ID, err)
				continue
			}
			payloads := igdecoder.Chunks(doc, m, igdecoder.ChunkOptions{
				MaxTokens: *maxTokens, IncludeCaption: true,
			})
			for _, p := range payloads {
				if err := sink.Deliver(ctx, p); err != nil {
					fmt.Fprintf(os.Stderr, "  deliver %s: %v\n", p.IdempotencyKey, err)
				}
			}
		}
	}
}

func selectSink(webhook string) igdecoder.Sink {
	if webhook != "" {
		return igdecoder.WebhookSink{
			URL:     webhook,
			Secret:  os.Getenv("IGDEC_WEBHOOK_SECRET"),
			Timeout: 30 * time.Second,
		}
	}
	enc := json.NewEncoder(os.Stdout)
	return igdecoder.FuncSink(func(_ context.Context, p igdecoder.Payload) error {
		return enc.Encode(p)
	})
}

func parseKinds(s string) []igdecoder.Kind {
	var ks []igdecoder.Kind
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			ks = append(ks, igdecoder.Kind(p))
		}
	}
	return ks
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "erro:", err)
	os.Exit(1)
}
