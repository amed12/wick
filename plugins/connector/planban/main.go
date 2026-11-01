// Command planban is a wick connector plugin for a local Planban board.
//
// Planban (https://github.com/fthllh/planban, local copy at ~/4_utility/planban)
// is a local kanban/protocol app: groups of cards, each with a title, status,
// tags and free-form metadata. This connector lets an agent file what it finds
// (Instagram saves, notes, tasks) onto a board without a human opening the UI.
//
// Identity rule: Meta.Key == folder name (slug, no dash) == "planban".
package main

import (
	wickplugin "github.com/yogasw/wick/pkg/plugin"
)

func main() {
	wickplugin.Serve(Module())
}
