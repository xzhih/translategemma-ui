package app

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"translategemma-ui/internal/config"
	"translategemma-ui/internal/huggingface"
	"translategemma-ui/internal/models"
	"translategemma-ui/internal/modelstore"
	"translategemma-ui/internal/runtimeutil"
	"translategemma-ui/internal/tui"
	"translategemma-ui/internal/version"
	"translategemma-ui/internal/web"
)

// Run is the single process entrypoint for CLI, TUI, and WebUI modes.
func Run(args []string) error {
	if len(args) > 0 && args[0] == "translate" {
		return runTranslateCommand(args[1:])
	}
	if len(args) > 0 && args[0] == "models" {
		return runModelsCommand(args[1:])
	}

	fs := flag.NewFlagSet("translategemma-ui", flag.ContinueOnError)
	var (
		tuiMode     bool
		webMode     bool
		listen      string
		showHelp    bool
		showVersion bool
	)

	fs.BoolVar(&tuiMode, "tui", false, "Run Bubble Tea interface")
	fs.BoolVar(&webMode, "webui", false, "Run local web interface")
	fs.StringVar(&listen, "listen", "127.0.0.1:8090", "Web UI listen address")
	fs.BoolVar(&showHelp, "help", false, "Show help")
	fs.BoolVar(&showHelp, "h", false, "Show help")
	fs.BoolVar(&showVersion, "version", false, "Print version")

	if err := fs.Parse(args); err != nil {
		return err
	}
	if showHelp {
		printUsage(fs)
		return nil
	}
	if showVersion {
		fmt.Println(version.String())
		return nil
	}
	if tuiMode && webMode {
		return errors.New("flags --tui and --webui are mutually exclusive")
	}

	root, err := config.EnsureDataDirs("")
	if err != nil {
		return fmt.Errorf("initialize data dir: %w", err)
	}

	switch {
	case tuiMode:
		return tui.Run("", root)
	case webMode:
		return web.Run(listen, "", root)
	default:
		printUsage(fs)
		return nil
	}
}

func runModelsCommand(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: translategemma-ui models [list|download|delete]")
	}

	root, err := config.EnsureDataDirs("")
	if err != nil {
		return fmt.Errorf("initialize data dir: %w", err)
	}
	cfg, _ := config.LoadAppConfig(root)
	state, _ := config.LoadAppState(root)
	available := huggingface.ListTranslateGemmaModels()

	switch args[0] {
	case "list", "ls":
		catalog := modelstore.Catalog(root, available, strings.TrimSpace(cfg.ActiveModelID), strings.TrimSpace(state.ActiveModelPath))
		for _, item := range catalog {
			status := "remote"
			if item.Installed {
				status = "installed"
			}
			if item.Active {
				status += ",active"
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", item.ID, item.FileName, item.Size, status)
		}
		return nil

	case "download":
		fs := flag.NewFlagSet("models download", flag.ContinueOnError)
		var modelID string
		fs.StringVar(&modelID, "id", strings.TrimSpace(cfg.ActiveModelID), "Model ID to download")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(modelID) == "" {
			return errors.New("missing --id for models download")
		}
		item, ok := models.FindByID(available, modelID)
		if !ok {
			return fmt.Errorf("unknown model id %q", modelID)
		}
		fmt.Printf("Downloading %s\n", item.FileName)
		modelPath, err := huggingface.DownloadModel(root, item, func(p huggingface.DownloadProgress) {
			fmt.Printf("\r[%3.0f%%] %s", p.Percent, p.Message)
		})
		if err != nil {
			fmt.Println()
			return err
		}
		fmt.Println()
		runtimeutil.ApplyActiveModel(&cfg, &state, item, modelPath)
		upsertArtifact(&state, config.InstalledArtifact{
			Kind:      "model",
			ID:        item.ID,
			FileName:  item.FileName,
			Path:      modelPath,
			SizeBytes: item.SizeBytes,
		})
		if err := config.SaveAppConfig(root, cfg); err != nil {
			return err
		}
		if err := config.SaveAppState(root, state); err != nil {
			return err
		}
		fmt.Printf("Saved to %s\n", modelPath)
		return nil

	case "delete", "rm":
		fs := flag.NewFlagSet("models delete", flag.ContinueOnError)
		var modelID string
		fs.StringVar(&modelID, "id", strings.TrimSpace(cfg.ActiveModelID), "Model ID to delete")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if strings.TrimSpace(modelID) == "" {
			return errors.New("missing --id for models delete")
		}
		item, ok := models.FindByID(available, modelID)
		if !ok {
			return fmt.Errorf("unknown model id %q", modelID)
		}
		path, deleted, err := modelstore.DeleteModel(root, item, &state)
		if err != nil {
			return err
		}
		if !deleted {
			fmt.Printf("Model %s is not installed locally\n", item.FileName)
			return nil
		}
		if strings.TrimSpace(cfg.ActiveModelID) == item.ID && state.ActiveModelPath == "" {
			cfg.ActiveModelID = ""
		}
		if err := config.SaveAppConfig(root, cfg); err != nil {
			return err
		}
		if err := config.SaveAppState(root, state); err != nil {
			return err
		}
		fmt.Printf("Deleted %s\n", path)
		return nil
	default:
		return fmt.Errorf("unknown models subcommand %q", args[0])
	}
}

func upsertArtifact(state *config.AppState, next config.InstalledArtifact) {
	for i := range state.Artifacts {
		if state.Artifacts[i].Kind == next.Kind && state.Artifacts[i].ID == next.ID {
			state.Artifacts[i] = next
			return
		}
	}
	state.Artifacts = append(state.Artifacts, next)
}

func printUsage(fs *flag.FlagSet) {
	fmt.Println("TranslateGemmaUI")
	fmt.Println(version.String())
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [--tui | --webui | --version] [flags]\n", fs.Name())
	fmt.Printf("  %s translate [text|image] [flags]\n", fs.Name())
	fmt.Printf("  %s models [list|download|delete] [flags]\n", fs.Name())
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Printf("  %s --version\n", fs.Name())
	fmt.Printf("  %s --tui\n", fs.Name())
	fmt.Printf("  %s --webui --listen 127.0.0.1:8090\n", fs.Name())
	fmt.Printf("  %s translate text --text 'Hello world'\n", fs.Name())
	fmt.Printf("  %s translate image --file /path/to/image.png --model-id q8_0_vision\n", fs.Name())
	fmt.Printf("  %s models list\n", fs.Name())
	fmt.Printf("  %s models download --id q4_k_m\n", fs.Name())
	fmt.Printf("  %s models delete --id q4_k_m\n", fs.Name())
	fmt.Println()
	fmt.Println("Flags:")
	fs.PrintDefaults()
}
