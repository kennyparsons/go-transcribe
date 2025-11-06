package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/manifoldco/promptui"
	"github.com/schollz/progressbar/v3"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var version = "dev"

// --- Global Flags ---
// Defined globally to be accessible by the transcribe function for the output shim.
var (
	modelPath = flag.String("model", "", "Path to the whisper.cpp model file. Overrides the configured default.")
	stdout    = flag.Bool("stdout", false, "Print transcription to standard output. This suppresses all other logging.")
	logLevel  = flag.Int("log-level", LogLevelInfo, "Set log verbosity (0=Silent, 1=Error, 2=Warn, 3=Info, 4=Debug).")
)

// --- Logging ---

const (
	LogLevelSilent = iota
	LogLevelError
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

var globalLogLevel = LogLevelInfo

func logit(level int, format string, a ...any) {
	if level <= globalLogLevel {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}

// --- Configuration ---

type Config struct {
	DefaultModelPath string `json:"default_model_path"`
}

func getConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".config", "go-transcribe.json"), nil
}

func loadConfig() (Config, error) {
	var config Config
	configPath, err := getConfigPath()
	if err != nil {
		return config, err
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default config if file doesn't exist
			return Config{DefaultModelPath: filepath.Join(os.Getenv("HOME"), ".config", "whisper-cpp", "models", "ggml-base.en.bin")}, nil
		}
		return config, err
	}

	err = json.Unmarshal(data, &config)
	return config, err
}

func saveConfig(config Config) error {
	configPath, err := getConfigPath()
	if err != nil {
		return err
	}

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}

func must(err error) {
	if err != nil {
		logit(LogLevelError, "error: %v", err)
		os.Exit(1)
	}
}

// --- Transcribe Command ---

func vlcToPCM(inputFile string) ([]float32, error) {
	// Check if VLC is in PATH
	_, err := exec.LookPath("vlc")
	if err != nil {
		return nil, errors.New("VLC command not found, please install VLC and ensure it is in your PATH")
	}

	// Create a temporary file for VLC to write the WAV output
	tempFile, err := os.CreateTemp("", "vlc-*.wav")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFilePath := tempFile.Name()
	tempFile.Close()              // Close the file so VLC can write to it
	defer os.Remove(tempFilePath) // Clean up the temp file

	// Construct the VLC command
	soutString := fmt.Sprintf("#transcode{acodec=s16l,samplerate=16000,channels=1}:standard{access=file,mux=wav,dst=%s}", tempFilePath)
	cmd := exec.Command("vlc", "-I", "dummy", "--no-sout-video", inputFile, "--sout", soutString, "vlc://quit")

	// Run the command and capture output
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("VLC execution failed: %w\nVLC stderr: %s", err, stderr.String())
	}

	// --- Silence ffmpeg-go logger ---
	log.SetOutput(io.Discard)
	defer log.SetOutput(os.Stderr) // Restore logger output

	// Now, use ffmpeg-go to read the clean WAV file produced by VLC
	buf := bytes.NewBuffer(nil)
	err = ffmpeg.Input(tempFilePath).
		Output("pipe:", ffmpeg.KwArgs{
			"format": "s16le",
			"acodec": "pcm_s16le",
		}).
		WithOutput(buf).
		Run()

	if err != nil {
		return nil, fmt.Errorf("ffmpeg-go failed to read WAV file: %w", err)
	}

	data := buf.Bytes()
	if len(data) == 0 {
		return nil, errors.New("ffmpeg-go produced no output from WAV file")
	}

	if len(data)%2 != 0 {
		return nil, errors.New("odd PCM data length from WAV file")
	}

	samples := make([]float32, len(data)/2)
	reader := bytes.NewReader(data)

	for i := range samples {
		var v int16
		if err := binary.Read(reader, binary.LittleEndian, &v); err != nil {
			return nil, fmt.Errorf("failed to read PCM data from WAV file: %w", err)
		}
		samples[i] = float32(v) / 32768.0
	}

	return samples, nil
}

