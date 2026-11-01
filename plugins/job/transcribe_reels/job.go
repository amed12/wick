package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yogasw/wick/pkg/entity"
	"github.com/yogasw/wick/pkg/job"
	wickplugin "github.com/yogasw/wick/pkg/plugin"
	"github.com/yogasw/wick/pkg/plugin/host"
	"github.com/yogasw/wick/pkg/plugin/proto/hostpb"
)

// Config is editable from the Jobs page. Field names become snake_case keys.
type Config struct {
	BoardRepo  string `wick:"required;default=fathullah-instagram-learning;desc=Planban board repoId the cards live on."`
	MetaKey    string `wick:"default=analysisStatus;desc=Card metadata key that marks work for this job."`
	MetaValue  string `wick:"default=pending;desc=Value metadata_key must equal to be picked up."`
	WhisperBin string `wick:"required;default=/Users/achmadfathullah/ig-tools/tools/whisper.cpp/build/bin/whisper-cli;desc=whisper.cpp CLI binary."`
	ModelPath  string `wick:"required;default=/Users/achmadfathullah/ig-tools/models/ggml-small.bin;desc=ggml model file. small is the quality/speed sweet spot; base is ~2x faster."`
	Language   string `wick:"default=id;desc=Spoken language forced for transcription (id, en, …). Empty = auto-detect."`
	FFmpegBin  string `wick:"default=ffmpeg;desc=ffmpeg binary used to convert the download to 16 kHz mono wav."`
	MaxPerRun  int    `wick:"default=2;desc=How many reels one run processes. Keep it low: each minute of audio costs ~40s of CPU."`
	WorkDir    string `wick:"desc=Scratch directory for downloads. Default: the plugin data dir (survives temp cleanup)."`
}

// Run is one execution of the job.
func Run(ctx context.Context) (string, error) {
	cfg := job.FromContext(ctx)
	repo := strings.TrimSpace(cfg.Cfg("board_repo"))
	if repo == "" {
		return "", fmt.Errorf("board_repo is not set")
	}
	metaKey := strings.TrimSpace(cfg.Cfg("meta_key"))
	metaValue := strings.TrimSpace(cfg.Cfg("meta_value"))
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

	h, err := wickplugin.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("host api: %w", err)
	}

	planban, err := firstInstance(ctx, h, "planban")
	if err != nil {
		return "", err
	}
	cardRes, err := execConnector(ctx, h, "planban", planban, "find_cards", map[string]any{
		"repo_id":        repo,
		"metadata_key":   metaKey,
		"metadata_value": metaValue,
		"limit":          max,
	})
	if err != nil {
		return "", err
	}
	cards, _ := cardRes["cards"].([]any)
	if len(cards) == 0 {
		job.Logf(ctx, "nothing to do: no card on %s has %s=%s", repo, metaKey, metaValue)
		return "Tidak ada reel berstatus `" + metaKey + "=" + metaValue + "`.", nil
	}
	job.Logf(ctx, "%d card(s) to transcribe on %s", len(cards), repo)

	ig, err := firstInstance(ctx, h, "instagram")
	if err != nil {
		return "", err
	}

	var lines []string
	done, failed, skipped := 0, 0, 0
	for _, raw := range cards {
		card, _ := raw.(map[string]any)
		id := str(card["id"])
		url := str(card["source_url"])
		title := str(card["title"])
		if id == "" || url == "" {
			skipped++
			lines = append(lines, fmt.Sprintf("- ⏭️ `%s` — tidak punya source_url", id))
			continue
		}
		if err := ctx.Err(); err != nil {
			return "", err
		}
		job.Logf(ctx, "processing %s (%s)", id, url)

		post, err := execConnector(ctx, h, "instagram", ig, "post", map[string]any{"url": url})
		if err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- ❌ `%s` — gagal ambil media: %v", id, err))
			mark(ctx, h, "planban", planban, repo, id, "failed: "+firstLine(err.Error()))
			continue
		}
		media := str(post["video_url"])
		if media == "" {
			skipped++
			lines = append(lines, fmt.Sprintf("- ⏭️ `%s` — bukan video", id))
			mark(ctx, h, "planban", planban, repo, id, "skipped:no-video")
			continue
		}

		text, err := transcribe(ctx, cfg, workDir, id, media)
		if err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- ❌ `%s` — %v", id, err))
			mark(ctx, h, "planban", planban, repo, id, "failed: "+firstLine(err.Error()))
			continue
		}

		doc := fmt.Sprintf("# Transkrip — %s\n\nSumber: %s\nModel: %s · bahasa: %s · %s\n\n## Transkrip (otomatis, belum diverifikasi)\n\n%s\n",
			title, url, filepath.Base(cfg.Cfg("model_path")), orAuto(cfg.Cfg("language")), time.Now().Format("2006-01-02 15:04"), text)
		if _, err := execConnector(ctx, h, "planban", planban, "write_doc", map[string]any{
			"repo_id": repo, "card_id": id, "kind": "spec", "markdown": doc,
		}); err != nil {
			failed++
			lines = append(lines, fmt.Sprintf("- ❌ `%s` — transkrip jadi tapi gagal menulis dokumen: %v", id, err))
			continue
		}
		meta, _ := json.Marshal(map[string]any{metaKey: "done", "analyzedAt": time.Now().Format(time.RFC3339)})
		if _, err := execConnector(ctx, h, "planban", planban, "set_metadata", map[string]any{
			"repo_id": repo, "card_id": id, "metadata": string(meta),
		}); err != nil {
			job.Logf(ctx, "warn: %s transcribed but metadata not updated: %v", id, err)
		}
		done++
		lines = append(lines, fmt.Sprintf("- ✅ `%s` — %d karakter transkrip", id, len([]rune(text))))
	}

	summary := fmt.Sprintf("**Transkripsi selesai** — %d berhasil, %d gagal, %d dilewati.\n\n%s",
		done, failed, skipped, strings.Join(lines, "\n"))
	return summary, nil
}

