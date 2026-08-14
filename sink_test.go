package igdecoder

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebhookSinkDelivers(t *testing.T) {
	var gotBody []byte
	var gotSig, gotIdem, gotDoc, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("X-Signature-256")
		gotIdem = r.Header.Get("Idempotency-Key")
		gotDoc = r.Header.Get("X-Igdecoder-Document")
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sink := WebhookSink{URL: srv.URL, Secret: "s3cr3t"}
	p := Payload{DocumentID: "doc", MediaID: "doc", IdempotencyKey: "doc:0"}
	if err := sink.Deliver(context.Background(), p); err != nil {
		t.Fatalf("Deliver: %v", err)
	}

	if gotIdem != "doc:0" {
		t.Errorf("Idempotency-Key = %q", gotIdem)
	}
	if gotDoc != "doc" {
		t.Errorf("X-Igdecoder-Document = %q", gotDoc)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}

	mac := hmac.New(sha256.New, []byte("s3cr3t"))
	mac.Write(gotBody)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if gotSig != want {
		t.Errorf("assinatura = %q, queria %q", gotSig, want)
	}

	var decoded Payload
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("corpo não é json: %v", err)
	}
	if decoded.DocumentID != "doc" {
		t.Errorf("corpo document_id = %q", decoded.DocumentID)
	}
}

func TestWebhookSinkNoSecretOmitsSignature(t *testing.T) {
	var hadSig bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadSig = r.Header["X-Signature-256"]
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if err := (WebhookSink{URL: srv.URL}).Deliver(context.Background(), Payload{}); err != nil {
		t.Fatal(err)
	}
	if hadSig {
		t.Error("sem Secret não deveria assinar")
	}
}

func TestWebhookSinkErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	err := (WebhookSink{URL: srv.URL}).Deliver(context.Background(), Payload{})
	if err == nil {
		t.Fatal("esperava erro em 500")
	}
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 500 {
		t.Errorf("erro = %v", err)
	}
}

func TestWebhookSinkEmptyURL(t *testing.T) {
	if err := (WebhookSink{}).Deliver(context.Background(), Payload{}); err == nil {
		t.Error("URL vazia deveria falhar")
	}
}

func TestFuncSink(t *testing.T) {
	called := false
	var s Sink = FuncSink(func(_ context.Context, _ Payload) error {
		called = true
		return nil
	})
	if err := s.Deliver(context.Background(), Payload{}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("FuncSink não invocou a função")
	}
}

func TestMultiSinkFanout(t *testing.T) {
	n := 0
	inc := FuncSink(func(_ context.Context, _ Payload) error {
		n++
		return nil
	})
	if err := (MultiSink{inc, inc, inc}).Deliver(context.Background(), Payload{}); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("entregou a %d sinks, queria 3", n)
	}
}

func TestMultiSinkReturnsFirstError(t *testing.T) {
	boom := errors.New("boom")
	ok := FuncSink(func(_ context.Context, _ Payload) error { return nil })
	bad := FuncSink(func(_ context.Context, _ Payload) error { return boom })
	err := (MultiSink{ok, bad, ok}).Deliver(context.Background(), Payload{})
	if !errors.Is(err, boom) {
		t.Errorf("erro = %v, queria boom", err)
	}
}
