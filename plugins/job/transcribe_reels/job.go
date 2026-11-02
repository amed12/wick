package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yogasw/wick/pkg/entity"
	"github.com/yogasw/wick/pkg/job"
	wickplugin "github.com/yogasw/wick/pkg/plugin"
)

// Config is editable from the Jobs page. Field names become snake_case keys.
type Config struct {
	BoardURL   string `wick:"required;default=http://127.0.0.1:4317;desc=Planban base URL."`
	BoardRepo  string `wick:"required;default=fathullah-instagram-learning;desc=Planban board repoId the cards live on."`
	MetaKey    string `wick:"default=analysisStatus;desc=Card metadata key that marks work for this job."`
	MetaValue  string `wick:"default=pending;desc=Value metadata_key must equal to be picked up."`
	YtDlp      string `wick:"required;default=/usr/local/bin/yt-dlp;desc=yt-dlp binary used to download the reel."`
	CookieFile string `wick:"required;default=/Users/achmadfathullah/ig-tools/cookies.txt;desc=Netscape cookie jar exported from the logged-in browser; yt-dlp needs it for saved media."`
	WhisperBin string `wick:"required;default=/Users/achmadfathullah/ig-tools/tools/whisper.cpp/build/bin/whisper-cli;desc=whisper.cpp CLI binary."`
	ModelPath  string `wick:"required;default=/Users/achmadfathullah/ig-tools/models/ggml-small.bin;desc=ggml model file. small is the quality/speed sweet spot."`
	Language   string `wick:"desc=Spoken language forced for transcription (id, en, …). Empty = auto-detect, which is the right default for a mixed library."`
	FFmpegBin  string `wick:"key=ffmpeg_bin;default=ffmpeg;desc=ffmpeg binary used to convert the download to 16 kHz mono wav."`
	MaxPerRun  int    `wick:"default=2;desc=How many reels one run processes. Keep it low: each minute of audio costs ~40s of CPU."`
	WorkDir    string `wick:"desc=Scratch directory for downloads. Default: the plugin data dir."`
}

// card is the slice of a Planban roadmap item this job needs.
type card struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	IsGroup  bool           `json:"isGroup"`
	Metadata map[string]any `json:"metadata"`
}

// Run is one execution of the job.
func Run(ctx context.Context) (string, error) {
	cfg := job.FromContext(ctx)
	repo := orDefault(cfg.Cfg("board_repo"), "fathullah-instagram-learning")
	base := strings.TrimRight(orDefault(cfg.Cfg("board_url"), "http://127.0.0.1:4317"), "/")
	if repo == "" || base == "" {
		return "", fmt.Errorf("board_url and board_repo are required")
	}
	metaKey := orDefault(cfg.Cfg("meta_key"), "analysisStatus")
	metaValue := orDefault(cfg.Cfg("meta_value"), "pending")
	max := cfg.CfgInt("max_per_run")
	if max <= 0 {
		max = 2
	}
	workDir := strings.TrimSpace(cfg.Cfg("work_dir"))
	if workDir == "" {
		workDir = filepath.Join(wickplugin.DataDir("transcribe_reels"), "work")
	}
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return "", fmt.Errorf("create work dir: %w", err)
	}

	cards, err := fetchCards(ctx, base, repo)
	if err != nil {
		return "", err
	}
	var todo []card
	for _, c := range cards {
		if c.IsGroup {
			continue
		}
		if asString(c.Metadata[metaKey]) == metaValue {
			todo = append(todo, c)
		}
	}
	if len(todo) == 0 {
		job.Logf(ctx, "nothing to do: no card carries %s=%s", metaKey, metaValue)
		return "Tidak ada reel berstatus `" + metaKey + "=" + metaValue + "`.", nil
	}
	if len(todo) > max {
		todo = todo[:max]
	}
	job.Logf(ctx, "%d card(s) to transcribe on %s", len(todo), repo)

	var lines []string
	done, failed := 0, 0
	for _, c := range todo {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		sourceURL := asString(c.Metadata["sourceUrl"])
		if sourceURL == "" {
			failed++
			lines = append(lines, fmt.Sprintf("- ⏭️ `%s` — tidak punya `sourceUrl`", c.ID))
			continue
		}
		job.Logf(ctx, "transcribing %s (%s)", c.ID, sourceURL)

		text, err := transcribe(ctx, cfg, workDir, c.ID, sourceURL)
		if err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- ❌ `%s` — %v", c.ID, err))
			mark(ctx, base, repo, c.ID, "failed: "+firstLine(err.Error()))
			continue
		}

		doc := fmt.Sprintf("# Transkrip — %s\n\nSumber: %s\nModel: %s · bahasa: %s · %s\n\n## Transkrip (otomatis, belum diverifikasi)\n\n%s\n",
			c.Title, sourceURL, filepath.Base(cfg.Cfg("model_path")), orAuto(cfg.Cfg("language")),
			time.Now().Format("2006-01-02 15:04"), text)
		if err := putDoc(ctx, base, repo, c.ID, doc); err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- ❌ `%s` — transkrip jadi tapi gagal menulis dokumen: %v", c.ID, err))
			continue
		}
		mark(ctx, base, repo, c.ID, "done")
		done++
		lines = append(lines, fmt.Sprintf("- ✅ `%s` — %d karakter transkrip", c.ID, len([]rune(text))))
	}

	return fmt.Sprintf("**Transkripsi selesai** — %d berhasil, %d gagal.\n\n%s",
		done, failed, strings.Join(lines, "\n")), nil
}

