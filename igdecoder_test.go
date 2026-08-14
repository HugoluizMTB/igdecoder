package igdecoder

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mustJSON(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("fixture inválida: %v", err)
	}
	return v
}

func TestNormalizeReel(t *testing.T) {
	resp := mustJSON(t, `{
		"items": [{"media": {
			"pk": "123", "id": "123_456", "code": "ABC",
			"media_type": 2, "taken_at": 1700000000,
			"video_versions": [{"url": "https://cdn.example/v.mp4", "width": 720, "height": 1280}],
			"video_duration": 12.5,
			"caption": {"text": "minha legenda"},
			"user": {"pk": "456", "username": "Fulano"},
			"play_count": 1000, "like_count": 50
		}}],
		"paging_info": {"max_id": "", "more_available": false}
	}`)

	prof := Profile{Username: "fulano", UserID: "456"}
	got := normalizeAll(resp, Reel, prof)
	if len(got) != 1 {
		t.Fatalf("esperava 1 mídia, veio %d", len(got))
	}
	m := got[0]
	if m.Kind != Reel {
		t.Errorf("kind = %q, queria reel", m.Kind)
	}
	if m.Owner.Username != "fulano" {
		t.Errorf("owner = %q", m.Owner.Username)
	}
	if m.VideoURL != "https://cdn.example/v.mp4" {
		t.Errorf("video url = %q", m.VideoURL)
	}
	if !m.IsVideo() {
		t.Error("IsVideo() = false")
	}
	if m.Caption != "minha legenda" {
		t.Errorf("caption = %q", m.Caption)
	}
	if m.Views != 1000 {
		t.Errorf("views = %d", m.Views)
	}
	if m.TakenAt.IsZero() {
		t.Error("taken_at zerado")
	}
	if m.ID == "" {
		t.Error("id vazio")
	}
}

func TestNormalizeStory(t *testing.T) {
	resp := mustJSON(t, `{
		"reels": {"456": {"items": [{
			"pk": "999", "id": "999_456", "media_type": 2,
			"taken_at": 1700000100, "expiring_at": 1700086500,
			"video_versions": [{"url": "https://cdn.example/s.mp4", "width": 720, "height": 1280}],
			"user": {"pk": "456", "username": "fulano"}
		}]}}
	}`)
	got := normalizeAll(resp, Story, Profile{Username: "fulano", UserID: "456"})
	if len(got) != 1 {
		t.Fatalf("esperava 1 story, veio %d", len(got))
	}
	if got[0].Kind != Story {
		t.Errorf("kind = %q, queria story", got[0].Kind)
	}
	if got[0].ExpiresAt.IsZero() {
		t.Error("expires_at zerado")
	}
}

func TestNormalizeCarousel(t *testing.T) {
	resp := mustJSON(t, `{"items": [{"media": {
		"pk": "1", "code": "CAR", "media_type": 8,
		"taken_at": 1700000000,
		"user": {"pk": "7", "username": "fulano"},
		"carousel_media": [
			{"pk": "1a", "media_type": 2, "video_versions": [{"url": "https://cdn/a.mp4", "width": 640, "height": 640}]},
			{"pk": "1b", "media_type": 1, "image_versions2": {"candidates": [{"url": "https://cdn/b.jpg", "width": 640, "height": 640}]}}
		]
	}}]}`)
	got := normalizeAll(resp, "", Profile{Username: "fulano"})
	if len(got) != 2 {
		t.Fatalf("carrossel deveria virar 2 itens, veio %d", len(got))
	}
	ids := map[string]bool{}
	for _, m := range got {
		if ids[m.ID] {
			t.Errorf("id duplicado: %s", m.ID)
		}
		ids[m.ID] = true
	}
}

func TestChunksSplitsLongText(t *testing.T) {
	text := strings.Repeat("uma frase de teste. ", 100)
	doc := Transcript{Text: text, Language: "pt"}
	m := Media{
		ID:      "media-1",
		Kind:    Reel,
		Owner:   Profile{Username: "fulano", UserID: "1"},
		TakenAt: time.Unix(1700000000, 0).UTC(),
	}
	ps := Chunks(doc, m, ChunkOptions{MaxTokens: 20})
	if len(ps) < 2 {
		t.Fatalf("texto longo deveria gerar vários chunks, veio %d", len(ps))
	}
	for i, p := range ps {
		if p.Chunk.Index != i {
			t.Errorf("chunk %d com index %d", i, p.Chunk.Index)
		}
		if p.Chunk.Total != len(ps) {
			t.Errorf("total = %d, queria %d", p.Chunk.Total, len(ps))
		}
		if p.Profile.Username != "fulano" {
			t.Errorf("profile não propagado: %q", p.Profile.Username)
		}
		if p.DocumentID != "media-1" {
			t.Errorf("document id = %q", p.DocumentID)
		}
		if p.IdempotencyKey == "" {
			t.Error("idempotency key vazio")
		}
		if p.Language != "pt" {
			t.Errorf("language = %q", p.Language)
		}
	}
}

func TestChunksSingleForShortText(t *testing.T) {
	doc := Transcript{Text: "curto e direto", Language: "pt"}
	m := Media{ID: "x", Kind: Reel, Owner: Profile{Username: "f"}}
	ps := Chunks(doc, m, ChunkOptions{MaxTokens: 900})
	if len(ps) != 1 {
		t.Fatalf("texto curto deveria ser 1 chunk, veio %d", len(ps))
	}
	if ps[0].Chunk.Total != 1 {
		t.Errorf("total = %d", ps[0].Chunk.Total)
	}
}

func TestChunksEmptyReturnsNil(t *testing.T) {
	if ps := Chunks(Transcript{}, Media{ID: "x"}, ChunkOptions{}); ps != nil {
		t.Errorf("transcript vazio deveria dar nil, veio %d", len(ps))
	}
}

func TestSessionFromCookieString(t *testing.T) {
	s, err := SessionFromCookieString("sessionid=abc123; ds_user_id=456; csrftoken=tok")
	if err != nil {
		t.Fatal(err)
	}
	if s.SessionID != "abc123" || s.UserID != "456" || s.CSRFToken != "tok" {
		t.Errorf("parse errado: %+v", s)
	}
}

func TestSessionValidate(t *testing.T) {
	if err := (Session{}).Validate(); err == nil {
		t.Error("sessão vazia deveria falhar validação")
	}
	if err := (Session{SessionID: "x"}).Validate(); err != nil {
		t.Errorf("sessão com sessionid deveria validar: %v", err)
	}
}
