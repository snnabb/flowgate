package common

import (
	"flag"
	"fmt"
	"os"
)

// PanelConfig holds panel server configuration
type PanelConfig struct {
	Host     string
	Port     int
	DBPath   string
	JWTSecret string
	TLS      bool
	CertFile string
	KeyFile  string
}

// NodeConfig holds node agent configuration
type NodeConfig struct {
	PanelURL string
	APIKey   string
	TLS      bool
}

// ParseArgs parses command line arguments and returns the mode
func ParseArgs() (string, *PanelConfig, *NodeConfig) {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	mode := os.Args[1]

	switch mode {
	case "panel":
		cfg := &PanelConfig{}
		fs := flag.NewFlagSet("panel", flag.ExitOnError)
		fs.StringVar(&cfg.Host, "host", "0.0.0.0", "Listen host")
		fs.IntVar(&cfg.Port, "port", 8080, "Listen port")
		fs.StringVar(&cfg.DBPath, "db", "flowgate.db", "SQLite database path")
		fs.StringVar(&cfg.JWTSecret, "secret", "flowgate-secret-change-me", "JWT secret key")
		fs.BoolVar(&cfg.TLS, "tls", false, "Enable TLS")
		fs.StringVar(&cfg.CertFile, "cert", "", "TLS certificate file")
		fs.StringVar(&cfg.KeyFile, "key", "", "TLS key file")
		fs.Parse(os.Args[2:])
		return "panel", cfg, nil

	case "node":
		cfg := &NodeConfig{}
		fs := flag.NewFlagSet("node", flag.ExitOnError)
		fs.StringVar(&cfg.PanelURL, "panel", "", "Panel WebSocket URL (ws://host:port/ws/node)")
		fs.StringVar(&cfg.APIKey, "key", "", "Node API key")
		fs.BoolVar(&cfg.TLS, "tls", false, "Use wss:// for TLS")
		fs.Parse(os.Args[2:])

		if cfg.PanelURL == "" || cfg.APIKey == "" {
			fmt.Println("Error: --panel and --key are required for node mode")
			fs.Usage()
			os.Exit(1)
		}
		return "node", nil, cfg

	default:
		printUsage()
		os.Exit(1)
	}

	return "", nil, nil
}

func printUsage() {
	fmt.Println(`FlowGate - Lightweight Port Forwarding Panel

Usage:
  flowgate panel [options]    Start as panel (management server)
  flowgate node  [options]    Start as node  (forwarding agent)

Panel Options:
  --host     Listen host (default: 0.0.0.0)
  --port     Listen port (default: 8080)
  --db       SQLite database path (default: flowgate.db)
  --secret   JWT secret key
  --tls      Enable TLS
  --cert     TLS certificate file
  --key      TLS key file

Node Options:
  --panel    Panel WebSocket URL (required, e.g. ws://panel-ip:8080/ws/node)
  --key      Node API key (required)
  --tls      Use wss:// for TLS`)
}
