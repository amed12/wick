// Command transcribe_reels turns saved Instagram reels into card documents.
//
// It is a batch job on purpose: transcription burns minutes of CPU per video,
// so it must never happen inside a chat turn. The job picks cards that carry a
// metadata flag, downloads the reel, runs whisper.cpp, and writes the transcript
// onto the card.
//
// It talks to Planban over plain HTTP and to Instagram via yt-dlp with an
// exported cookie jar — deliberately NOT through the wick Host API. A job run
// has no person behind it, so connector calls would need a plugin identity the
// job runtime does not carry; keeping the job self-contained means it works
// from a cron tick with no session and no token.
package main

import (
	wickplugin "github.com/yogasw/wick/pkg/plugin"
)

func main() {
	wickplugin.ServeJob(Module())
}