func transcribe(args []string, modelPathOverride string) {
	config, err := loadConfig()
	must(err)

	// Determine model path: flag > config > default
	modelPath := config.DefaultModelPath
	if modelPathOverride != "" {
		modelPath = modelPathOverride
	}

	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s transcribe <media-file>\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "Use the global --model flag to override the default model path.")
		os.Exit(1)
	}

	in := args[0]
	base := filepath.Base(in)
	name := base[:len(base)-len(filepath.Ext(base))]
	dir := filepath.Dir(in)
	outTxt := filepath.Join(dir, name+".txt")

	logit(3, "Extracting audio...")
	samples, err := vlcToPCM(in)
	must(err)

	logit(3, "Loading model...")
	logit(3, "Transcribing...")

	// --- Capture and suppress C++ output using low-level file descriptor redirection ---

	// Save original file descriptors
	origStdout, err := syscall.Dup(int(os.Stdout.Fd()))
	must(err)
	origStderr, err := syscall.Dup(int(os.Stderr.Fd()))
	must(err)

	// Create a pipe
	r, w, err := os.Pipe()
	must(err)

	// Redirect stdout and stderr to the write end of the pipe
	err = syscall.Dup2(int(w.Fd()), int(os.Stdout.Fd()))
	must(err)
	err = syscall.Dup2(int(w.Fd()), int(os.Stderr.Fd()))
	must(err)

	// This defer block is crucial to ensure the original FDs are restored
	defer func() {
		w.Close()
		syscall.Dup2(origStdout, int(os.Stdout.Fd()))
		syscall.Dup2(origStderr, int(os.Stderr.Fd()))
		syscall.Close(origStdout)
		syscall.Close(origStderr)
	}()

	// --- Start of captured section ---

	model, err := whisper.New(modelPath)
	if err != nil {
		// Manually restore output to print the error
		w.Close()
		syscall.Dup2(origStdout, int(os.Stdout.Fd()))
		syscall.Dup2(origStderr, int(os.Stderr.Fd()))

		outputBytes, _ := io.ReadAll(r)
		logit(1, "Error loading model:\n--- C/C++ Output ---\n%s\n---------------------\n", outputBytes)
		must(err)
	}
	defer model.Close()

	ctx, err := model.NewContext()
	if err != nil {
		w.Close()
		syscall.Dup2(origStdout, int(os.Stdout.Fd()))
		syscall.Dup2(origStderr, int(os.Stderr.Fd()))

		outputBytes, _ := io.ReadAll(r)
		logit(1, "Error creating context:\n--- C/C++ Output ---\n%s\n---------------------\n", outputBytes)
		must(err)
	}

	// Auto-detect language from model name
	language := "en"
	if strings.Contains(filepath.Base(modelPath), "kotoba") {
		language = "ja"
		logit(3, "Japanese model detected, setting language to 'ja'.")
	}
	ctx.SetLanguage(language)

	err = ctx.Process(samples, nil, nil, nil)

	// --- End of captured section ---

	// Close the write end of the pipe to signal EOF to the reader
	w.Close()

	// Restore original file descriptors
	syscall.Dup2(origStdout, int(os.Stdout.Fd()))
	syscall.Dup2(origStderr, int(os.Stderr.Fd()))

	if err != nil {
		outputBytes, _ := io.ReadAll(r)
		logit(1, "Error during transcription:\n--- C/C++ Output ---\n%s\n---------------------\n", outputBytes)
		must(err)
	}
	r.Close()

	// --- Output Shim ---
	// Check the global `stdout` flag variable.
	if *stdout {
		// If --stdout is used, print transcription directly to os.Stdout
		for {
			seg, err := ctx.NextSegment()
			if errors.Is(err, io.EOF) {
				break
			}
			must(err)
			// Use fmt.Fprintln to os.Stdout, NOT logit
			fmt.Fprintln(os.Stdout, seg.Text)
		}
	} else {
		// Default behavior: write to a .txt file
		f, err := os.Create(outTxt)
		must(err)
		defer f.Close()

		// Write UTF-8 BOM
		_, err = f.Write([]byte{0xEF, 0xBB, 0xBF})
		must(err)

		for {
			seg, err := ctx.NextSegment()
			if errors.Is(err, io.EOF) {
				break
			}
			must(err)
			fmt.Fprintln(f, seg.Text)
		}
		logit(3, "✅ Transcription saved to \"%s\"", outTxt)
	}
}

// --- Setup Command ---

func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

func downloadFileWithProgress(url, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	bar := progressbar.DefaultBytes(
		resp.ContentLength,
		fmt.Sprintf("Downloading %s", filepath.Base(destPath)),
	)
	_, err = io.Copy(io.MultiWriter(f, bar), resp.Body)
	return err
}

