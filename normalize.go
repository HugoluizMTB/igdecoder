package igdecoder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func stableID(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(sum[:])[:24]
}

func looksLikeMedia(m jmap) bool {
	if tn, _ := m["__typename"].(string); strings.Contains(tn, "Graph") &&
		(strings.Contains(tn, "Video") || strings.Contains(tn, "Image") || strings.Contains(tn, "Sidecar")) {
		return true
	}
	hasID := firstKey(m, "pk", "id", "code", "shortcode") != nil
	if !hasID {
		return false
	}
	return firstKey(m,
		"media_type", "video_versions", "image_versions2", "video_url",
		"display_url", "taken_at", "taken_at_timestamp", "carousel_media",
	) != nil
}

func collectMediaNodes(root any) []jmap {
	var out []jmap
	seen := map[*struct{}]bool{}
	_ = seen

	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 30 || v == nil {
			return
		}
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e, depth+1)
			}
		case jmap:

			if inner, ok := asMap(x["media"]); ok && looksLikeMedia(inner) {
				walk(inner, depth+1)
				for _, key := range containerKeys {
					if c, ok := x[key]; ok {
						walk(c, depth+1)
					}
				}
				return
			}
			if looksLikeMedia(x) {
				out = append(out, x)
				return
			}
			for _, e := range x {
				walk(e, depth+1)
			}
		}
	}
	walk(root, 0)
	return out
}

var containerKeys = []string{"edges", "items", "reels_media", "tray", "carousel_media"}

func normNode(node jmap, kindHint Kind, owner Profile, engine string) []Media {
	kind := inferKind(node, kindHint)
	own := extractOwner(node, owner)

	if children := carouselChildren(node); len(children) > 0 {
		caption := extractCaption(node)
		code := digStr(node, "code")
		if code == "" {
			code = digStr(node, "shortcode")
		}
		var out []Media
		for i, ch := range children {
			childPK := mediaPK(ch)
			for _, m := range normNode(ch, kind, own, engine) {
				if m.Caption == "" {
					m.Caption = caption
				}
				if m.Shortcode == "" {
					m.Shortcode = code
				}
				m.ID = stableID(string(m.Kind), code, itoa(i), childPK)
				m.Permalink = permalink(m.Kind, code, own, childPK)
				out = append(out, m)
			}
		}
		if len(out) > 0 {
			return out
		}
	}

	pk := mediaPK(node)
	code := digStr(node, "code")
	if code == "" {
		code = digStr(node, "shortcode")
	}

	videoURL, vw, vh := bestVideoURL(node)
	imageURL, iw, ih := bestImageURL(node)

	dur := firstFloat(node, "video_duration", "duration")
	if dur > 3600 {
		dur /= 1000
	}

	title, artist := extractAudio(node)

	m := Media{
		Kind:        kind,
		Owner:       own,
		TakenAt:     epochToTime(firstKey(node, "taken_at", "taken_at_timestamp", "device_timestamp")),
		ExpiresAt:   epochToTime(node["expiring_at"]),
		Shortcode:   code,
		Caption:     extractCaption(node),
		Duration:    time.Duration(dur * float64(time.Second)),
		Width:       pickInt(vw, iw, int(digInt(node, "original_width"))),
		Height:      pickInt(vh, ih, int(digInt(node, "original_height"))),
		HasAudio:    toBool(firstKey(node, "has_audio")) || videoURL != "",
		Views:       firstInt(node, "play_count", "view_count", "video_view_count", "ig_play_count"),
		Likes:       firstInt(node, "like_count"),
		Comments:    firstInt(node, "comment_count"),
		AudioTitle:  title,
		AudioArtist: artist,
		VideoURL:    videoURL,
		ImageURL:    imageURL,
	}
	m.ID = stableID(string(kind), orStr(pk, code))
	m.Permalink = permalink(kind, code, own, pk)
	if raw, err := json.Marshal(node); err == nil {
		m.Raw = raw
	}
	return []Media{m}
}

func mediaPK(node jmap) string {
	pk := digStr(node, "pk")
	if pk == "" {
		pk = digStr(node, "id")
	}
	if i := strings.IndexByte(pk, '_'); i >= 0 {
		pk = pk[:i]
	}
	return pk
}

func inferKind(node jmap, hint Kind) Kind {
	if node["expiring_at"] != nil {
		if hint == Highlight {
			return Highlight
		}
		return Story
	}
	product := strings.ToLower(digStr(node, "product_type"))
	typename := digStr(node, "__typename")
	mediaType := digInt(node, "media_type")
	isVideo := node["video_versions"] != nil || node["video_url"] != nil ||
		mediaType == 2 || strings.Contains(typename, "Video") || toBool(node["is_video"])

	switch {
	case product == "clips":
		return Reel
	case product == "story":
		return Story
	case hint == Reel && isVideo:
		return Reel
	case isVideo:
		if hint == Story || hint == Highlight {
			return hint
		}
		return Video
	case mediaType == 1 || strings.Contains(typename, "Image") || node["display_url"] != nil:
		if hint == Story || hint == Highlight {
			return hint
		}
		return Image
	}
	if hint != "" {
		return hint
	}
	return Unknown
}

