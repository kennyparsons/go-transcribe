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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/schollz/progressbar/v3"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var version = "dev"

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
		fmt.Fprintln(os.Stderr, "error:", err)
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
	tempFile.Close() // Close the file so VLC can write to it
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

	transcribeCmd := flag.NewFlagSet("transcribe", flag.ExitOnError)

	transcribeCmd.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s transcribe <media-file>\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "Use the global --model flag to override the default model path.")
	}

	transcribeCmd.Parse(args)

	if transcribeCmd.NArg() < 1 {
		transcribeCmd.Usage()
		os.Exit(1)
	}

	in := transcribeCmd.Arg(0)
	base := filepath.Base(in)
	name := base[:len(base)-len(filepath.Ext(base))]
	dir := filepath.Dir(in)
	outTxt := filepath.Join(dir, name+".txt")

	fmt.Println("Extracting audio...")
	samples, err := vlcToPCM(in)
	must(err)

	// --- Capture and suppress C++ output ---
	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	fmt.Println("Loading model...")
	model, err := whisper.New(modelPath)
	if err != nil {
		// Stop capturing and print error
		wOut.Close()
		wErr.Close()
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		stdoutBytes, _ := io.ReadAll(rOut)
		stderrBytes, _ := io.ReadAll(rErr)
		fmt.Printf("Error loading model:\n---stdout---\n%s\n---stderr---\n%s\n", stdoutBytes, stderrBytes)
		must(err)
	}
	defer model.Close()

	ctx, err := model.NewContext()
	if err != nil {
		wOut.Close()
		wErr.Close()
		os.Stdout = oldStdout
		os.Stderr = oldStderr
		stdoutBytes, _ := io.ReadAll(rOut)
		stderrBytes, _ := io.ReadAll(rErr)
		fmt.Printf("Error creating context:\n---stdout---\n%s\n---stderr---\n%s\n", stdoutBytes, stderrBytes)
		must(err)
	}

	ctx.SetLanguage("en")

	fmt.Println("Transcribing...")
	err = ctx.Process(samples, nil, nil, nil)

	// --- Restore output ---
	wOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err != nil {
		stdoutBytes, _ := io.ReadAll(rOut)
		stderrBytes, _ := io.ReadAll(rErr)
		fmt.Printf("Error during transcription:\n---stdout---\n%s\n---stderr---\n%s\n", stdoutBytes, stderrBytes)
		must(err)
	}

	f, err := os.Create(outTxt)
	must(err)
	defer f.Close()

	for {
		seg, err := ctx.NextSegment()
		if errors.Is(err, io.EOF) {
			break
		}
		must(err)
		fmt.Fprintln(f, seg.Text)
	}

	fmt.Printf("✅ Transcription saved to %s\n", outTxt)
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
	fmt.Printf("Preparing to download: %s\n\n", modelName)

	src := "https://huggingface.co/ggerganov/whisper.cpp"
	pfx := "resolve/main/ggml"
	if strings.Contains(modelName, "tdrz") {
		src = "https://huggingface.co/akashmjn/tinydiarize-whisper.cpp"
	}
	url := fmt.Sprintf("%s/%s-%s.bin", src, pfx, modelName)

	homeDir, err := os.UserHomeDir()
	must(err)
	destDir := filepath.Join(homeDir, ".config", "whisper-cpp", "models")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		fmt.Printf("Error creating destination directory: %v\n", err)
		return
	}
	destFile := filepath.Join(destDir, fmt.Sprintf("ggml-%s.bin", modelName))

	if _, err := os.Stat(destFile); err == nil {
		fmt.Printf("Model %s already exists at %s. Skipping download.\n", modelName, destFile)
	} else {
		fmt.Printf("Downloading from: %s\n", url)
		fmt.Printf("Saving to: %s\n\n", destFile)
		if err := downloadFileWithProgress(url, destFile); err != nil {
			fmt.Printf("\n\nError downloading model: %v\n", err)
			os.Remove(destFile)
		} else {
			fmt.Printf("\n\n✅ Download complete!\n")
		}
	}

	fmt.Print("\nPress Enter to return to the menu...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}

func downloadModels() {
	models := []string{"tiny.en", "base.en", "small.en", "small.en-tdrz", "medium.en", "large-v3"}
	reader := bufio.NewReader(os.Stdin)

	for {
		clearScreen()
		fmt.Println("--------------------------")
		fmt.Println("   Model Download Menu    ")
		fmt.Println("--------------------------")
		for i, model := range models {
			fmt.Printf("%d. %s\n", i+1, model)
		}
		fmt.Printf("%d. Back to main menu\n", len(models)+1)
		fmt.Print("\nPlease select a model to download: ")

		input, _ := reader.ReadString('\n')
		choice, err := strconv.Atoi(strings.TrimSpace(input))

		if err != nil || choice < 1 || choice > len(models)+1 {
			fmt.Println("\nInvalid option.")
			fmt.Print("Press Enter to continue...")
			reader.ReadString('\n')
			continue
		}

		if choice == len(models)+1 {
			return
		}

		performDownload(models[choice-1])
	}
}

func setDefaultModel() {
	reader := bufio.NewReader(os.Stdin)
	config, err := loadConfig()
	must(err)

	clearScreen()
	fmt.Println("--------------------------")
	fmt.Println("  Set Default Model Path  ")
	fmt.Println("--------------------------")
	fmt.Printf("\nCurrent default: %s\n", config.DefaultModelPath)
	fmt.Print("\nEnter the new full path to your default model file: ")

	input, _ := reader.ReadString('\n')
	newPath := strings.TrimSpace(input)

	if _, err := os.Stat(newPath); err != nil {
		fmt.Printf("\nError: File does not exist at '%s'\n", newPath)
	} else {
		config.DefaultModelPath = newPath
		err := saveConfig(config)
		must(err)
		fmt.Printf("\n✅ Default model path updated to: %s\n", newPath)
	}

	fmt.Print("\nPress Enter to return to the menu...")
	reader.ReadString('\n')
}

func setup(args []string) {
	// Ensure the config file exists with defaults if it's missing.
	configPath, err := getConfigPath()
	must(err)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Println("No config file found. Creating one with default settings.")
		config, err := loadConfig()
		must(err)
		err = saveConfig(config)
		must(err)
	}

	reader := bufio.NewReader(os.Stdin)

	for {
		clearScreen()
		fmt.Println("---------------------")
		fmt.Println(" Go Transcribe Setup Menu ")
		fmt.Println("---------------------")
		fmt.Println("1. Download models")
		fmt.Println("2. Set default model path")
		fmt.Println("3. Exit")
		fmt.Print("\nPlease select an option: ")

		input, _ := reader.ReadString('\n')
		choice := strings.TrimSpace(input)

		switch choice {
		case "1":
			downloadModels()
		case "2":
			setDefaultModel()
		case "3":
			fmt.Println("Exiting setup.")
			return
		default:
			fmt.Println("\nInvalid option.")
			fmt.Print("Press Enter to continue...")
			reader.ReadString('\n')
		}
	}
}

// --- Version Command ---

func showVersion() {
	fmt.Printf("go-transcribe: %s\n", version)
}

// --- Main ---

func main() {
	// Global flag for model path
	modelPath := flag.String("model", "", "Path to the whisper.cpp model file. Overrides the configured default.")
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [global options] <command> [command options]\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "\nGlobal Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr, "\nCommands:")
		fmt.Fprintln(os.Stderr, "  transcribe   Transcribe a media file")
		fmt.Fprintln(os.Stderr, "  setup        Enter interactive setup mode")
		fmt.Fprintln(os.Stderr, "  version      Show application version")
		os.Exit(1)
	}

	command := args[0]
	commandArgs := args[1:]

	// Default to "transcribe" command if the command is not recognized
	switch command {
	case "setup":
		setup(commandArgs)
	case "version":
		showVersion()
	case "transcribe":
		transcribe(commandArgs, *modelPath)
	default:
		// If the command is not a recognized command, assume it's a file path for transcription.
		transcribe(args, *modelPath)
	}
}