func performDownload(modelName string) {
	clearScreen()
	logit(LogLevelInfo, "Preparing to download: %s\n", modelName)

	var url string
	switch modelName {
	case "small.en-tdrz":
		url = "https://huggingface.co/akashmjn/tinydiarize-whisper.cpp/resolve/main/ggml-small.en-tdrz.bin"
	case "large-v3-kotoba.ja_JP":
		url = "https://huggingface.co/kotoba-tech/kotoba-whisper-v2.0-ggml/resolve/main/ggml-kotoba-whisper-v2.0.bin"
	default:
		url = fmt.Sprintf("https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-%s.bin", modelName)
	}

	homeDir, err := os.UserHomeDir()
	must(err)
	destDir := filepath.Join(homeDir, ".config", "whisper-cpp", "models")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		logit(LogLevelError, "Error creating destination directory: %v", err)
		return
	}
	destFile := filepath.Join(destDir, fmt.Sprintf("ggml-%s.bin", modelName))

	if _, err := os.Stat(destFile); err == nil {
		logit(LogLevelInfo, "Model %s already exists at %s. Skipping download.", modelName, destFile)
	} else {
		logit(LogLevelInfo, "Downloading from: %s", url)
		logit(LogLevelInfo, "Saving to: %s\n", destFile)
		if err := downloadFileWithProgress(url, destFile); err != nil {
			logit(LogLevelError, "\n\nError downloading model: %v", err)
			os.Remove(destFile)
		} else {
			logit(LogLevelInfo, "\n\n✅ Download complete!")
		}
	}

	fmt.Print("\nPress Enter to return to the menu...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func downloadModels() {
	models := []string{"tiny.en", "base.en", "small.en", "small.en-tdrz", "medium.en", "large-v3", "large-v3-q5_0", "large-v3-kotoba.ja_JP"}
	homeDir, err := os.UserHomeDir()
	must(err)
	modelsDir := filepath.Join(homeDir, ".config", "whisper-cpp", "models")

	type modelStatus struct {
		Name       string
		Downloaded bool
	}

	for {
		clearScreen()

		var modelStatuses []modelStatus
		for _, model := range models {
			modelPath := filepath.Join(modelsDir, fmt.Sprintf("ggml-%s.bin", model))
			_, err := os.Stat(modelPath)
			modelStatuses = append(modelStatuses, modelStatus{Name: model, Downloaded: err == nil})
		}

		// Add "Back" option separately as it doesn't have a downloaded status
		type menuItem struct {
			Name       string
			Downloaded bool
			IsBack     bool
		}

		var menuItems []menuItem
		for _, ms := range modelStatuses {
			menuItems = append(menuItems, menuItem{Name: ms.Name, Downloaded: ms.Downloaded})
		}
		menuItems = append(menuItems, menuItem{Name: "Back to main menu", IsBack: true})

		templates := &promptui.SelectTemplates{
			Label:    "{{ . }}?",
			Active:   `→ {{ if .IsBack }}{{ .Name | cyan }}{{ else }}{{ .Name | cyan }} {{ if .Downloaded }}(downloaded){{ end }}{{ end }}`,
			Inactive: `  {{ if .IsBack }}{{ .Name | faint }}{{ else }}{{ .Name | faint }} {{ if .Downloaded }}(downloaded){{ end }}{{ end }}`,
			Selected: `✓ {{ .Name | green }}`,
		}

		prompt := promptui.Select{
			Label:     "Model Download Menu",
			Items:     menuItems,
			Templates: templates,
			Size:      10,
		}

		i, _, err := prompt.Run()
		if err != nil {
			if err == promptui.ErrInterrupt {
				return
			}
			logit(LogLevelError, "Prompt failed %v", err)
			return
		}

		selectedItem := menuItems[i]

		if selectedItem.IsBack {
			return
		}

		performDownload(selectedItem.Name)
	}
}