// transcribe downloads the reel and runs whisper.cpp on it.
func transcribe(ctx context.Context, cfg *job.Ctx, workDir, cardID, sourceURL string) (string, error) {
	media := filepath.Join(workDir, safeName(cardID)+".mp4")
	defer os.Remove(media)
	args := []string{"-q", "--no-warnings", "-f", "mp4/best", "-o", media}
	if cookies := orDefault(cfg.Cfg("cookie_file"), "/Users/achmadfathullah/ig-tools/cookies.txt"); cookies != "" {
		args = append(args, "--cookies", cookies)
	}
	args = append(args, sourceURL)
	if out, err := runBin(ctx, orDefault(cfg.Cfg("yt_dlp"), "/usr/local/bin/yt-dlp"), args...); err != nil {
		return "", fmt.Errorf("unduh reel: %w: %s", err, tail(out, 200))
	}
	if st, err := os.Stat(media); err != nil || st.Size() == 0 {
		return "", fmt.Errorf("unduhan kosong (cookies kedaluwarsa?)")
	}

	wav := strings.TrimSuffix(media, ".mp4") + ".wav"
	defer os.Remove(wav)
	if out, err := runBin(ctx, orDefault(cfg.Cfg("ffmpeg_bin"), "/usr/local/bin/ffmpeg"), "-hide_banner", "-loglevel", "error", "-y",
		"-i", media, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav); err != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", err, tail(out, 200))
	}

	prefix := strings.TrimSuffix(wav, ".wav")
	whisperArgs := []string{"-m", orDefault(cfg.Cfg("model_path"), "/Users/achmadfathullah/ig-tools/models/ggml-small.bin"), "-f", wav, "-otxt", "-of", prefix, "-np", "-nt"}
	if lang := strings.TrimSpace(cfg.Cfg("language")); lang != "" {
		whisperArgs = append(whisperArgs, "-l", lang)
	}
	if out, err := runBin(ctx, orDefault(cfg.Cfg("whisper_bin"), "/Users/achmadfathullah/ig-tools/tools/whisper.cpp/build/bin/whisper-cli"), whisperArgs...); err != nil {
		return "", fmt.Errorf("whisper: %w: %s", err, tail(out, 200))
	}
	raw, err := os.ReadFile(prefix + ".txt")
	if err != nil {
		return "", fmt.Errorf("baca hasil transkrip: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	os.Remove(prefix + ".txt")
	if text == "" {
		return "", fmt.Errorf("transkrip kosong (audio tanpa ucapan?)")
	}
	return text, nil
}

// ---------------------------------------------------------------- planban HTTP

func fetchCards(ctx context.Context, base, repo string) ([]card, error) {
	var out []card
	if err := pbDo(ctx, base, http.MethodGet, "/api/boards/"+repo+"/cards", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func putDoc(ctx context.Context, base, repo, cardID, markdown string) error {
	return pbDo(ctx, base, http.MethodPut, "/api/boards/"+repo+"/cards/"+cardID+"/docs/spec",
		map[string]any{"markdown": markdown}, nil)
}

// mark records the outcome on the card so the next run skips it. Planban's
// PATCH replaces the metadata object, so read the card first and merge —
// otherwise the flags this job writes would wipe instagramShortcode, sourceUrl
// and everything else the card already carries.
func mark(ctx context.Context, base, repo, cardID, state string) {
	var current card
	if err := pbDo(ctx, base, http.MethodGet, "/api/boards/"+repo+"/cards/"+cardID, nil, &current); err != nil {
		job.Logf(ctx, "warn: could not read %s before marking: %v", cardID, err)
		return
	}
	meta := map[string]any{}
	for k, v := range current.Metadata {
		meta[k] = v
	}
	meta["analysisStatus"] = state
	meta["analysisCheckedAt"] = time.Now().Format(time.RFC3339)
	if err := pbDo(ctx, base, http.MethodPatch, "/api/boards/"+repo+"/cards/"+cardID,
		map[string]any{"metadata": meta}, nil); err != nil {
		job.Logf(ctx, "warn: could not record %s on %s: %v", state, cardID, err)
	}
}

// pbDo is the Planban client. Mutations must state the Planban origin: the
// server refuses writes whose Origin/Sec-Fetch-Site it cannot recognise.
func pbDo(ctx context.Context, base, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", method, path, err)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", base)
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, tail(string(raw), 300))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

// ---------------------------------------------------------------- helpers

// orDefault keeps the job working when a config row is missing: a job plugin's
// manifest currently serialises Configs as null, so `default=` tags never reach
// the runtime and every empty read needs a usable fallback.
func orDefault(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}

func runBin(ctx context.Context, bin string, args ...string) (string, error) {
	if strings.TrimSpace(bin) == "" {
		return "", fmt.Errorf("binary path is empty")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func orAuto(s string) string {
	if strings.TrimSpace(s) == "" {
		return "auto"
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func safeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "reel"
	}
	return b.String()
}

// Module returns the job definition.
func Module() job.Module {
	return job.Module{
		Meta: job.Meta{
			Key:         "transcribe_reels",
			Name:        "Transcribe Saved Reels",
			Description: "Batch-transcribe saved Instagram reels onto their Planban cards: pick cards flagged in metadata, download the reel, run whisper.cpp locally, write the transcript into the card document.",
			Icon:        "🎙️",
			DefaultCron: "0 3 * * *",
		},
		Configs: entity.StructToConfigs(Config{}),
		Run:     Run,
	}
}