// transcribe downloads the media and runs whisper.cpp on it.
func transcribe(ctx context.Context, cfg *job.Ctx, workDir, cardID, mediaURL string) (string, error) {
	media, err := download(ctx, mediaURL, workDir, cardID)
	if err != nil {
		return "", fmt.Errorf("unduh media: %w", err)
	}
	defer os.Remove(media)

	wav := strings.TrimSuffix(media, filepath.Ext(media)) + ".wav"
	defer os.Remove(wav)
	if out, err := runBin(ctx, cfg.Cfg("ffmpeg_bin"), "-hide_banner", "-loglevel", "error", "-y",
		"-i", media, "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", wav); err != nil {
		return "", fmt.Errorf("ffmpeg: %w: %s", err, tail(out, 200))
	}

	prefix := strings.TrimSuffix(media, filepath.Ext(media))
	args := []string{"-m", cfg.Cfg("model_path"), "-f", wav, "-otxt", "-of", prefix, "-np", "-nt"}
	if lang := strings.TrimSpace(cfg.Cfg("language")); lang != "" {
		args = append(args, "-l", lang)
	}
	if out, err := runBin(ctx, cfg.Cfg("whisper_bin"), args...); err != nil {
		return "", fmt.Errorf("whisper: %w: %s", err, tail(out, 200))
	}
	raw, err := os.ReadFile(prefix + ".txt")
	if err != nil {
		return "", fmt.Errorf("baca hasil transkrip: %w", err)
	}
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return "", fmt.Errorf("transkrip kosong (audio tanpa ucapan?)")
	}
	return text, nil
}

func download(ctx context.Context, url, dir, name string) (string, error) {
	req, err := httpRequest(ctx, url)
	if err != nil {
		return "", err
	}
	resp, err := httpDo(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	ext := ".mp4"
	path := filepath.Join(dir, safeName(name)+ext)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := ioCopy(ctx, f, resp.Body); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

func mark(ctx context.Context, h host.Host, connector, instance, repo, cardID, state string) {
	meta, _ := json.Marshal(map[string]any{"analysisStatus": state, "analysisCheckedAt": time.Now().Format(time.RFC3339)})
	if _, err := execConnector(ctx, h, connector, instance, "set_metadata", map[string]any{
		"repo_id": repo, "card_id": cardID, "metadata": string(meta),
	}); err != nil {
		job.Logf(ctx, "warn: could not mark %s as %s: %v", cardID, state, err)
	}
}

// ---------------------------------------------------------------- helpers

func firstInstance(ctx context.Context, h host.Host, connectorKey string) (string, error) {
	resp, err := h.ListConnectorInstances(ctx, &hostpb.ListConnectorInstancesRequest{Connector: connectorKey})
	if err != nil {
		return "", fmt.Errorf("list %s instances: %w", connectorKey, err)
	}
	for _, inst := range resp.GetInstances() {
		return inst.GetId(), nil
	}
	return "", fmt.Errorf("no %s connector instance is visible; install and configure it first", connectorKey)
}

func execConnector(ctx context.Context, h host.Host, connectorKey, instance, op string, args map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	resp, err := h.ExecuteConnector(ctx, &hostpb.ExecuteConnectorRequest{
		Connector: connectorKey, InstanceId: instance, Op: op, ArgsJson: raw,
	})
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", connectorKey, op, err)
	}
	out := map[string]any{}
	if len(resp.GetResultJson()) > 0 {
		if err := json.Unmarshal(resp.GetResultJson(), &out); err != nil {
			return nil, fmt.Errorf("decode %s.%s: %w", connectorKey, op, err)
		}
	}
	return out, nil
}

func runBin(ctx context.Context, bin string, args ...string) (string, error) {
	if strings.TrimSpace(bin) == "" {
		return "", fmt.Errorf("binary path is empty")
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func str(v any) string {
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