func selectDefaultModel() {
	config, err := loadConfig()
	must(err)

	models := []string{"tiny.en", "base.en", "small.en", "small.en-tdrz", "medium.en", "large-v3", "large-v3-q5_0", "large-v3-kotoba.ja_JP"}
	homeDir, err := os.UserHomeDir()
	must(err)
	modelsDir := filepath.Join(homeDir, ".config", "whisper-cpp", "models")

	type menuItem struct {
		Name       string
		Downloaded bool
		IsCurrent  bool
		IsBack     bool
	}

	var menuItems []menuItem
	for _, model := range models {
		modelPath := filepath.Join(modelsDir, fmt.Sprintf("ggml-%s.bin", model))
		_, err := os.Stat(modelPath)
		isCurrent := (modelPath == config.DefaultModelPath)
		menuItems = append(menuItems, menuItem{Name: model, Downloaded: err == nil, IsCurrent: isCurrent})
	}
	menuItems = append(menuItems, menuItem{Name: "Back to main menu", IsBack: true})

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}?",
		Active:   `→ {{ if .IsBack }}{{ .Name | cyan }}{{ else }}{{ .Name | cyan }} {{ if .IsCurrent }}(current){{ end }}{{ end }}`,
		Inactive: `  {{ if .IsBack }}{{ .Name | faint }}{{ else }}{{ .Name | faint }} {{ if .IsCurrent }}(current){{ end }}{{ end }}`,
		Selected: "✓ {{ .Name | green }}",
	}

	prompt := promptui.Select{
		Label:     "Select default model",
		Items:     menuItems,
		Templates: templates,
		Size:      10,
	}

	i, _, err := prompt.Run()
	if err != nil {
		if err == promptui.ErrInterrupt {
			return
		}
		logit(LogLevelError, "Prompt failed %v", err)
		return
	}

	selectedItem := menuItems[i]

	if selectedItem.IsBack {
		return
	}

	if selectedItem.IsCurrent {
		logit(LogLevelInfo, "\nThis is already the default model.")
		fmt.Print("\nPress Enter to return to the menu...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
		return
	}

	newPath := filepath.Join(modelsDir, fmt.Sprintf("ggml-%s.bin", selectedItem.Name))

	// Update config immediately
	config.DefaultModelPath = newPath
	err = saveConfig(config)
	must(err)
	logit(LogLevelInfo, "\n✅ Default model path updated to: %s", newPath)

	// If not downloaded, start download
	if !selectedItem.Downloaded {
		logit(LogLevelInfo, "\nModel '%s' is not downloaded. Starting download...", selectedItem.Name)
		performDownload(selectedItem.Name)
	} else {
		fmt.Print("\nPress Enter to return to the menu...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}
}

func setup(args []string) {
	// Ensure the config file exists with defaults if it's missing.
	configPath, err := getConfigPath()
	must(err)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		logit(LogLevelInfo, "No config file found. Creating one with default settings.")
		config, err := loadConfig()
		must(err)
		err = saveConfig(config)
		must(err)
	}

	for {
		clearScreen()
		prompt := promptui.Select{
			Label: "Go Transcribe Setup Menu",
			Items: []string{"Download models", "Select default model", "Exit"},
			Size:  5,
		}

		_, result, err := prompt.Run()

		if err != nil {
			if err == promptui.ErrInterrupt {
				logit(LogLevelInfo, "Exiting setup.")
				return
			}
			logit(LogLevelError, "Prompt failed %v", err)
			return
		}

		switch result {
		case "Download models":
			downloadModels()
		case "Select default model":
			selectDefaultModel()
		case "Exit":
			logit(LogLevelInfo, "Exiting setup.")
			return
		}
	}
}

// --- Version Command ---

func showVersion() {
	// Version info should always print to stdout
	fmt.Printf("go-transcribe: %s\n", version)
}

// --- Main ---

func main() {
	// --- Argument Parsing ---
	// The Go `flag` package stops parsing at the first non-flag argument.
	// To allow flags to be placed anywhere (e.g., after the filename),
	// we manually separate flags from non-flag arguments.
	var flags, nonFlags []string
	args := os.Args[1:]
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			nonFlags = append(nonFlags, arg)
			i++
			continue
		}

		// It's a flag.
		flags = append(flags, arg)
		i++

		// If it's a flag that takes a value...
		if (arg == "-model" || arg == "--model" || arg == "-log-level" || arg == "--log-level") {
			// ...and the value is not specified with '=', and there is a next argument that is not a flag...
			if !strings.Contains(arg, "=") && i < len(args) && !strings.HasPrefix(args[i], "-") {
				// ...then the next argument is the value.
				flags = append(flags, args[i])
				i++ // Consume the value
			}
		}
	}
	// Reassemble os.Args with the program name, followed by flags, then non-flags.
	os.Args = append([]string{os.Args[0]}, append(flags, nonFlags...)...)

	// Parse the now-ordered flags.
	flag.Parse()

	// --- Log Level Determination ---
	if *stdout {
		globalLogLevel = LogLevelSilent
	} else {
		globalLogLevel = *logLevel
	}

	// --- Command Dispatch ---
	commandArgs := flag.Args() // These are now guaranteed to be the non-flag arguments
	if len(commandArgs) < 1 {
		// Usage info should go to stderr.
		fmt.Fprintf(os.Stderr, "Usage: %s [global options] <command> [command options]\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "\nGlobal Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nCommands:")
		fmt.Fprintln(os.Stderr, "  transcribe   Transcribe a media file")
		fmt.Fprintln(os.Stderr, "  setup        Enter interactive setup mode")
		fmt.Fprintln(os.Stderr, "  version      Show application version")
		os.Exit(1)
	}

	command := commandArgs[0]
	switch command {
	case "setup":
		setup(commandArgs[1:])
	case "version":
		showVersion()
	case "transcribe":
		transcribe(commandArgs[1:], *modelPath)
	default:
		// If the command is not a recognized one, assume it's a file path for transcription.
		transcribe(commandArgs, *modelPath)
	}
}
