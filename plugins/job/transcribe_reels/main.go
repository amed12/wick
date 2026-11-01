// Command transcribe_reels turns saved Instagram reels into card documents.
//
// It is a batch job on purpose: transcription burns minutes of CPU per video,
// so it must never happen inside a chat turn. The job picks cards that carry a
// metadata flag, downloads the reel, runs whisper.cpp, and writes the transcript
// onto the card.
//
// Media lookup and board writes are delegated to the instagram and planban
// connectors over the Host API — this plugin owns no browser and no board code.
package main

import (
	wickplugin "github.com/yogasw/wick/pkg/plugin"
)

func main() {
	wickplugin.DeclareHostAccess(
		[]string{wickplugin.ScopeHostConnectorsRead, wickplugin.ScopeHostConnectorsExec},
		&wickplugin.HostResources{
			Connectors: []wickplugin.HostConnectorResource{
				{Connector: "planban", Ops: []string{"find_cards", "write_doc", "set_metadata"}},
				{Connector: "instagram", Ops: []string{"post"}},
			},
		},
	)
	wickplugin.ServeJob(Module())
}
