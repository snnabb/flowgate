package main

import (
	"io/fs"
	"log"
	"os"

	"github.com/flowgate/flowgate/internal/common"
	"github.com/flowgate/flowgate/internal/node"
	"github.com/flowgate/flowgate/internal/panel"
	"github.com/flowgate/flowgate/web"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	if len(os.Args) < 2 {
		common.ParseArgs() // Will print usage and exit
		return
	}

	mode, panelCfg, nodeCfg := common.ParseArgs()

	switch mode {
	case "panel":
		// Extract the static subdirectory from the embedded web FS
		webFS, err := fs.Sub(web.EmbeddedFiles, "static")
		if err != nil {
			log.Fatalf("Failed to load embedded web UI: %v", err)
		}
		if err := panel.Start(panelCfg, webFS); err != nil {
			log.Fatalf("Panel failed: %v", err)
		}
	case "node":
		agent := node.NewAgent(nodeCfg.PanelURL, nodeCfg.APIKey, nodeCfg.TLS)
		if err := agent.Start(); err != nil {
			log.Fatalf("Node failed: %v", err)
		}
	}
}