func extractOwner(node jmap, fallback Profile) Profile {
	u, _ := asMap(firstKey(node, "user", "owner"))
	if u == nil {
		return fallback
	}
	username := digStr(u, "username")
	if username == "" {
		username = fallback.Username
	}
	p := Profile{
		Username:  strings.ToLower(username),
		UserID:    orStr(digStr(u, "pk"), digStr(u, "id")),
		FullName:  digStr(u, "full_name"),
		Verified:  toBool(u["is_verified"]),
		Private:   toBool(u["is_private"]),
		PicURL:    digStr(u, "profile_pic_url"),
		Followers: firstInt(u, "follower_count"),
	}
	if p.UserID == "" {
		p.UserID = fallback.UserID
	}
	if p.Followers == 0 {
		p.Followers = fallback.Followers
	}
	return p
}

func extractCaption(node jmap) string {
	if c, ok := asMap(node["caption"]); ok {
		return cleanText(digStr(c, "text"))
	}
	if s, ok := node["caption"].(string); ok {
		return cleanText(s)
	}

	if edges, ok := asSlice(dig(node, "edge_media_to_caption", "edges")); ok && len(edges) > 0 {
		return cleanText(digStr(edges[0], "node", "text"))
	}
	for _, k := range []string{"caption_text", "title", "text"} {
		if s := digStr(node, k); s != "" {
			return cleanText(s)
		}
	}
	return ""
}

func extractAudio(node jmap) (title, artist string) {
	meta, _ := asMap(node["clips_metadata"])
	if meta != nil {
		if a, ok := asMap(dig(meta, "music_info", "music_asset_info")); ok {
			if t := digStr(a, "title"); t != "" {
				return t, orStr(digStr(a, "display_artist"), digStr(a, "artist_name"))
			}
		}
		if o, ok := asMap(meta["original_sound_info"]); ok {
			if t := digStr(o, "original_audio_title"); t != "" {
				return t, digStr(o, "ig_artist", "username")
			}
		}
	}
	return "", ""
}

func carouselChildren(node jmap) []jmap {
	if cs, ok := asSlice(node["carousel_media"]); ok {
		return toMaps(cs)
	}
	if edges, ok := asSlice(dig(node, "edge_sidecar_to_children", "edges")); ok {
		var out []jmap
		for _, e := range edges {
			if n, ok := asMap(dig(e, "node")); ok {
				out = append(out, n)
			}
		}
		return out
	}
	return nil
}

func bestVideoURL(node jmap) (url string, w, h int) {
	if vs, ok := asSlice(node["video_versions"]); ok {
		return bestByArea(vs)
	}
	if s := digStr(node, "video_url"); s != "" {
		return s, 0, 0
	}
	return "", 0, 0
}

func bestImageURL(node jmap) (url string, w, h int) {
	if cs, ok := asSlice(dig(node, "image_versions2", "candidates")); ok {
		return bestByArea(cs)
	}
	if s := firstStr(node, "display_url", "thumbnail_url", "thumbnail_src"); s != "" {
		return s, 0, 0
	}
	return "", 0, 0
}

func bestByArea(versions []any) (url string, w, h int) {
	bestArea := -1
	for _, v := range versions {
		m, ok := asMap(v)
		if !ok {
			continue
		}
		u := digStr(m, "url")
		if u == "" {
			continue
		}
		vw, vh := int(digInt(m, "width")), int(digInt(m, "height"))
		if area := vw * vh; area > bestArea {
			bestArea, url, w, h = area, u, vw, vh
		}
	}
	return url, w, h
}

func permalink(kind Kind, code string, owner Profile, pk string) string {
	const base = "https://www.instagram.com"
	switch kind {
	case Story, Highlight:
		if owner.Username != "" && pk != "" {
			return base + "/stories/" + owner.Username + "/" + pk + "/"
		}
		return ""
	}
	if code == "" {
		return ""
	}
	slug := "p"
	if kind == Reel {
		slug = "reel"
	}
	return base + "/" + slug + "/" + code + "/"
}

func nextCursor(root any) string {
	var cursor string
	walkMaps(root, func(m jmap) bool {
		for _, key := range []string{"page_info", "paging_info"} {
			if pi, ok := asMap(m[key]); ok {
				if toBool(pi["has_next_page"]) || toBool(pi["more_available"]) {
					if c := orStr(digStr(pi, "end_cursor"), digStr(pi, "max_id")); c != "" {
						cursor = c
						return false
					}
				}
			}
		}
		if c := firstStr(m, "next_max_id"); c != "" {
			cursor = c
			return false
		}
		return true
	})
	return cursor
}

func firstFloat(m jmap, keys ...string) float64 { return toFloat(firstKey(m, keys...)) }
func firstInt(m jmap, keys ...string) int64     { return toInt64(firstKey(m, keys...)) }
func firstStr(m jmap, keys ...string) string    { return toString(firstKey(m, keys...)) }

func toMaps(s []any) []jmap {
	out := make([]jmap, 0, len(s))
	for _, e := range s {
		if m, ok := asMap(e); ok {
			out = append(out, m)
		}
	}
	return out
}

func pickInt(vals ...int) int {
	for _, v := range vals {
		if v > 0 {
			return v
		}
	}
	return 0
}

func orStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func itoa(i int) string { return toString(float64(i)) }

func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSpace(s)
	return s
}
