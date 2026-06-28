package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/aeon022/calctl/internal/config"
	"github.com/aeon022/calctl/internal/tui"
	"github.com/spf13/cobra"
)

var Version = "dev"

var formatFlag string

var rootCmd = &cobra.Command{
	Use:   "calctl",
	Short: "Calendar management from the terminal",
	Long:  "calctl reads and writes Apple Calendar (and Google Calendar) from the command line.\nDesigned for AI-assisted scheduling via MCP or shell pipelines.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return config.Load()
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&formatFlag, "format", "human", "Output format: human | json")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("calctl %s\n", Version)
		},
	})
}

// outputJSON is a helper used by all commands for JSON output.
func outputJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(b))
}

func isJSON() bool { return formatFlag == "json" }
